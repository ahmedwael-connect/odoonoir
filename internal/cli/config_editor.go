package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/odoconf"
	"github.com/ahmed/odoonoir/internal/tui"
)

// loadInstanceConf loads the instance and its conf document.
func loadInstanceConf(name string) (*odoconf.OdooConf, error) {
	inst, err := loadInstance(name)
	if err != nil {
		return nil, err
	}
	return odoconf.Load(inst.ResolvePaths(instRoot(inst)).Conf)
}

// confRestartHint reminds that the running process must be restarted.
func confRestartHint(name string) {
	fmt.Println(th.Hintf("restart the instance for changes to apply: odoonoir restart %s", name))
}

// mutateConf runs fn on the loaded conf, saves and reports.
func mutateConf(name, what string, fn func(c *odoconf.OdooConf) error) error {
	c, err := loadInstanceConf(name)
	if err != nil {
		return err
	}
	if err := fn(c); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Println(th.Successf("%s", what))
	confRestartHint(name)
	return nil
}

func newConfigCommentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "comment <name> <key>",
		Short: "Disable a conf option, keeping its value (# key = value)",
		Long: `Comments the option out in place — the line becomes "# key = value",
so the value is kept and the option can be re-enabled later with
` + "`odoonoir config uncomment`" + `. Safer than unset for options like
workers or logfile.`,
		Example: `  odoonoir config comment myapp workers
  odoonoir config comment myapp logfile`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateConf(args[0], "commented "+args[1], func(c *odoconf.OdooConf) error {
				return c.Comment(args[1])
			})
		},
	}
}

func newConfigUncommentCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "uncomment <name> <key>",
		Short:   "Re-enable a commented conf option",
		Example: `  odoonoir config uncomment myapp workers`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateConf(args[0], "uncommented "+args[1], func(c *odoconf.OdooConf) error {
				return c.Uncomment(args[1])
			})
		},
	}
}

// printAddons lists addons_path entries 1-based (1 = highest priority).
func printAddons(c *odoconf.OdooConf) error {
	paths, err := c.AddonsPath()
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		fmt.Println(th.Warningf("addons_path is empty"))
		return nil
	}
	rows := make([][]string, 0, len(paths))
	for i, p := range paths {
		rows = append(rows, []string{fmt.Sprint(i + 1), p})
	}
	fmt.Println(th.Table([]string{"#", "PATH"}, rows))
	fmt.Println(th.Hintf("first = highest priority (Odoo resolves modules first-match-wins)"))
	return nil
}

// mutateAddons loads the conf, applies fn to the addons_path list, saves
// and prints the new list plus the restart hint.
func mutateAddons(name string, fn func(c *odoconf.OdooConf) error) error {
	c, err := loadInstanceConf(name)
	if err != nil {
		return err
	}
	if err := fn(c); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Println(th.Infof("addons_path of %s updated", name))
	if err := printAddons(c); err != nil {
		return err
	}
	confRestartHint(name)
	return nil
}

func newConfigAddonsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addons",
		Short: "Edit the addons_path list of an instance",
		Long: `addons_path is a comma-separated list where ORDER MATTERS: Odoo
resolves a module from the first directory that contains it. Subcommands:
list, add, remove, move, up, down. Use --edit for an interactive editor
that reorders with the arrow keys.`,
	}
	cmd.AddCommand(
		newConfigAddonsListCmd(),
		newConfigAddonsAddCmd(),
		newConfigAddonsRemoveCmd(),
		newConfigAddonsMoveCmd(),
		newConfigAddonsUpCmd(),
		newConfigAddonsDownCmd(),
	)
	return cmd
}

func newConfigAddonsListCmd() *cobra.Command {
	var edit bool
	cmd := &cobra.Command{
		Use:   "list <name>",
		Short: "Show addons_path entries numbered by priority",
		Long: `Shows the addons_path entries in order, numbered from 1 (highest
priority). With --edit, opens an interactive editor: arrow keys to
navigate, [u]/[d] to move a path, [a] to add, [e] to edit, [x] to
remove, [q] to save and quit.`,
		Example: `  odoonoir config addons list myapp
  odoonoir config addons list myapp --edit`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if edit {
				return runAddonsEditor(args[0])
			}
			c, err := loadInstanceConf(args[0])
			if err != nil {
				return err
			}
			return printAddons(c)
		},
	}
	cmd.Flags().BoolVar(&edit, "edit", false, "open the interactive list editor")
	return cmd
}

func newConfigAddonsAddCmd() *cobra.Command {
	var at int
	cmd := &cobra.Command{
		Use:   "add <name> <path>",
		Short: "Append a path to addons_path (--at <n> to insert at position)",
		Example: `  odoonoir config addons add myapp /srv/mymodules
  odoonoir config addons add myapp /srv/mymodules --at 1`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateAddons(args[0], func(c *odoconf.OdooConf) error {
				return c.AddonsAdd(args[1], at)
			})
		},
	}
	cmd.Flags().IntVar(&at, "at", -1, "insert at position (1-based, 1 = highest priority); default: append")
	return cmd
}

func newConfigAddonsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name> <path>",
		Short:   "Delete a path from addons_path",
		Example: `  odoonoir config addons remove myapp /srv/odoo/odoo/addons`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateAddons(args[0], func(c *odoconf.OdooConf) error {
				return c.AddonsRemove(args[1])
			})
		},
	}
}

func newConfigAddonsMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move <name> <from> <to>",
		Short: "Reorder addons_path (1-based positions)",
		Long: `Moves the entry at position <from> to position <to> (1-based,
1 = highest priority). Positions are clamped to the list bounds.`,
		Example: `  odoonoir config addons move myapp 3 1`,
		Args:    cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid position %q", args[1])
			}
			to, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("invalid position %q", args[2])
			}
			return mutateAddons(args[0], func(c *odoconf.OdooConf) error {
				return c.AddonsMove(from-1, to-1)
			})
		},
	}
}

func newConfigAddonsUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "up <name> <n>",
		Short:   "Move the entry at position <n> one step up (higher priority)",
		Example: `  odoonoir config addons up myapp 3`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid position %q", args[1])
			}
			return mutateAddons(args[0], func(c *odoconf.OdooConf) error {
				return c.AddonsMove(n-1, n-2)
			})
		},
	}
}

func newConfigAddonsDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "down <name> <n>",
		Short:   "Move the entry at position <n> one step down (lower priority)",
		Example: `  odoonoir config addons down myapp 1`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid position %q", args[1])
			}
			return mutateAddons(args[0], func(c *odoconf.OdooConf) error {
				return c.AddonsMove(n-1, n)
			})
		},
	}
}

// runAddonsEditor opens the interactive addons_path editor.
func runAddonsEditor(name string) error {
	if !isInteractive() {
		return fmt.Errorf("the interactive editor requires a terminal — use: odoonoir config addons list %s", name)
	}
	c, err := loadInstanceConf(name)
	if err != nil {
		return err
	}
	paths, err := c.AddonsPath()
	if err != nil {
		return err
	}
	res, err := tui.RunAddonsEditor(tui.AddonsEditorDeps{
		Instance: name,
		Paths:    paths,
		Save: func(paths []string) error {
			c.SetAddonsPath(paths)
			return c.Save()
		},
	})
	if err != nil {
		return err
	}
	if res.Saved {
		fmt.Printf("addons_path of %s updated (%d paths):\n", name, len(paths))
		printAddons(c)
		confRestartHint(name)
	}
	return nil
}
