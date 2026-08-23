package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/checker"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/odoconf"
	"github.com/ahmed/odoonoir/internal/ui"
)

// lineWriter streams command output line by line through a callback.
type lineWriter struct{ fn func(string) }

func (l lineWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			l.fn(line)
		}
	}
	return len(p), nil
}

// applyConfEdits sets several keys on a conf file in a single lossless
// load-edit-save pass.
func applyConfEdits(confPath string, edits map[string]string) error {
	c, err := odoconf.Load(confPath)
	if err != nil {
		return err
	}
	for k, v := range edits {
		c.Set(k, v)
	}
	return c.Save()
}

// copyTree duplicates a directory tree with cp -r (no-op when src is absent).
func copyTree(src, dst string, stdout func(string)) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("cp", "-r", src, dst)
	cmd.Stdout = &lineWriter{fn: stdout}
	cmd.Stderr = &lineWriter{fn: stdout}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	return nil
}

func newCloneCmd() *cobra.Command {
	var port int
	var noDB, noData, dbsAll bool
	cmd := &cobra.Command{
		Use:   "clone <name> <newname>",
		Short: "Clone an instance (source, custom modules, data, database)",
		Long: `Creates a new instance with the same Odoo version, custom addons,
data dir and a copied database. The source is cloned fresh and the database
is dumped + restored, so the original instance is never touched.

What you will see: the clone steps (clone source, venv, database dump +
restore, start). The source instance may keep running.`,
		Example: `  odoonoir clone myapp myapp2            # full copy incl. primary database
  odoonoir clone myapp myapp2 --dbs     # also copy additional databases
  odoonoir clone myapp myapp2 --no-data # copy without the database`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcName, newName := args[0], args[1]
			if err := db.IsValidName(newName); err != nil {
				return err
			}
			src, err := loadInstance(srcName)
			if err != nil {
				return err
			}
			if src.Adopted {
				return fmt.Errorf("cannot clone %q: it is an adopted instance with paths outside the standard layout", srcName)
			}
			if _, err := reg.Get(newName); err == nil {
				return fmt.Errorf("instance %s already exists", newName)
			} else if !errors.Is(err, instance.ErrNotFound) {
				return err
			}
			sp := src.ResolvePaths(instRoot(src))

			// pick a port: explicit flag, else the source port if still free,
			// else the next free port.
			if port == 0 {
				if !checker.PortInUse(src.Port) && registryPortFree(src, src.Port) == nil {
					port = src.Port
				} else if p, err := nextFreePort(8069); err == nil {
					port = p
				} else {
					return err
				}
			}
			if port > 65532 {
				return fmt.Errorf("port %d is too high — the longpoll port (port+3) would exceed the valid range", port)
			}
			if err := registryPortFree(src, port); err != nil {
				return err
			}
			longpoll := port + 3

			// reuse the original db credentials when available
			dbUser := src.DBUser
			dbPass := ""
			if c, err := odoconf.Load(sp.Conf); err == nil {
				if v, ok := c.Get("db_password"); ok {
					dbPass = v
				}
			}
			dbName := newName

			newInst := &instance.Instance{
				Name:         newName,
				Version:      src.Version,
				Branch:       src.Branch,
				SourceURL:    src.SourceURL,
				Port:         port,
				LongpollPort: longpoll,
				DBName:       dbName,
				DBUser:       dbUser,
				Root:         src.Root,
				Description:  "clone of " + srcName,
				Workers:      src.Workers,
				LogLevel:     src.LogLevel,
				PythonBin:    src.PythonBin,
			}

			fmt.Println(th.Infof("copying custom addons and data"))
			np := newInst.ResolvePaths(instRoot(newInst))
			prog := ui.NewProgress(cmd.OutOrStdout(), "copying custom addons and data")
			if err := copyTree(sp.Addons, np.Addons, func(s string) { prog.Line(s) }); err != nil {
				prog.Fail()
				return err
			}
			if !noData {
				if err := copyTree(sp.DataDir, np.DataDir, func(s string) { prog.Line(s) }); err != nil {
					prog.Fail()
					return err
				}
			}
			prog.Done()

			prog = ui.NewProgress(cmd.OutOrStdout(), "installing (source, venv, conf)")
			if err := installer.Install(cmd.Context(), cfg, newInst, installer.Options{
				Version:  src.Version,
				Port:     port,
				LongPoll: longpoll,
				DBUser:   dbUser,
				DBPass:   dbPass,
				DBName:   dbName,
				Workers:  src.Workers,
				LogLevel: src.LogLevel,
				Python:   src.PythonBin,
			}, func(s string) { prog.Line(s) }); err != nil {
				prog.Fail()
				return err
			}
			prog.Done()

			if !noDB {
				if err := ui.Step(cmd.OutOrStdout(), "copying database "+src.DBName+" -> "+dbName, func() error {
					pg := db.New(cfg)
					tmp, err := os.CreateTemp("", "odoonoir-clone-*.dump.gz")
					if err != nil {
						return err
					}
					tmpPath := tmp.Name()
					_ = tmp.Close()
					defer os.Remove(tmpPath)
					if err := pg.Backup(cmd.Context(), src.DBName, tmpPath, true, true); err != nil {
						return err
					}
					if err := pg.CreateDatabase(dbName, dbUser); err != nil {
						return err
					}
					return pg.Restore(cmd.Context(), dbName, tmpPath)
				}); err != nil {
					return err
				}
				if dbsAll {
					for _, extra := range src.Databases {
						if err := ui.Step(cmd.OutOrStdout(), "copying database "+extra, func() error {
							pg := db.New(cfg)
							tmp2, err := os.CreateTemp("", "odoonoir-clone-*.dump.gz")
							if err != nil {
								return err
							}
							extraTmp := tmp2.Name()
							_ = tmp2.Close()
							defer os.Remove(extraTmp)
							if err := pg.Backup(cmd.Context(), extra, extraTmp, true, true); err != nil {
								return err
							}
							if err := pg.CreateDatabase(extra, dbUser); err != nil {
								return err
							}
							return pg.Restore(cmd.Context(), extra, extraTmp)
						}); err != nil {
							return err
						}
						newInst.AddDB(extra)
					}
				}
			}

			if err := reg.Put(newInst); err != nil {
				return err
			}
			fmt.Println(th.Successf("instance %q cloned from %q", newName, srcName))
			fmt.Println(th.Panel("", th.KV([][2]string{
				{"url", fmt.Sprintf("http://localhost:%d", port)},
				{"db", dbName},
				{"start", "odoonoir start " + newName},
			})))
			return nil
		},
	}
	cmd.Flags().IntVarP(&port, "port", "p", 0, "HTTP port for the clone (default: source port if free, else next free)")
	cmd.Flags().BoolVar(&noDB, "no-db", false, "do not copy the database (start from an empty DB)")
	cmd.Flags().BoolVar(&noData, "no-data", false, "do not copy the filestore data dir")
	cmd.Flags().BoolVar(&dbsAll, "dbs", false, "also copy the additional databases of the source instance")
	return cmd
}

func newRenameCmd() *cobra.Command {
	var renameDB bool
	cmd := &cobra.Command{
		Use:   "rename <name> <newname>",
		Short: "Rename an instance (folder, conf paths, database)",
		Long: `Moves the instance folder, rewrites the conf paths (addons_path,
logfile, data_dir, db_name) and renames the database. The instance must be
stopped first.

What you will see: the folder move, conf rewrite and database rename as
they happen, then the new instance is listed.`,
		Example: `  odoonoir rename myapp myapp2   # move + rename database, paths rewritten`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldName, newName := args[0], args[1]
			if err := db.IsValidName(newName); err != nil {
				return err
			}
			inst, err := loadInstance(oldName)
			if err != nil {
				return err
			}
			if inst.Adopted {
				return fmt.Errorf("cannot rename %q: it is an adopted instance with paths outside the standard layout", oldName)
			}
			if _, err := reg.Get(newName); err == nil {
				return fmt.Errorf("instance %s already exists", newName)
			} else if !errors.Is(err, instance.ErrNotFound) {
				return err
			}
			status, _, err := statusOf(inst)
			if err != nil {
				return err
			}
			if status == instance.StatusRunning {
				return fmt.Errorf("instance %s is running — stop it first: odoonoir stop %s", oldName, oldName)
			}
			op := inst.ResolvePaths(instRoot(inst))
			np := (&instance.Instance{Name: newName}).ResolvePaths(instRoot(inst))

			if _, err := os.Stat(np.Root); err == nil {
				return fmt.Errorf("target folder already exists: %s", np.Root)
			}
			fmt.Println(th.Infof("moving folder"))
			if err := os.Rename(op.Root, np.Root); err != nil {
				return fmt.Errorf("move %s -> %s: %w", op.Root, np.Root, err)
			}

			dbRenameTo := ""
			if renameDB && inst.DBName == oldName {
				dbRenameTo = newName
			}
			fmt.Println(th.Infof("rewriting conf paths"))
			confEdits := map[string]string{
				"addons_path": strings.Join(installer.BuildAddonsPath(np), ","),
				"logfile":     np.Log,
				"data_dir":    np.DataDir,
			}
			if dbRenameTo != "" {
				confEdits["db_name"] = dbRenameTo
			}
			if err := applyConfEdits(np.Conf, confEdits); err != nil {
				return err
			}

			if dbRenameTo != "" {
				if err := ui.Step(cmd.OutOrStdout(), "renaming database "+inst.DBName+" -> "+dbRenameTo, func() error {
					return db.New(cfg).RenameDatabase(inst.DBName, dbRenameTo)
				}); err != nil {
					return err
				}
				inst.DBName = dbRenameTo
			} else if inst.DBName != oldName {
				fmt.Println(th.Hintf("database name kept (%s)", inst.DBName))
			}

			inst.Name = newName
			if err := reg.Put(inst); err != nil {
				return err
			}
			if err := reg.Delete(oldName); err != nil {
				return err
			}
			fmt.Println(th.Successf("instance %q renamed to %q", oldName, newName))
			fmt.Println(th.Hintf("start: odoonoir start %s", newName))
			return nil
		},
	}
	cmd.Flags().BoolVar(&renameDB, "db", true, "rename the database too")
	return cmd
}
