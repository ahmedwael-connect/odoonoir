package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/notify"
	"github.com/ahmed/odoonoir/internal/service"
	"github.com/ahmed/odoonoir/internal/ui"
)

// resolveDB resolves the optional database argument of database commands
// against the databases the instance serves. isPrimary reports whether the
// resolved name is the instance's primary database. An empty arg resolves
// to the primary database.
func resolveDB(inst *instance.Instance, arg string) (name string, isPrimary bool, err error) {
	if arg == "" {
		return inst.DBName, true, nil
	}
	for _, d := range inst.AllDBs() {
		if d == arg {
			return arg, arg == inst.DBName, nil
		}
	}
	return "", false, fmt.Errorf("database %q is not served by instance %q (see: odoonoir list %s)", arg, inst.Name, inst.Name)
}

// resolveDBNew is like resolveDB but additionally accepts a database name
// that the instance does not serve yet, so a dump can be restored into a
// fresh database (it gets tracked after a successful restore). The name
// must still be a valid database identifier.
func resolveDBNew(inst *instance.Instance, arg string) (name string, tracked bool, err error) {
	if arg == "" {
		return inst.DBName, true, nil
	}
	for _, d := range inst.AllDBs() {
		if d == arg {
			return arg, true, nil
		}
	}
	if err := db.IsValidName(arg); err != nil {
		return "", false, err
	}
	return arg, false, nil
}

// requireStopped refuses database mutations while the instance process runs.
func requireStopped(inst *instance.Instance) error {
	status, _, err := statusOf(inst)
	if err != nil {
		return err
	}
	if status == instance.StatusRunning {
		return fmt.Errorf("instance %s is running and holds database connections — stop it first: odoonoir stop %s", inst.Name, inst.Name)
	}
	return nil
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <instance> <db>",
		Short: "Create and initialize a database for an instance (-i base)",
		Long: `Creates the database (if missing) and runs Odoo's base install against
it, so the database becomes a fully usable Odoo database served by the
instance. The instance must be stopped for the init step.

The database is tracked as an additional database of the instance unless it
is the primary one (the primary database is initialized by ` + "`odoonoir create`" + `).`,
		Example: `  odoonoir init myapp sales          # create + initialize "sales" (-i base)
  odoonoir init myapp myapp         # initialize the primary database when missing
  # after init you can start it directly:
  odoonoir start myapp sales`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			dbName := args[1]
			if err := db.IsValidName(dbName); err != nil {
				return err
			}
			if dbName == inst.DBName {
				pg := db.New(cfg)
				if err := pg.ServerRunning(); err != nil {
					return err
				}
				exists, err := pg.DatabaseExists(dbName)
				if err != nil {
					return err
				}
				if exists {
					initialized, err := pg.IsInitialized(dbName)
					if err != nil {
						return err
					}
					if initialized {
						return fmt.Errorf("database %s is the instance primary database — it is initialized by: odoonoir create %s", dbName, inst.Name)
					}
				}
			}
			if err := requireStopped(inst); err != nil {
				return err
			}
			var prog *ui.Progress
			err = svc.InitDB(cmd.Context(), inst.Name, dbName, func(e service.Event) {
				switch e.Kind {
				case service.StepStart:
					prog = ui.NewProgress(cmd.OutOrStdout(), e.Step)
				case service.LogLine:
					if prog != nil {
						prog.Line(e.Message)
					}
				case service.StepDone:
					if prog != nil {
						prog.Done()
					}
				case service.StepFail:
					if prog != nil {
						prog.Fail()
					}
				}
			})
			if err != nil {
				return err
			}
			fmt.Println(th.Successf("database %s initialized — served by instance %s", dbName, inst.Name))
			fmt.Println(th.Hintf("open: http://localhost:%d/web?db=%s", inst.Port, dbName))
			if nerr := notify.Notify(cfg, "db.init", "database "+dbName+" ready", "initialized on instance "+inst.Name); nerr != nil {
				warn("notify: %v", nerr)
			}
			return nil
		},
	}
}

func newBackupCmd() *cobra.Command {
	var out string
	var custom bool
	var compress bool
	cmd := &cobra.Command{
		Use:   "backup <instance> [db]",
		Short: "Backup a database of an instance to a dump file",
		Long: `Dumps a database served by the instance. Without a database name the
primary database is backed up. The instance may stay running.`,
		Example: `  odoonoir backup myapp                   # primary database
  odoonoir backup myapp sales             # a specific database
  odoonoir backup myapp -o /tmp/db.dump   # custom output path`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			dbName, _, err := resolveDB(inst, dbArg(args, 1))
			if err != nil {
				return err
			}
			if out == "" {
				stamp := time.Now().Format("20060102_150405")
				ext := ".dump"
				if !custom && compress {
					ext = ".dump.gz"
				}
				out = filepath.Join(inst.ResolvePaths(instRoot(inst)).Root,
					"backups", dbName+"_"+stamp+ext)
			}
			var prog *ui.Progress
			err = svc.Backup(cmd.Context(), inst.Name, dbName, out, custom, compress, func(e service.Event) {
				switch e.Kind {
				case service.StepStart:
					prog = ui.NewProgress(cmd.OutOrStdout(), e.Step)
				case service.LogLine:
					if prog != nil {
						prog.Line(e.Message)
					}
				case service.StepDone:
					if prog != nil {
						prog.Done()
					}
				case service.StepFail:
					if prog != nil {
						prog.Fail()
					}
				}
			})
			if err != nil {
				return err
			}
			fmt.Println(th.Successf("backup written: %s", out))
			if nerr := notify.Notify(cfg, "backup.done", "backup "+inst.Name+" complete", "dumped to "+out); nerr != nil {
				warn("notify: %v", nerr)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "backup file path (default: <instance>/backups/<db>_<timestamp>.dump)")
	cmd.Flags().BoolVar(&custom, "custom", true, "pg_dump custom format (-Fc); disable for plain SQL")
	cmd.Flags().BoolVar(&compress, "compress", false, "compress the dump (gzip; pg_dump -Z9 for custom)")
	return cmd
}

func newRestoreCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "restore <instance> <dump-file> [db]",
		Short: "Restore a dump into a database of an instance",
		Long: `Restores a pg_dump into a database served by the instance. The database
must not exist yet (or pass --force to drop and recreate it), and the
instance must be stopped first.

The dump file is validated BEFORE any database is touched, so a missing
or corrupt dump never costs you an existing database. Restoring into a
database name the instance does not serve yet is allowed — it is tracked
automatically after a successful restore.`,
		Example: `  odoonoir restore myapp backup.dump            # into the primary database
  odoonoir restore myapp backup.dump sales      # into a specific database
  odoonoir restore myapp backup.dump staging    # fresh db, tracked automatically
  odoonoir restore --force myapp backup.dump    # replace the existing database`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			dbName, tracked, err := resolveDBNew(inst, dbArg(args, 2))
			if err != nil {
				return err
			}
			var prog *ui.Progress
			err = svc.Restore(cmd.Context(), inst.Name, dbName, args[1], service.RestoreOptions{Force: force}, func(e service.Event) {
				switch e.Kind {
				case service.StepStart:
					prog = ui.NewProgress(cmd.OutOrStdout(), e.Step)
				case service.LogLine:
					if prog != nil {
						prog.Line(e.Message)
					}
				case service.StepDone:
					if prog != nil {
						prog.Done()
					}
				case service.StepFail:
					if prog != nil {
						prog.Fail()
					}
				}
			})
			if err != nil {
				return err
			}
			fmt.Println(th.Successf("database %s restored from %s", dbName, args[1]))
			if !tracked {
				fmt.Println(th.Hintf("database %s is now served by instance %s", dbName, inst.Name))
			}
			if nerr := notify.Notify(cfg, "db.restore", "database "+dbName+" restored", "instance "+inst.Name); nerr != nil {
				warn("notify: %v", nerr)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "drop the existing database before restoring")
	return cmd
}

func newDropCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "drop <instance> <db>",
		Short: "Drop a database of an instance",
		Long: `Drops a database served by the instance. The instance must be stopped
first, and the database is untracked from the instance afterwards.`,
		Example: `  odoonoir drop myapp sales          # drop the "sales" database
  odoonoir drop --force myapp sales # confirm (required — this is destructive)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			dbName, _, err := resolveDB(inst, args[1])
			if err != nil {
				return err
			}
			if !force {
				return fmt.Errorf("this is destructive — pass --force to drop database %s", dbName)
			}
			if err := svc.DropDB(inst.Name, dbName); err != nil {
				return err
			}
			fmt.Println(th.Successf("database %s dropped", dbName))
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm the drop")
	return cmd
}

func newSetPrimaryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-primary <instance> <db>",
		Short: "Change the primary database of an instance",
		Long:  `Changes which database is the primary (DBName) for an instance. The new primary must be one of the databases already served by the instance. The old primary becomes an additional database. The odoo.conf db_name is updated accordingly.`,
		Example: `  odoonoir db set-primary myapp myapp2  # make myapp2 the primary`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			dbName, _, err := resolveDB(inst, args[1])
			if err != nil {
				return err
			}
			if err := svc.SetPrimaryDatabase(inst.Name, dbName); err != nil {
				return err
			}
			fmt.Println(th.Successf("primary database of %s is now %s", inst.Name, dbName))
			return nil
		},
	}
}

func newDbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database operations for an instance",
	}
	cmd.AddCommand(newSetPrimaryCmd())
	return cmd
}

// listDatabases prints the databases served by an instance.
func listDatabases(inst *instance.Instance, out interface{ Write([]byte) (int, error) }) error {
	pg := db.New(cfg)
	if err := pg.ServerRunning(); err != nil {
		return err
	}
	names := inst.AllDBs()
	if len(names) == 0 {
		return fmt.Errorf("instance %q has no databases registered", inst.Name)
	}
	rows := make([][]string, 0, len(names))
	for _, n := range names {
		mark := " "
		if n == inst.DBName {
			mark = "*"
		}
		size := "-"
		if s, err := pg.DatabaseSize(n); err == nil && s > 0 {
			size = humanSizeBytes(s)
		}
		owner := "-"
		if o, err := pg.DatabaseOwner(n); err == nil {
			owner = o
		}
		init := "-"
		if ok, err := pg.IsInitialized(n); err == nil {
			init = fmt.Sprint(ok)
		}
		rows = append(rows, []string{mark + n, size, owner, init})
	}
	body := th.Table([]string{"DATABASE", "SIZE", "OWNER", "INITIALIZED"}, rows, ui.WithTitle("databases of "+inst.Name))
	fmt.Fprintln(out, body)
	fmt.Fprintln(out, th.Hintf("* = primary database"))
	return nil
}

// dbArg returns the optional database argument at index i, if present.
func dbArg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func humanSizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
