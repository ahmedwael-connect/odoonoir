package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/odoconf"
	"github.com/ahmed/odoonoir/internal/ui"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and edit odoo.conf attributes",
		Long: `Reads and edits the odoo.conf of an instance: db_name, http_port,
addons_path, and any other key. Subcommands: list, get, set, unset,
comment, uncomment, addons, path, global.

Edits are lossless: comments, blank lines and key order are preserved,
and a .bak copy is written before each change.`,
		Example: `  odoonoir config list myapp                    # show the whole odoo.conf
  odoonoir config list myapp --all            # include commented options
  odoonoir config get myapp http_port         # one attribute
  odoonoir config set myapp http_port 8080    # change an attribute
  odoonoir config comment myapp workers       # disable, keep the value
  odoonoir config addons list myapp           # numbered addons_path
  odoonoir config addons list myapp --edit    # interactive editor
  odoonoir config global list                 # odoonoir's own settings`,
	}
	cmd.AddCommand(
		newConfigListCmd(),
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigUnsetCmd(),
		newConfigCommentCmd(),
		newConfigUncommentCmd(),
		newConfigAddonsCmd(),
		newConfigPathCmd(),
		newConfigGlobalCmd(),
	)
	// every subcommand taking an instance name first gets name completion
	for _, sub := range cmd.Commands() {
		if sub.Name() == "global" {
			continue
		}
		sub.ValidArgsFunction = completeInstanceNames
		for _, subsub := range sub.Commands() {
			subsub.ValidArgsFunction = completeInstanceNames
		}
	}
	return cmd
}

// globalKeys are the settings persisted in ~/.odoonoir/config.json.
var globalKeys = map[string]string{
	"instances_root": "where instance folders live",
	"postgres_user":  "postgres admin role for creating databases",
	"postgres_host":  "postgres server host (default: localhost)",
	"postgres_port":  "postgres server port (default: 5432)",
	"odoo_user":      "default database role for new instances",
	"odoo_password":  "default password for the odoo role",
	"webhook_url":    "URL receiving JSON notifications (optional)",
}

func newConfigGlobalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "global",
		Short: "Inspect and edit global odoonoir settings (config.json)",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "Show all global settings",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				keys := make([]string, 0, len(globalKeys))
				for k := range globalKeys {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("%s = %s\n", k, globalValue(k))
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "get <key>",
			Short: "Read one global setting",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if _, ok := globalKeys[args[0]]; !ok {
					return fmt.Errorf("unknown global key %q (valid: %s)", args[0], strings.Join(sortedGlobalKeys(), ", "))
				}
				fmt.Println(globalValue(args[0]))
				return nil
			},
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: "Set a global setting (persisted to config.json)",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				key, value := args[0], args[1]
				if _, ok := globalKeys[key]; !ok {
					return fmt.Errorf("unknown global key %q (valid: %s)", key, strings.Join(sortedGlobalKeys(), ", "))
				}
				if err := setGlobal(key, value); err != nil {
					return err
				}
				fmt.Println(th.Successf("set global %s = %s", key, value))
				return nil
			},
		},
	)
	return cmd
}

func sortedGlobalKeys() []string {
	keys := make([]string, 0, len(globalKeys))
	for k := range globalKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// globalValue reads a global key from the loaded config, masking passwords.
func globalValue(key string) string {
	switch key {
	case "instances_root":
		return cfg.InstancesRoot
	case "postgres_user":
		return cfg.PostgresUser
	case "postgres_host":
		return cfg.PostgresHost
	case "postgres_port":
		return fmt.Sprint(cfg.PostgresPort)
	case "odoo_user":
		return cfg.OdooUser
	case "odoo_password":
		if cfg.OdooPassword == "" {
			return ""
		}
		return "********"
	case "webhook_url":
		return cfg.WebhookURL
	}
	return ""
}

// setGlobal validates and persists one global key, then reloads cfg.
func setGlobal(key, value string) error {
	switch key {
	case "instances_root":
		if value == "" || !filepath.IsAbs(value) {
			return fmt.Errorf("instances_root must be an absolute path")
		}
		cfg.InstancesRoot = value
	case "postgres_user":
		if value == "" {
			return fmt.Errorf("postgres_user cannot be empty")
		}
		cfg.PostgresUser = value
	case "postgres_host":
		cfg.PostgresHost = value
	case "postgres_port":
		p, err := strconv.Atoi(value)
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("postgres_port must be a number between 1 and 65535")
		}
		cfg.PostgresPort = p
	case "odoo_user":
		if err := db.IsValidName(value); err != nil {
			return err
		}
		cfg.OdooUser = value
	case "odoo_password":
		cfg.OdooPassword = value
	case "webhook_url":
		if value != "" && !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return fmt.Errorf("webhook_url must start with http:// or https://")
		}
		cfg.WebhookURL = value
	}
	return config.Save(cfg)
}

func loadConf(instName string) (*odoconf.OdooConf, error) {
	inst, err := loadInstance(instName)
	if err != nil {
		return nil, err
	}
	return odoconf.Load(inst.ResolvePaths(instRoot(inst)).Conf)
}

func newConfigListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list <name>",
		Short: "Show all conf attributes (in file order)",
		Long: `Shows the active conf options in file order. With --all, commented
options are also shown as "# key = value". db_password is masked.`,
		Example: `  odoonoir config list myapp
  odoonoir config list myapp --all`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadConf(args[0])
			if err != nil {
				return err
			}
			if all {
				rows := make([][]string, 0, 16)
				for _, e := range c.AllKeys() {
					v := e.Value
					if e.Name == "db_password" && v != "" {
						v = "********"
					}
					key := e.Name
					if !e.Active {
						key = th.Muted.Render("# " + key)
						v = th.Muted.Render(v)
					}
					rows = append(rows, []string{key, v})
				}
				if len(rows) == 0 {
					fmt.Println(th.Hintf("no options in the conf of %s", args[0]))
					return nil
				}
				fmt.Println(th.Table([]string{"OPTION", "VALUE"}, rows, ui.WithTitle("odoo.conf of "+args[0])))
				return nil
			}
			for _, k := range c.Keys() {
				v, _ := c.Get(k)
				if k == "db_password" && v != "" {
					v = "********"
				}
				fmt.Printf("%s = %s\n", k, v)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include commented options")
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name> <key>",
		Short: "Read a single conf attribute",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadConf(args[0])
			if err != nil {
				return err
			}
			v, ok := c.Get(args[1])
			if !ok {
				return fmt.Errorf("key %q not set", args[1])
			}
			fmt.Println(v)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> <key> <value>",
		Short: "Set a conf attribute (persisted to odoo.conf)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadConf(args[0])
			if err != nil {
				return err
			}
			c.Set(args[1], args[2])
			if err := c.Save(); err != nil {
				return err
			}
			fmt.Println(th.Successf("set %s = %s", args[1], args[2]))
			// keep the registry in sync for keys the rest of the tool reads
			if synced, err := syncRegistryKey(args[0], args[1], args[2]); err != nil {
				return err
			} else if synced {
				fmt.Println(th.Hintf("registry updated (instance %s)", args[0]))
			}
			fmt.Println(th.Hintf("restart the instance for changes to apply: odoonoir restart %s", args[0]))
			return nil
		},
	}
}

func newConfigUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <name> <key>",
		Short: "Remove a conf attribute",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadConf(args[0])
			if err != nil {
				return err
			}
			if _, ok := c.Get(args[1]); !ok {
				return fmt.Errorf("key %q not set", args[1])
			}
			c.Unset(args[1])
			if err := c.Save(); err != nil {
				return err
			}
			fmt.Println(th.Successf("unset %s", args[1]))
			return nil
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <name>",
		Short: "Print the conf file path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			fmt.Println(inst.ResolvePaths(instRoot(inst)).Conf)
			return nil
		},
	}
}

// syncRegistryKey mirrors well-known conf keys into the instance registry so
// list/status/dash stay accurate. Returns whether anything was updated.
func syncRegistryKey(name, key, value string) (bool, error) {
	inst, err := loadInstance(name)
	if err != nil {
		return false, err
	}
	changed := false
	switch key {
	case "http_port":
		if p, err := strconv.Atoi(value); err == nil && p >= 1 && p <= 65535 {
			inst.Port = p
			changed = true
		}
	case "workers":
		if w, err := strconv.Atoi(value); err == nil && w >= 1 && w <= 64 {
			inst.Workers = w
			changed = true
		}
	case "log_level":
		inst.LogLevel = value
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := reg.Put(inst); err != nil {
		return false, err
	}
	return true, nil
}
