package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/instance"
)

// newEditCmd returns `odoonoir edit` — updates the tracked metadata of an
// instance (adopted or created): conf, log, pidfile, venv, python, source,
// port, database, description. Empty values clear an override, falling
// back to the default layout.
func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Update instance metadata (conf, log, ports, …)",
		Long: `Updates the tracked metadata of an instance. Adopted instances often
need their conf or log path corrected after the fact; created instances
use a standard layout and rarely need this.

Empty flag values clear a field (an empty conf falls back to
<root>/<name>/etc/odoo.conf). Run without flags to show what is tracked.`,
		Example: `  odoonoir edit myapp --conf /etc/odoo/odoo.conf --log /var/log/odoo/odoo.log
  odoonoir edit myapp --port 8090 --db my_db
  odoonoir edit myapp --description "production billing"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			before := *inst
			flags := cmd.Flags()
			touched := []string{}

			setStr := func(name string, field *string, validate func(string) error) error {
				if !flags.Changed(name) {
					return nil
				}
				v, err := flags.GetString(name)
				if err != nil {
					return err
				}
				if validate != nil {
					if err := validate(v); err != nil {
						return err
					}
				}
				*field = v
				touched = append(touched, name)
				return nil
			}

			if err := setStr("conf", &inst.ConfPath, func(v string) error {
				if v != "" {
					if _, err := os.Stat(v); err != nil {
						return fmt.Errorf("conf file not found: %s", v)
					}
					if fi, err := os.Stat(v); err == nil && fi.IsDir() {
						return fmt.Errorf("conf path is a directory: %s", v)
					}
				}
				return nil
			}); err != nil {
				return err
			}
			if err := setStr("log", &inst.LogPath, nil); err != nil {
				return err
			}
			if err := setStr("pidfile", &inst.PIDPath, nil); err != nil {
				return err
			}
			if err := setStr("venv", &inst.VenvPath, nil); err != nil {
				return err
			}
			if err := setStr("python", &inst.PythonBin, nil); err != nil {
				return err
			}
			if err := setStr("source", &inst.SourcePath, func(v string) error {
				if v != "" {
					if fi, err := os.Stat(v); err != nil || !fi.IsDir() {
						return fmt.Errorf("source path is not a directory: %s", v)
					}
				}
				return nil
			}); err != nil {
				return err
			}
			if err := setStr("db", &inst.DBName, func(v string) error {
				if v != "" {
					return db.IsValidName(v)
				}
				return nil
			}); err != nil {
				return err
			}
			if err := setStr("description", &inst.Description, nil); err != nil {
				return err
			}

			if flags.Changed("port") {
				port, err := flags.GetInt("port")
				if err != nil {
					return err
				}
				if err := registryPortFree(inst, port); err != nil {
					return err
				}
				inst.Port = port
				inst.LongpollPort = port + 3
				touched = append(touched, "port")
			}
			if flags.Changed("version") {
				v, _ := flags.GetString("version")
				if v == "" {
					return fmt.Errorf("empty version is not allowed — pass e.g. --version 19.0")
				}
				if !strings.Contains(v, ".") {
					v += ".0"
				}
				inst.Version = v
				touched = append(touched, "version")
			}

			if len(touched) == 0 {
				printInstanceMeta(inst)
				return nil
			}
			if err := reg.Put(inst); err != nil {
				return err
			}
			fmt.Println(th.Successf("instance %s updated: %s", inst.Name, strings.Join(touched, ", ")))
			printInstanceMeta(inst)
			if containsAny(touched, "conf", "log", "port") {
				fmt.Println(th.Hintf("restart the instance for changes to apply: odoonoir restart %s", inst.Name))
			}
			_ = before
			return nil
		},
	}
	cmd.Flags().String("conf", "", "path to the odoo.conf (empty clears the override)")
	cmd.Flags().String("log", "", "log file path (empty clears)")
	cmd.Flags().String("pidfile", "", "pid file path (empty clears)")
	cmd.Flags().String("venv", "", "python venv directory (empty clears)")
	cmd.Flags().String("python", "", "python interpreter for start/update (empty clears)")
	cmd.Flags().String("source", "", "directory containing odoo-bin (empty clears)")
	cmd.Flags().Int("port", 0, "http port (also sets longpoll = port + 3)")
	cmd.Flags().String("db", "", "primary database name")
	cmd.Flags().String("version", "", "odoo version (e.g. 19.0)")
	cmd.Flags().String("description", "", "free-form description (empty clears)")
	return cmd
}

func containsAny(touched []string, names ...string) bool {
	for _, t := range touched {
		for _, n := range names {
			if t == n {
				return true
			}
		}
	}
	return false
}

// printInstanceMeta renders the tracked metadata of an instance.
func printInstanceMeta(inst *instance.Instance) {
	p := inst.ResolvePaths(instRoot(inst))
	rows := [][2]string{
		{"version", inst.Version},
		{"port", fmt.Sprint(inst.Port)},
		{"database", inst.DBName},
		{"source", p.Source},
		{"conf", p.Conf},
	}
	if inst.VenvPath != "" {
		rows = append(rows, [2]string{"venv", p.Venv})
	}
	if inst.PythonBin != "" {
		rows = append(rows, [2]string{"python", inst.PythonBin})
	}
	rows = append(rows,
		[2]string{"log", p.Log},
		[2]string{"pidfile", p.PIDFile},
	)
	if inst.Description != "" {
		rows = append(rows, [2]string{"description", inst.Description})
	}
	if inst.Adopted {
		rows = append(rows, [2]string{"adopted", "yes"})
	}
	fmt.Println(th.Panel("instance "+inst.Name, th.KV(rows)))
}
