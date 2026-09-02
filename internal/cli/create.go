package cli

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/checker"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/notify"
	"github.com/ahmed/odoonoir/internal/proc"
	"github.com/ahmed/odoonoir/internal/service"
	"github.com/ahmed/odoonoir/internal/ui"
)

// createOpts collects everything needed to install an instance.
type createOpts struct {
	Name        string
	Description string
	Version     string
	Port        int
	Root        string
	DBUser      string
	DBPass      string
	DBName      string
	Workers     int
	LogLevel    string
	Dev         bool
	Source      string
	GitRef      string
	Python      string
	Extra       map[string]string

	// wizard-only string bindings (parsed into the int fields after the form)
	PortVal    string
	WorkersVal string
}

func newCreateCmd() *cobra.Command {
	var (
		opts         = createOpts{Workers: 2, LogLevel: "info"}
		flagNoCheck  bool
		flagNoStart  bool
		flagNoPrompt bool
		flagForce    bool
		flagSet      []string
	)
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Install a complete Odoo instance from scratch",
		Long: `Installs a self-contained Odoo instance:
  1. audits the system (unless --no-check)
  2. verifies PostgreSQL and the odoo db role
  3. clones the Odoo source pinned to the version branch
  4. creates a python venv and installs requirements
  5. generates etc/odoo.conf and the log file
  6. scaffolds custom_addons
  7. creates the database, initializes it and starts the instance

Run without flags to use the interactive wizard.

What you will see: every step is printed as it runs (audit, clone, venv,
database, start); the wizard additionally asks you for the name, version,
addons directory and database name.`,
		Example: `  odoonoir create myapp            # guided wizard (asks name/version/db)
  odoonoir create myapp -v 18      # everything automatic, no prompts
  odoonoir create myapp --no-start # install but do not start the instance
  odoonoir create myapp --db-name sale_db  # custom database name`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				opts.Name = args[0]
			}
			if flagSet != nil {
				opts.Extra = map[string]string{}
				for _, kv := range flagSet {
					parts := strings.SplitN(kv, "=", 2)
					if len(parts) != 2 {
						return fmt.Errorf("--set expects key=value, got %q", kv)
					}
					opts.Extra[parts[0]] = parts[1]
				}
			}
			if opts.Version == "" && !flagNoPrompt {
				if err := runCreateWizard(&opts); err != nil {
					return err
				}
				if opts.PortVal != "" {
					p, err := parsePort(opts.PortVal)
					if err != nil {
						return err
					}
					opts.Port = p
				}
				if opts.WorkersVal != "" {
					w, err := parseWorkers(opts.WorkersVal)
					if err != nil {
						return err
					}
					opts.Workers = w
				}
			}
			if opts.Name == "" {
				return fmt.Errorf("instance name is required — pass <name> or use the wizard")
			}
			if err := db.IsValidName(opts.Name); err != nil {
				return err
			}
			if _, err := reg.Get(opts.Name); err == nil {
				return fmt.Errorf("instance %q already exists", opts.Name)
			}
			if opts.Version == "" {
				return fmt.Errorf("select an Odoo version with -v (15, 16, 17, 18 or 19)")
			}
			if !strings.Contains(opts.Version, ".") {
				opts.Version += ".0"
			}
			major := strings.Split(opts.Version, ".")[0]
			if _, ok := checker.PythonCompat[major]; !ok {
				return fmt.Errorf("unsupported Odoo version %q (supported: 15, 16, 17, 18, 19)", major)
			}

			if !flagNoCheck {
				p := ui.NewProgress(cmd.OutOrStdout(), "auditing system requirements")
				chk := checker.Run()
				chk.ForVersion(opts.Version)
				failed := false
				for _, r := range chk.Results {
					mark := th.Muted.Render("·")
					switch r.Severity {
					case checker.OK:
						mark = th.Success.Render(ui.CheckMark)
					case checker.WARN:
						mark = th.Warning.Render(ui.WarnMark)
					case checker.ERROR:
						mark = th.Error.Render(ui.CrossMark)
						failed = true
					case checker.MISSING:
						mark = th.Warning.Render(ui.WarnMark)
					}
					line := mark + " " + r.Check
					if r.Severity == checker.ERROR || r.Severity == checker.MISSING {
						if r.Hint != "" {
							line += th.Hintf(" — %s", r.Hint)
						}
					}
					p.Line(line)
				}
				if failed {
					p.Fail()
					fmt.Println(th.Hintf("run `odoonoir check` for details and fixes"))
					return fmt.Errorf("system requirements not met")
				}
				p.Done()
			}

			out := cmd.OutOrStdout()
			fmt.Println(th.Infof("resolving storage root"))
			root := opts.Root
			if root == "" {
				root = cfg.InstancesDir()
			}
			if _, err := os.Stat(root); os.IsNotExist(err) {
				if err := os.MkdirAll(root, 0o755); err != nil {
					return fmt.Errorf("create instances root %s: %w", root, err)
				}
			}
			instPath := filepath.Join(root, opts.Name)

			// The engine handles all pre-flight checks, the install pipeline,
			// postgres role/database creation, registry tracking and rollback
			// on failure — identical to this command's previous inline flow.
			var prog *ui.Progress
			res, err := svc.Create(cmd.Context(), service.CreateOptions{
				Name:        opts.Name,
				Version:     opts.Version,
				GitRef:      opts.GitRef,
				Source:      opts.Source,
				Python:      opts.Python,
				Port:        opts.Port,
				DBName:      opts.DBName,
				DBUser:      opts.DBUser,
				DBPass:      opts.DBPass,
				Root:        opts.Root,
				Description: opts.Description,
				Workers:     opts.Workers,
				LogLevel:    opts.LogLevel,
				Dev:         opts.Dev,
				Force:       flagForce,
				InitDB:      !flagNoStart,
				Extra:       opts.Extra,
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
				return err
			}
			inst := res.Instance
			dbName := res.Database
			port := res.Port
			dbUser := opts.DBUser
			if dbUser == "" {
				dbUser = "odoo"
			}

			fmt.Println(th.Successf("instance %q created", inst.Name))
			fmt.Println(th.Panel("", th.KV([][2]string{
				{"version", opts.Version + " (branch " + inst.Branch + ")"},
				{"database", dbName + " (owner " + dbUser + ")"},
				{"url", fmt.Sprintf("http://localhost:%d", port)},
				{"source", instPath},
				{"conf", filepath.Join(instPath, "etc", "odoo.conf")},
				{"custom addons", filepath.Join(instPath, "custom_addons")},
			})))

			if flagNoStart {
				fmt.Println(th.Hintf("not started (--no-start) — start it with: odoonoir start %s", inst.Name))
				return nil
			}
			fmt.Println(th.Infof("starting instance"))
			return startInstance(inst)
		},
	}
	cmd.Flags().StringVarP(&opts.Version, "version", "v", "", "Odoo major version to install (15, 16, 17, 18, 19)")
	cmd.Flags().IntVarP(&opts.Port, "port", "p", 0, "HTTP port (default: first free port from 8069)")
	cmd.Flags().StringVar(&opts.Root, "root", "", "storage location for this instance (tracked in the registry)")
	cmd.Flags().StringVar(&opts.DBUser, "db-user", "", "postgres role for odoo (default: odoo)")
	cmd.Flags().StringVar(&opts.DBPass, "db-pass", "", "password for the odoo role (default: same as the role)")
	cmd.Flags().StringVar(&opts.DBName, "db-name", "", "database name (default: same as instance name)")
	cmd.Flags().IntVar(&opts.Workers, "workers", 2, "number of odoo workers")
	cmd.Flags().StringVar(&opts.LogLevel, "log-level", "info", "log level: debug, info, warning, error")
	cmd.Flags().StringVar(&opts.GitRef, "git-ref", "", "git tag/commit to pin the source (default: version branch)")
	cmd.Flags().StringVar(&opts.Python, "python", "", "python interpreter for the venv (default: best match for the version)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "free-form description for the instance")
	cmd.Flags().StringArrayVar(&flagSet, "set", nil, "extra odoo.conf key=value (repeatable: --set limit_time_cpu=60)")
	cmd.Flags().BoolVar(&opts.Dev, "dev", false, "development mode: dev_mode=all, debug logging")
	cmd.Flags().BoolVar(&flagNoCheck, "no-check", false, "skip the system audit")
	cmd.Flags().BoolVar(&flagNoStart, "no-start", false, "do not start the instance after install")
	cmd.Flags().BoolVar(&flagNoPrompt, "no-prompt", false, "never prompt interactively (flags only)")
	cmd.Flags().BoolVar(&flagForce, "force", false, "remove an existing instance folder before installing")
	cmd.Flags().StringVar(&opts.Source, "source", "", "custom git repo URL for Odoo source (default: github.com/odoo/odoo)")
	return cmd
}

// runCreateWizard collects install options interactively.
func runCreateWizard(opts *createOpts) error {
	if opts.Port == 0 {
		if p, err := nextFreePort(8069); err == nil {
			opts.PortVal = fmt.Sprint(p)
		}
	} else {
		opts.PortVal = fmt.Sprint(opts.Port)
	}
	opts.WorkersVal = fmt.Sprint(opts.Workers)
	versions := []huh.Option[string]{
		huh.NewOption("Odoo 19 (latest)", "19.0"),
		huh.NewOption("Odoo 18 (stable)", "18.0"),
		huh.NewOption("Odoo 17 (LTS)", "17.0"),
		huh.NewOption("Odoo 16 (legacy)", "16.0"),
		huh.NewOption("Odoo 15 (legacy)", "15.0"),
	}
	groupBasics := huh.NewGroup(
		huh.NewInput().
			Title("Instance name").
			Description("lowercase, digits and underscores only").
			Value(&opts.Name).
			Validate(func(s string) error { return db.IsValidName(s) }),
		huh.NewInput().
			Title("Description").
			Description("what is this instance for? (optional)").
			Value(&opts.Description),
		huh.NewSelect[string]().
			Title("Odoo version").
			Options(versions...).
			Value(&opts.Version),
	)
	groupSettings := huh.NewGroup(
		huh.NewInput().
			Title("HTTP port").
			Description("suggested free port; change it if you prefer").
			Value(&opts.PortVal).
			Validate(func(s string) error {
				if s == "" {
					return nil
				}
				p, err := parsePort(s)
				if err != nil {
					return err
				}
				if checker.PortInUse(p) {
					return fmt.Errorf("port %d is already in use", p)
				}
				return nil
			}),
		huh.NewInput().
			Title("Location (instance root)").
			Description("where the instance folder lives — tracked per instance").
			Value(&opts.Root).
			Suggestions([]string{cfg.InstancesDir(), filepath.Join(os.Getenv("HOME"), "odoo")}).
			Validate(func(s string) error {
				if s == "" {
					return nil
				}
				if !filepath.IsAbs(s) {
					return fmt.Errorf("use an absolute path")
				}
				return nil
			}),
		huh.NewInput().
			Title("Database role (postgres user for odoo)").
			Description("created if missing; password defaults to the role name").
			Value(&opts.DBUser).
			Validate(func(s string) error {
				if s == "" {
					return nil
				}
				return db.IsValidName(s)
			}),
		huh.NewInput().
			Title("Database name").
			Description("leave empty to use the instance name").
			Value(&opts.DBName).
			Validate(func(s string) error {
				if s == "" {
					return nil
				}
				return db.IsValidName(s)
			}),
	)
	groupAdvanced := huh.NewGroup(
		huh.NewInput().
			Title("Workers").
			Description("number of odoo worker processes").
			Value(&opts.WorkersVal).
			Validate(func(s string) error {
				if s == "" {
					return nil
				}
				_, err := parseWorkers(s)
				return err
			}),
		huh.NewSelect[string]().
			Title("Log level").
			Options(huh.NewOption("info (default)", "info"),
				huh.NewOption("debug (verbose)", "debug"),
				huh.NewOption("warning", "warning"),
				huh.NewOption("error", "error")).
			Value(&opts.LogLevel),
		huh.NewConfirm().
			Title("Development mode?").
			Description("dev_mode=all with debug logging — for module development").
			Value(&opts.Dev),
	)
	groupConfirm := huh.NewGroup(
		huh.NewNote().
			Title("Ready to install").
			Description(func() string {
				return fmt.Sprintf(
					"instance:   %s\nversion:    %s\nport:       %d\nlocation:   %s\ndatabase:   %s (role %s)\nworkers:    %d\nlog level:  %s\ndev mode:   %v",
					opts.Name, opts.Version, opts.Port, rootOr(opts.Root, cfg.InstancesDir()),
					dbNameOr(opts.DBName, opts.Name), dbUserOr(opts.DBUser), opts.Workers, opts.LogLevel, opts.Dev)
			}()),
	)
	form := huh.NewForm(groupBasics, groupSettings, groupAdvanced, groupConfirm).
		WithTheme(huh.ThemeCatppuccin()).
		WithShowHelp(true).
		WithShowErrors(true).
		WithKeyMap(huhCompatKeyMap())
	return form.Run()
}

func parsePort(s string) (int, error) {
	var p int
	if _, err := fmt.Sscanf(s, "%d", &p); err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return p, nil
}

func parseWorkers(s string) (int, error) {
	var w int
	if _, err := fmt.Sscanf(s, "%d", &w); err != nil || w < 1 || w > 64 {
		return 0, fmt.Errorf("invalid workers count %q", s)
	}
	return w, nil
}

func rootOr(root, def string) string {
	if root == "" {
		return def
	}
	return root
}

func dbNameOr(name, def string) string {
	if name == "" {
		return def
	}
	return name
}

func dbUserOr(user string) string {
	if user == "" {
		return "odoo"
	}
	return user
}

func nextFreePort(start int) (int, error) {
	for p := start; p < start+100; p++ {
		// reserve both the HTTP port and its longpoll sibling (port+3)
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		_ = ln.Close()
		ln2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p+3))
		if err != nil {
			continue
		}
		_ = ln2.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free port found from %d", start)
}

func startInstance(inst *instance.Instance) error {
	p := inst.ResolvePaths(instRoot(inst))
	mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
	if err := mgr.Start(""); err != nil {
		return fmt.Errorf("start failed: %w — inspect logs with `odoonoir doctor %s`", err, inst.Name)
	}
	if err := waitForStable(inst, mgr); err != nil {
		return err
	}
	fmt.Println(th.Successf("instance %s is up — http://localhost:%d", inst.Name, inst.Port))
	fmt.Println(th.Hintf("follow logs with: odoonoir logs -f %s", inst.Name))
	return nil
}

// waitForStable checks the process is still alive shortly after start and
// surfaces the log tail if it crashed immediately (e.g. missing module,
// DB auth failure).
func waitForStable(inst *instance.Instance, mgr *proc.Manager) error {
	time.Sleep(4 * time.Second)
	status, _, err := mgr.Status()
	if err != nil {
		return err
	}
	if status == instance.StatusRunning {
		return nil
	}
	lines, tailErr := logmon.Tail(inst.ResolvePaths(instRoot(inst)).Log, 15)
	msg := fmt.Sprintf("instance %s crashed shortly after starting — run `odoonoir doctor %s` for hints", inst.Name, inst.Name)
	if tailErr == nil && len(lines) > 0 {
		msg += ":\n" + strings.Join(lines, "\n")
	}
	if nerr := notify.Notify(cfg, "instance.crash", "instance "+inst.Name+" crashed", "run `odoonoir doctor "+inst.Name+"` for hints"); nerr != nil {
		warn("notify: %v", nerr)
	}
	return fmt.Errorf("%s", msg)
}
