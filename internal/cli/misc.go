package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/notify"
	"github.com/ahmed/odoonoir/internal/proc"
	"github.com/ahmed/odoonoir/internal/service"
	"github.com/ahmed/odoonoir/internal/ui"
)

func newUpdateCmd() *cobra.Command {
	var installMods []string
	var updateMods []string
	var upgradeAll bool
	var flagRestart bool
	var flagDB string
	var prog *ui.Progress
	cmd := &cobra.Command{
		Use:   "update <name> [db]",
		Short: "Update the source, then install/upgrade modules",
		Long: `git pull the Odoo source, reinstall requirements, then
optionally install (-i) or upgrade (-u) modules in a database of the
instance. Without a database name the primary database is used (the
--db flag is an alias for the positional one). Use --restart to restart
the instance automatically when done.`,
		Example: `  odoonoir update myapp                   # git pull only
  odoonoir update myapp -i sale_management # install a module in the primary db
  odoonoir update myapp sales -u sale_management # upgrade a module in "sales"
  odoonoir update myapp --restart          # pull + restart the instance`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			if db := dbArg(args, 1); db != "" {
				flagDB = db
			}
			if flagDB != "" {
				found := false
				for _, d := range inst.AllDBs() {
					if d == flagDB {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("database %q is not served by instance %q (see: odoonoir list %s)", flagDB, inst.Name, inst.Name)
				}
			}
			out := cmd.OutOrStdout()
			err = svc.Update(cmd.Context(), inst.Name, service.UpdateOptions{
				InstallMods: installMods,
				UpdateMods:  updateMods,
				UpgradeAll:  upgradeAll,
				DB:          flagDB,
			}, func(e service.Event) {
				switch e.Kind {
				case service.StepStart:
					prog = ui.NewProgress(out, e.Step)
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
				if prog != nil {
					prog.Fail()
				}
				if nerr := notify.Notify(cfg, "update.failed", "update "+inst.Name+" failed", err.Error()); nerr != nil {
					warn("notify: %v", nerr)
				}
				return err
			}
			ranMods := len(installMods) > 0 || len(updateMods) > 0 || upgradeAll
			done := ""
			if flagRestart {
				fmt.Println(th.Infof("restarting"))
				if err := restartInstance(inst); err != nil {
					return err
				}
				done = "updated and restarted"
			} else if ranMods {
				fmt.Println(th.Successf("modules updated — restart the instance to serve the change: odoonoir restart %s", inst.Name))
				done = "modules updated — restart to apply"
			} else {
				fmt.Println(th.Successf("source updated — restart to apply: odoonoir restart %s", inst.Name))
				done = "source updated — restart to apply"
			}
			if nerr := notify.Notify(cfg, "update.done", "update "+inst.Name+" complete", done); nerr != nil {
				warn("notify: %v", nerr)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&installMods, "install", "i", nil, "modules to install (comma separated)")
	cmd.Flags().StringSliceVarP(&updateMods, "update", "u", nil, "modules to upgrade (comma separated)")
	cmd.Flags().BoolVar(&upgradeAll, "upgrade-all", false, "upgrade all installable modules")
	cmd.Flags().BoolVar(&flagRestart, "restart", false, "restart the instance when done")
	cmd.Flags().StringVar(&flagDB, "db", "", "database to run -i/-u against (alias for the positional <db>; default: the primary database)")
	return cmd
}

// restartInstance stops and starts the instance through its proc manager.
func restartInstance(inst *instance.Instance) error {
	p := inst.ResolvePaths(instRoot(inst))
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf)
	if err := mgr.Restart(); err != nil {
		return err
	}
	return waitForStable(inst, mgr)
}

func newRemoveCmd() *cobra.Command {
	var force, keepData bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an instance (stop, optionally drop DB, delete files)",
		Long: `Stops the instance, deletes its folder and drops its primary database.
Additional databases are left untouched (drop them with
` + "`odoonoir drop <name> <db>`" + `). Adopted instances are only unregistered:
their files and databases are never touched.`,
		Example: `  odoonoir remove --force myapp2          # delete folder + database
  odoonoir remove --force --keep-data myapp2  # delete files, keep the database`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			if !force {
				return fmt.Errorf("this deletes the instance folder and database — pass --force to confirm")
			}
			if inst.Adopted {
				// An adopted instance's files belong to the user's own
				// installation: unregister only. The process is never
				// stopped and the database is never dropped here; use
				// `odoonoir drop <name> <db>` for the database.
				metaDir := filepath.Join(instRoot(inst), inst.Name)
				if err := reg.Delete(inst.Name); err != nil {
					return err
				}
				fmt.Println(th.Successf("instance %s unregistered (adopted: source, conf and database left untouched)", inst.Name))
				if _, err := os.Stat(metaDir); err == nil {
					fmt.Println(th.Hintf("metadata (logs/backups/pid) kept at %s — remove it manually if desired", metaDir))
				}
				return nil
			}
			p := inst.ResolvePaths(instRoot(inst))
			// stop it first so the process does not hold files open
			mgr := newProcFor(inst)
			_ = mgr.Stop()
			if err := os.RemoveAll(p.Root); err != nil {
				return fmt.Errorf("remove %s: %w", p.Root, err)
			}
			if !keepData {
				others, err := reg.All()
				if err != nil {
					return err
				}
				for _, o := range others {
					if o.Name != inst.Name && o.DBName == inst.DBName {
						return fmt.Errorf("database %s is also used by instance %s — pass --keep-data or remove %s first", inst.DBName, o.Name, o.Name)
					}
				}
				pg := db.New(cfg)
				if err := pg.DropDatabase(inst.DBName); err != nil {
					warn("could not drop database %s: %v", inst.DBName, err)
				} else {
					fmt.Println(th.Successf("database %s dropped", inst.DBName))
				}
				if len(inst.Databases) > 0 {
					warn("additional databases left untouched: %s (drop with: odoonoir drop %s <db>)",
						strings.Join(inst.Databases, ", "), inst.Name)
				}
			}
			if err := reg.Delete(inst.Name); err != nil {
				return err
			}
			fmt.Println(th.Successf("instance %s removed", inst.Name))
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm removal")
	cmd.Flags().BoolVar(&keepData, "keep-data", false, "keep the database (default: drop it)")
	return cmd
}

func newShellCmd() *cobra.Command {
	var dbName string
	var code string
	var script string
	cmd := &cobra.Command{
		Use:   "shell <name> [db] [-- args...]",
		Short: "Open an interactive odoo shell for the instance",
		Long: `Opens ` + "`odoo shell -d <db>`" + ` for the instance with the same Python
environment. Without a database name the primary database is used; the
--db flag is an alias for the positional one.

Everything after -- is passed to odoo as-is. A one-liner (-c) or a script
file (--script) runs non-interactively and prints the result.`,
		Example: `  odoonoir shell myapp              # primary database
  odoonoir shell myapp sales        # the "sales" database
  odoonoir shell myapp -c "print(env['ir.module.module'].search_count([]))"
  odoonoir shell myapp --script script.py
  odoonoir shell myapp -- --no-http  # passthrough args to odoo`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			// Everything after "--" goes to odoo untouched; only the first
			// two positional arguments are instance name / database.
			passthrough := passthroughFromArgs(os.Args)
			positional := args
			if n := len(passthrough); n > 0 && len(positional) > n {
				positional = positional[:len(positional)-n]
			}
			if db := dbArg(positional, 1); db != "" {
				dbName = db
			}
			if dbName != "" {
				found := false
				for _, d := range inst.AllDBs() {
					if d == dbName {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("database %q is not served by instance %q (see: odoonoir list %s)", dbName, inst.Name, inst.Name)
				}
			}
			p := inst.ResolvePaths(instRoot(inst))
			py := installer.PythonFor(inst, p)
			target := dbName
			if target == "" {
				target = inst.DBName
			}
			if code != "" && script != "" {
				return fmt.Errorf("use either -c or --script, not both")
			}
			shell := exec.Command(py, append([]string{"-m", "odoo", "shell", "-c", p.Conf, "-d", target}, passthrough...)...)
			if code != "" {
				shell.Stdin = strings.NewReader(code + "\n")
			} else if script != "" {
				data, err := os.ReadFile(script)
				if err != nil {
					return fmt.Errorf("read script: %w", err)
				}
				shell.Stdin = strings.NewReader(string(data))
			} else {
				shell.Stdin = cmd.InOrStdin()
			}
			shell.Stdout = cmd.OutOrStdout()
			shell.Stderr = cmd.OutOrStderr()
			shell.Dir = p.Source
			return shell.Run()
		},
	}
	cmd.Flags().StringVarP(&dbName, "db", "d", "", "database to open (alias for the positional <db>; default: instance database)")
	cmd.Flags().StringVarP(&code, "code", "c", "", "run one line of python in the shell and exit (e.g. env['res.partner'].search_count([]))")
	cmd.Flags().StringVar(&script, "script", "", "run a python script file in the shell and exit")
	cmd.Flags().SetInterspersed(false)
	return cmd
}

// passthroughFromArgs extracts the tokens after "--" from the raw argv.
func passthroughFromArgs(argv []string) []string {
	for i, a := range argv {
		if a == "--" {
			return argv[i+1:]
		}
	}
	return nil
}
