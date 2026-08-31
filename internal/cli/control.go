package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/proc"
	"github.com/ahmed/odoonoir/internal/service"
	"github.com/ahmed/odoonoir/internal/ui"
)

func newListCmd() *cobra.Command {
	var flagRunning bool
	var flagJSON bool
	cmd := &cobra.Command{
		Use:   "list [instance]",
		Short: "List instances, or the databases of one instance",
		Long: `Without arguments, lists every managed instance (name, version, status,
port, primary database, databases count, path). With an instance name,
lists the databases served by that instance (size, owner, whether it is
initialized; * marks the primary database).

--json prints the instance list as JSON (scriptable); --running filters
to instances that are currently running.`,
		Example: `  odoonoir list                # all instances
  odoonoir list myapp          # databases served by myapp
  odoonoir list --running      # only running instances
  odoonoir list --json         # machine-readable output`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				inst, err := loadInstance(args[0])
				if err != nil {
					return err
				}
				return listDatabases(inst, cmd.OutOrStdout())
			}
			instances, err := svc.Instances()
			if err != nil {
				return err
			}
			if len(instances) == 0 {
				fmt.Println(th.Hintf("no instances yet — create one with: odoonoir create <name> -v 18"))
				return nil
			}
			type instRow struct {
				Name    string
				Version string
				Status  instance.Status
				PID     int
				Port    int
				DBName  string
				DBs     int
				Path    string
			}
			rows := make([]instRow, 0, len(instances))
			for _, v := range instances {
				if flagRunning && v.Status != instance.StatusRunning {
					continue
				}
				rows = append(rows, instRow{
					Name:    v.Name,
					Version: v.Version,
					Status:  v.Status,
					PID:     v.PID,
					Port:    v.Port,
					DBName:  v.DBName,
					DBs:     v.DBs,
					Path:    v.Path,
				})
			}
			if len(rows) == 0 {
				if flagRunning {
					fmt.Println(th.Hintf("no instances are running"))
					return nil
				}
				fmt.Println(th.Hintf("no instances yet — create one with: odoonoir create <name> -v 18"))
				return nil
			}
			if flagJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			tableRows := make([][]string, 0, len(rows))
			for _, r := range rows {
				statusStr := string(r.Status)
				if r.Status == instance.StatusRunning {
					statusStr = fmt.Sprintf("%s (pid %d)", string(r.Status), r.PID)
				}
				tableRows = append(tableRows, []string{
					r.Name,
					r.Version,
					th.StatusPill(statusStr),
					fmt.Sprint(r.Port),
					r.DBName,
					fmt.Sprint(r.DBs),
					r.Path,
				})
			}
			fmt.Println(th.Table(
				[]string{"NAME", "VERSION", "STATUS", "PORT", "DATABASE", "DBS", "PATH"},
				tableRows,
				ui.WithTitle("instances"),
			))
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagRunning, "running", false, "only show running instances")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "print the instance list as JSON")
	return cmd
}

func instanceFromArgs(cmd *cobra.Command, args []string) (*instance.Instance, error) {
	return loadInstance(args[0])
}

// targetInstances resolves the instances a control command applies to:
// --all selects every registered instance, otherwise the single instance
// from the positional argument (interactively when omitted).
func targetInstances(cmd *cobra.Command, args []string, all bool, what string) ([]*instance.Instance, error) {
	if all {
		allInsts, err := reg.All()
		if err != nil {
			return nil, err
		}
		if len(allInsts) == 0 {
			return nil, fmt.Errorf("no instances registered — create one with: odoonoir create <name>")
		}
		return allInsts, nil
	}
	var inst *instance.Instance
	var err error
	if len(args) > 0 {
		inst, err = instanceFromArgs(cmd, args)
	} else if isInteractive() {
		inst, err = pickInstance(what)
	} else {
		return nil, fmt.Errorf("instance name required — `odoonoir %s <name>` (or pass --all) (run in a terminal to pick interactively)", cmd.Name())
	}
	if err != nil {
		return nil, err
	}
	return []*instance.Instance{inst}, nil
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Show instance runtime status",
		Long: `Shows the instance's runtime state (running/stopped, pid, port), its
primary database, and — when it was started with a specific database via
` + "`odoonoir start <name> <db>`" + ` — which database it is currently serving.`,
		Example: `  odoonoir status myapp`,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			var err error
			if len(args) == 1 {
				name = args[0]
			} else if isInteractive() {
				name, err = pickInstanceName("which instance?")
			} else {
				return fmt.Errorf("instance name required — `odoonoir status <name>` (run in a terminal to pick interactively)")
			}
			if err != nil {
				return err
			}
			st, err := svc.Status(name)
			if err != nil {
				return err
			}
			statusStr := string(st.Instance.Status)
			if st.Instance.PID > 0 {
				statusStr += fmt.Sprintf(" (pid %d)", st.Instance.PID)
			}
			rows := [][2]string{
				{"version", st.Instance.Version},
				{"status", th.StatusPill(statusStr)},
			}
			if st.Instance.PID > 0 {
				rows = append(rows, [2]string{"pid", fmt.Sprint(st.Instance.PID)})
			}
			rows = append(rows,
				[2]string{"port", fmt.Sprint(st.Instance.Port)},
				[2]string{"database", st.Instance.DBName},
			)
			if st.Serving != "" {
				rows = append(rows, [2]string{"serving", st.Serving})
			}
			rows = append(rows,
				[2]string{"conf", st.Conf},
				[2]string{"log", st.Log},
			)
			fmt.Println(th.Panel("instance "+name, th.KV(rows)))
			return nil
		},
	}
}

func newStartCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "start <name> [db]",
		Short: "Start an instance (optionally serving one of its databases)",
		Long: `Starts the Odoo instance. Without a database name the instance serves its
primary database. With a database name, Odoo is started with -d <db> so
that database is served; the choice is remembered for ` + "`odoonoir restart`" + `
and shown by ` + "`odoonoir status`" + `.`,
		Example: `  odoonoir start myapp            # serve the primary database
  odoonoir start myapp sales      # serve the "sales" database
  odoonoir start                  # pick an instance and database interactively
  odoonoir start --all            # start every instance`,
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			insts, err := targetInstances(cmd, args, all, "start which instance?")
			if err != nil {
				return err
			}
			// with --all every instance serves its primary database
			dbName := ""
			if !all {
				if db := dbArg(args, 1); db != "" {
					dbName = db
				}
				if dbName == "" && len(args) == 0 {
					dbName, err = pickDatabase(insts[0], "serve which database?")
					if err != nil {
						return err
					}
				}
			}
			for _, inst := range insts {
				served := dbName
				served, _, err = resolveDB(inst, served)
				if err != nil {
					return err
				}
				if err := svc.Start(cmd.Context(), inst.Name, served, func(service.Event) {}); err != nil {
					return err
				}
				if served == inst.DBName {
					fmt.Println(th.Successf("instance %s started — http://localhost:%d", inst.Name, inst.Port))
				} else {
					fmt.Println(th.Successf("instance %s started serving %q — http://localhost:%d/web?db=%s", inst.Name, served, inst.Port, served))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "start every instance (each with its primary database)")
	return cmd
}

func newStopCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "stop <name>",
		Short:   "Stop an instance (SIGTERM, escalate to SIGKILL)",
		Example: `  odoonoir stop myapp   # graceful stop, waits up to 10s`,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			insts, err := targetInstances(cmd, args, all, "stop which instance?")
			if err != nil {
				return err
			}
			for _, inst := range insts {
				if err := svc.Stop(cmd.Context(), inst.Name, func(service.Event) {}); err != nil {
					return err
				}
				fmt.Println(th.Successf("instance %s stopped", inst.Name))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "stop every instance")
	return cmd
}

func newRestartCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "restart <name> [db]",
		Short: "Restart an instance (keep or switch its database)",
		Long: `Restarts the Odoo instance. Without a database name the instance restarts
with the database it was last started with (` + "`odoonoir start <name> <db>`" + `,
otherwise the primary database). With a database name the instance is
restarted serving that database instead.`,
		Example: `  odoonoir restart myapp          # keep the current database
  odoonoir restart myapp sales    # switch to serving the "sales" database
  odoonoir restart --all          # restart every instance`,
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			insts, err := targetInstances(cmd, args, all, "restart which instance?")
			if err != nil {
				return err
			}
			for _, inst := range insts {
				dbName := ""
				if !all {
					dbName = dbArg(args, 1)
				}
				if err := svc.Restart(cmd.Context(), inst.Name, dbName, func(service.Event) {}); err != nil {
					return err
				}
				fmt.Println(th.Successf("instance %s restarted — http://localhost:%d", inst.Name, inst.Port))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "restart every instance")
	return cmd
}

// newProcFor builds a process manager for an instance using global config.
func newProcFor(inst *instance.Instance) *proc.Manager {
	p := inst.ResolvePaths(instRoot(inst))
	return proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
}

// registryPortFree verifies that no OTHER registered instance uses the given
// port (the OS-level port check is done separately). skip is the instance the
// caller is about to create/clone (its own port is not a conflict).
func registryPortFree(skip *instance.Instance, port int) error {
	if port < 1 || port > 65532 {
		return fmt.Errorf("invalid port %d", port)
	}
	all, err := reg.All()
	if err != nil {
		return err
	}
	for _, other := range all {
		if skip != nil && other.Name == skip.Name {
			continue
		}
		if other.Port == port {
			return fmt.Errorf("port %d is already used by instance %q — pass --port to pick another", port, other.Name)
		}
	}
	return nil
}

// statusOf reports the runtime status of an instance.
func statusOf(inst *instance.Instance) (instance.Status, int, error) {
	return newProcFor(inst).Status()
}
