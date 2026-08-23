// Package service is the toolkit-agnostic engine shared by the CLI and the
// future Wails GUI. It owns the registry/config/DB access and exposes
// typed operations that emit Event streams for long-running work, so a
// UI layer never touches os/exec or instance internals directly.
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/odoconf"
	"github.com/ahmed/odoonoir/internal/proc"
)

// Service is the application engine. Create one per process; it is safe to
// call from multiple goroutines (registry access is file-locked).
type Service struct {
	mu  sync.Mutex
	cfg *config.Config
	reg *instance.Registry
	pg  *db.PG
}

// New loads the global config and registry and returns a Service.
func New(cfg *config.Config) (*Service, error) {
	reg, err := instance.NewRegistry(cfg.RegistryPath())
	if err != nil {
		return nil, err
	}
	return &Service{cfg: cfg, reg: reg, pg: db.New(cfg)}, nil
}

// InstanceView is a flat, GUI-friendly snapshot of an instance.
type InstanceView struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Status  instance.Status `json:"status"`
	PID     int             `json:"pid"`
	Port    int             `json:"port"`
	DBName  string          `json:"dbName"`
	DBs     int             `json:"dbs"`
	Path    string          `json:"path"`
	Adopted bool            `json:"adopted"`
}

// Instances returns every registered instance with its live status.
func (s *Service) Instances() ([]InstanceView, error) {
	all, err := s.reg.All()
	if err != nil {
		return nil, err
	}
	out := make([]InstanceView, 0, len(all))
	for _, inst := range all {
		p := inst.ResolvePaths(s.rootFor(inst))
		mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
		status, pid, err := mgr.Status()
		if err != nil {
			status = instance.StatusUnknown
		}
		out = append(out, InstanceView{
			Name:    inst.Name,
			Version: inst.Version,
			Status:  status,
			PID:     pid,
			Port:    inst.Port,
			DBName:  inst.DBName,
			DBs:     len(inst.AllDBs()),
			Path:    p.Root,
			Adopted: inst.Adopted,
		})
	}
	return out, nil
}

// StatusView is the runtime detail of a single instance.
type StatusView struct {
	Instance InstanceView `json:"Instance"`
	Serving  string       `json:"Serving"` // database currently served, if any
	Conf     string       `json:"Conf"`
	Log      string       `json:"Log"`
}

// Status returns the live status of one instance.
func (s *Service) Status(name string) (StatusView, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return StatusView{}, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
	status, pid, err := mgr.Status()
	if err != nil {
		status = instance.StatusUnknown
	}
	return StatusView{
		Instance: InstanceView{
			Name:    inst.Name,
			Version: inst.Version,
			Status:  status,
			PID:     pid,
			Port:    inst.Port,
			DBName:  inst.DBName,
			DBs:     len(inst.AllDBs()),
			Path:    p.Root,
			Adopted: inst.Adopted,
		},
		Serving: mgr.ServingDB(),
		Conf:    p.Conf,
		Log:     p.Log,
	}, nil
}

// Start launches an instance. db selects the database to serve (empty =
// primary). emit receives StatusChanged events.
func (s *Service) Start(ctx context.Context, name, dbName string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if dbName != "" {
		if err := s.requireDB(inst, dbName); err != nil {
			return err
		}
	} else {
		dbName = inst.DBName
	}
	mgr := s.procFor(inst)
	if err := mgr.Start(dbName); err != nil {
		return err
	}
	if err := waitForStable(ctx, mgr, s.logPath(inst)); err != nil {
		return err
	}
	emit(Event{Kind: StatusChanged, Instance: name, Message: fmt.Sprintf("instance %s started serving %q", name, dbName)})
	return nil
}

// Stop stops an instance, emitting StatusChanged on success.
func (s *Service) Stop(ctx context.Context, name string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	if err := s.procFor(inst).Stop(); err != nil {
		return err
	}
	emit(Event{Kind: StatusChanged, Instance: name, Message: "instance " + name + " stopped"})
	return nil
}

// Restart restarts an instance, optionally switching the served database.
func (s *Service) Restart(ctx context.Context, name, dbName string, emit Sink) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	mgr := s.procFor(inst)
	if dbName != "" {
		if err := s.requireDB(inst, dbName); err != nil {
			return err
		}
		if err := mgr.Stop(); err != nil {
			return err
		}
		if err := mgr.Start(dbName); err != nil {
			return err
		}
	} else if err := mgr.Restart(); err != nil {
		return err
	}
	if err := waitForStable(ctx, mgr, s.logPath(inst)); err != nil {
		return err
	}
	emit(Event{Kind: StatusChanged, Instance: name, Message: "instance " + name + " restarted"})
	return nil
}

// Conf returns the instance's odoo.conf (lossless view/edit).
func (s *Service) Conf(name string) (*odoconf.OdooConf, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	return odoconf.Load(p.Conf)
}

// ConfPath resolves the conf path of an instance without loading it.
func (s *Service) ConfPath(name string) (string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return "", err
	}
	return inst.ResolvePaths(s.rootFor(inst)).Conf, nil
}

// requireStopped verifies an instance is not currently running (database
// operations and restores must not race a live server).
func (s *Service) requireStopped(name string) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
	status, _, err := mgr.Status()
	if err != nil {
		return err
	}
	if status == instance.StatusRunning {
		return fmt.Errorf("instance %q is running — stop it first (odoonoir stop %s)", name, name)
	}
	return nil
}

func (s *Service) requireDB(inst *instance.Instance, name string) error {
	for _, d := range inst.AllDBs() {
		if d == name {
			return nil
		}
	}
	return fmt.Errorf("database %q is not served by instance %q", name, inst.Name)
}

func (s *Service) rootFor(inst *instance.Instance) string {
	if inst.Root != "" {
		return inst.Root
	}
	return s.cfg.InstancesDir()
}

func (s *Service) procFor(inst *instance.Instance) *proc.Manager {
	p := inst.ResolvePaths(s.rootFor(inst))
	return proc.New(p, installer.PythonFor(inst, p), p.Conf)
}

func (s *Service) logPath(inst *instance.Instance) string {
	return inst.ResolvePaths(s.rootFor(inst)).Log
}

// ReadConf reads all key-value pairs from the instance's odoo.conf.
func (s *Service) ReadConf(name string) ([]odoconf.Entry, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	c, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, err
	}
	return c.AllKeys(), nil
}

// SetConf updates a key in the instance's odoo.conf and saves it.
func (s *Service) SetConf(name string, key string, value string) error {
	inst, err := s.reg.Get(name)
	if err != nil {
		return err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	c, err := odoconf.Load(p.Conf)
	if err != nil {
		return err
	}
	c.Set(key, value)
	return c.Save()
}

// Instance returns the raw registry record for a name.
func (s *Service) Instance(name string) (*instance.Instance, error) {
	return s.reg.Get(name)
}

// IsNotFound reports whether err is a registry miss.
func IsNotFound(err error) bool { return errors.Is(err, instance.ErrNotFound) }
