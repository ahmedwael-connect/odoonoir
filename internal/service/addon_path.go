package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmed/odoonoir/internal/odoconf"
)

// AddonPathEntry is one addons_path line with enabled flag.
type AddonPathEntry struct {
	Path     string `json:"path"`
	Enabled  bool   `json:"enabled"`
	Exists   bool   `json:"exists"`
	Position int    `json:"position"` // 1-based priority
	Comment  string `json:"comment,omitempty"`
}

// ListAddonPaths returns ordered entries, disabled = commented ; path.
func (s *Service) ListAddonPaths(name string) ([]AddonPathEntry, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, err
	}
	var paths []string
	var enabled []bool
	// odoconf keeps commented entries with Active=false but AllKeys includes them? Check: we need to parse raw file for disabled.
	// Instead read raw file for ; addons_path lines
	raw := conf.RawAddonsPathEntries() // will add helper; fallback to AddonsPath
	if raw != nil {
		for i, re := range raw {
			paths = append(paths, re.Path)
			enabled = append(enabled, re.Enabled)
			_ = i
		}
	}
	if len(paths) == 0 {
		// fallback to current active list
		active, _ := conf.AddonsPath()
		for _, ap := range active {
			paths = append(paths, ap)
			enabled = append(enabled, true)
		}
	}
	out := make([]AddonPathEntry, 0, len(paths))
	for i, ap := range paths {
		_, err := os.Stat(ap)
		out = append(out, AddonPathEntry{
			Path:     ap,
			Enabled:  enabled[i],
			Exists:   err == nil,
			Position: i + 1,
		})
	}
	return out, nil
}

// AddAddonPath inserts path at position (1-based, 0 = append default/last).
func (s *Service) AddAddonPath(name, path string, position int) ([]AddonPathEntry, error) {
	if path = strings.TrimSpace(path); path == "" {
		return nil, fmt.Errorf("path required")
	}
	if strings.Contains(path, ",") {
		return nil, fmt.Errorf("path cannot contain comma")
	}
	// clean
	path = filepath.Clean(path)
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, err
	}
	cur, _ := conf.AddonsPath()
	// check duplicate (enabled)
	for _, c := range cur {
		if c == path {
			return nil, fmt.Errorf("path %q already in addons_path at position %d", path, indexOf(cur, path)+1)
		}
	}
	// also check disabled
	if raw := conf.RawAddonsPathEntries(); raw != nil {
		for _, re := range raw {
			if re.Path == path && !re.Enabled {
				// re-enable instead
				return s.ToggleAddonPath(name, path, true)
			}
		}
	}
	if position <= 0 || position > len(cur)+1 {
		cur = append(cur, path)
	} else {
		// insert at position
		pos := position - 1
		cur = append(cur[:pos], append([]string{path}, cur[pos:]...)...)
	}
	if err := conf.SetAddonsPath(cur); err != nil {
		return nil, err
	}
	if err := conf.Save(); err != nil {
		return nil, err
	}
	return s.ListAddonPaths(name)
}

// RemoveAddonPath deletes path whether enabled or disabled.
func (s *Service) RemoveAddonPath(name, path string) ([]AddonPathEntry, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, err
	}
	// try remove from active
	if err := conf.AddonsRemove(path); err == nil {
		_ = conf.Save()
		return s.ListAddonPaths(name)
	}
	// try remove from raw (disabled)
	if raw := conf.RawAddonsPathEntries(); raw != nil {
		for _, re := range raw {
			if re.Path == path {
				// rebuild without this disabled entry
				conf.RemoveRawAddonsEntry(path)
				_ = conf.Save()
				return s.ListAddonPaths(name)
			}
		}
	}
	return nil, fmt.Errorf("path %q not in addons_path", path)
}

// ToggleAddonPath enables/disables (comment) a path.
func (s *Service) ToggleAddonPath(name, path string, enable bool) ([]AddonPathEntry, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, err
	}
	if enable {
		// if disabled exists, enable it
		if raw := conf.RawAddonsPathEntries(); raw != nil {
			for _, re := range raw {
				if re.Path == path && !re.Enabled {
					conf.EnableAddonsPath(path)
					_ = conf.Save()
					return s.ListAddonPaths(name)
				}
			}
		}
		// if not present, add
		return s.AddAddonPath(name, path, 0)
	}
	// disable = comment out
	cur, _ := conf.AddonsPath()
	found := false
	for _, c := range cur {
		if c == path {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("path %q not found enabled", path)
	}
	conf.DisableAddonsPath(path)
	_ = conf.Save()
	return s.ListAddonPaths(name)
}

// MoveAddonPath reorders from→to (1-based).
func (s *Service) MoveAddonPath(name string, from, to int) ([]AddonPathEntry, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, err
	}
	cur, _ := conf.AddonsPath()
	if from < 1 || from > len(cur) || to < 1 || to > len(cur) {
		return nil, fmt.Errorf("invalid move %d→%d (have %d entries)", from, to, len(cur))
	}
	// splice
	val := cur[from-1]
	tmp := append(cur[:from-1], cur[from:]...)
	pos := to - 1
	if to > from {
		pos = to - 1
	}
	// insert
	newPaths := make([]string, 0, len(tmp)+1)
	newPaths = append(newPaths, tmp[:pos]...)
	newPaths = append(newPaths, val)
	newPaths = append(newPaths, tmp[pos:]...)
	if err := conf.SetAddonsPath(newPaths); err != nil {
		return nil, err
	}
	_ = conf.Save()
	return s.ListAddonPaths(name)
}

func indexOf(arr []string, v string) int {
	for i, x := range arr {
		if x == v {
			return i
		}
	}
	return -1
}
