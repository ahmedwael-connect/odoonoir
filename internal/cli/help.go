package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/ui"
)

// commandGroups maps help topics -> command names for organized help output.
var commandGroups = []struct {
	Title  string
	Header string
	Cmds   []string
}{
	{"INSTALL", "create, check", []string{"create", "check"}},
	{"RUN", "start, stop, restart, status, list", []string{"start", "stop", "restart", "status", "list"}},
	{"DATABASES", "init, backup, restore, drop", []string{"init", "backup", "restore", "drop"}},
	{"CONFIGURE", "config", []string{"config"}},
	{"DEV", "module (new/list/uninstall), test, update, shell, logs, doctor", []string{"module", "test", "update", "shell", "logs", "doctor"}},
	{"MONITOR", "ps, watch, dash", []string{"ps", "watch", "dash"}},
	{"UTIL", "adopt, edit, remove, clone, rename, completion", []string{"adopt", "edit", "remove", "clone", "rename", "completion"}},
}

// helpExamples groups the most useful real-world invocations by topic.
var helpExamples = []struct {
	Title string
	Rows  [][2]string
}{
	{"install", [][2]string{
		{"odoonoir create myapp -v 18", "one-command full install (auto ports, db, start)"},
		{"odoonoir create myapp", "interactive wizard — same result, guided"},
		{"odoonoir check -v 18", "audit the system before installing"},
	}},
	{"run", [][2]string{
		{"odoonoir start", "pick the instance (and database) interactively"},
		{"odoonoir start myapp", "serve the primary database on http://localhost:8069"},
		{"odoonoir start myapp sales", "serve a specific database (-d sales)"},
		{"odoonoir restart myapp", "apply conf and module changes (keeps the database)"},
		{"odoonoir status myapp", "running state, pid, port and the database served"},
		{"odoonoir list", "status of every instance at a glance"},
		{"odoonoir list myapp", "the databases served by myapp (size, init state)"},
	}},
	{"databases", [][2]string{
		{"odoonoir init myapp sales", "create + initialize a second database (-i base)"},
		{"odoonoir backup myapp", "timestamped dump of the primary db in <root>/backups/"},
		{"odoonoir backup myapp sales", "backup a specific database of the instance"},
		{"odoonoir restore --force myapp db.dump", "drop and restore the primary database from a dump"},
		{"odoonoir restore myapp db.dump sales", "restore into a fresh database named sales"},
		{"odoonoir drop --force myapp sales", "delete a database (instance must be stopped)"},
	}},
	{"dev", [][2]string{
		{"odoonoir module new mymod --instance myapp", "scaffold a full module in custom_addons"},
		{"odoonoir update -i mymod myapp", "install the new module into the primary database"},
		{"odoonoir update -i mymod myapp sales", "install a module into a specific database"},
		{"odoonoir update myapp", "git pull source + reinstall python deps"},
		{"odoonoir shell myapp", "interactive python shell against the primary database"},
		{"odoonoir shell myapp sales", "python shell against the sales database"},
	}},
	{"debug", [][2]string{
		{"odoonoir doctor myapp", "explain errors found in the log"},
		{"odoonoir logs -f myapp", "follow the live log"},
		{"odoonoir dash", "interactive dashboard (logs, update, backup)"},
	}},
	{"manage", [][2]string{
		{"odoonoir config comment myapp workers", "disable an option, keep its value (# key = value)"},
		{"odoonoir config addons list myapp", "addons_path numbered by priority (1 = first match)"},
		{"odoonoir config addons list myapp --edit", "interactive editor: u/d move, a add, x remove, q save"},
		{"odoonoir config addons move myapp 3 1", "reorder addons_path: entry 3 to the top"},
		{"odoonoir adopt --scan", "discover existing Odoo installs and import one (read-only)"},
		{"odoonoir adopt --scan-dir /srv", "search inside a specific directory for installs"},
		{"odoonoir adopt legacy --source /srv/odoo --check", "import with an audit of the installation"},
		{"odoonoir edit legacy --conf /etc/odoo/odoo.conf --log /var/log/odoo.log", "point an instance at its conf and log files"},
		{"odoonoir edit legacy --conf \"\"", "clear the conf override (back to the default layout)"},
		{"odoonoir start --all / stop --all", "start or stop every instance at once"},
		{"odoonoir list --json", "machine-readable instance list (scriptable)"},
		{"odoonoir module new mymod --instance myapp", "scaffold a complete module (models, views, security, tests)"},
		{"odoonoir module list myapp --installed", "what is installed in a database (ir_module_module)"},
		{"odoonoir test myapp -m mymod", "run the module's tests (odoo --test-enable)"},
		{"odoonoir shell myapp -c \"print(env['res.partner'].search_count([]))\"", "one-liner against the odoo shell"},
		{"odoonoir ps", "process overview: pid, uptime, memory"},
		{"odoonoir watch myapp", "live status + colored log tail (q to quit)"},
		{"odoonoir clone myapp myapp2 --dbs", "copy an instance incl. additional databases"},
		{"odoonoir rename myapp myapp2", "move an instance; the database name is kept"},
		{"odoonoir remove --force myapp2", "delete an instance (folder + database)"},
	}},
}

// attachHelpTemplate replaces the default root help with a grouped,
// colorized overview. Subcommand --help uses cobra's rich layout (Long,
// examples, flags) so every command documents itself.
func attachHelpTemplate(cmd *cobra.Command) {
	root := cmd.Root()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c != root {
			// Standard cobra layout: description, usage, examples, flags.
			// (c.Help() must not be called here — it routes back through
			// this help func, and subcommands inherit it from the root.)
			if c.Long != "" {
				fmt.Fprintln(c.OutOrStdout(), c.Long)
				fmt.Fprintln(c.OutOrStdout())
			}
			fmt.Fprint(c.OutOrStdout(), c.UsageString())
			return
		}
		printGroupedHelp(c)
	})
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Show help for a command",
		Args:  cobra.MaximumNArgs(1),
		Run: func(c *cobra.Command, args []string) {
			target := root
			if len(args) > 0 {
				if sub, _, err := root.Find(args); err == nil {
					target = sub
				}
			}
			_ = target.Help()
		},
	})
}

func printGroupedHelp(c *cobra.Command) {
	theme := ui.DefaultTheme()
	out := c.OutOrStdout()
	write := func(s string) { _, _ = fmt.Fprintln(out, s) }
	col := func(cmd, desc string) string {
		return "    " + theme.Accent.Render(cmd) + "  " + theme.Muted.Render(desc)
	}

	write("")
	write(theme.Header.Render("odoo noir  —  Odoo instance manager  " + theme.Muted.Render("v"+Version)))
	write(theme.Muted.Render("install, run and develop Odoo 17/18/19 like a pro"))
	write("")
	write("USAGE")
	write(theme.Muted.Render("  odoonoir <command> [options]"))
	write(theme.Muted.Render("  odoonoir help <command>    — detailed help and flags for one command"))
	write("")
	write(theme.Muted.Render("  every command works on an instance; database commands also take a"))
	write(theme.Muted.Render("  database name:  odoonoir start myapp sales    (instance + database)"))
	write(theme.Muted.Render("  run a command without the instance name to pick it interactively"))
	write("")
	write("COMMANDS")

	known := map[string]*cobra.Command{}
	for _, sub := range c.Commands() {
		known[sub.Name()] = sub
	}
	for _, g := range commandGroups {
		write("  " + theme.Accent.Render(g.Title) + "   " + theme.Muted.Render(g.Header))
		for _, name := range g.Cmds {
			sub, ok := known[name]
			if !ok {
				continue
			}
			write("    " + theme.Primary.Render(name) + "    " + theme.Muted.Render(sub.Short))
		}
	}
	write("")
	write("EXAMPLES")
	for _, g := range helpExamples {
		write("  " + theme.Accent.Render(g.Title))
		for _, row := range g.Rows {
			write(col(row[0], row[1]))
		}
	}
	write("")
	write("GLOBAL FLAGS")
	write(theme.Muted.Render("  -h, --help       help for this command or any subcommand"))
	write(theme.Muted.Render("      --version    print the odoonoir version"))
	write(theme.Muted.Render("      --no-color   disable colored output (NO_COLOR env is also honored)"))
	write("")
	if dir, err := config.AppDir(); err == nil {
		write(theme.Muted.Render("  config: " + filepath.Join(dir, "config.json") + "   (or $ODOONOIR_HOME)"))
		write("")
	}
}
