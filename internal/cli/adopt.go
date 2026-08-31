package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/adopt"
	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/prompt"
	"github.com/ahmed/odoonoir/internal/ui"
)

func newAdoptCmd() *cobra.Command {
	var (
		opts         = adopt.Options{}
		flagNoPrompt bool
		flagCheck    bool
		flagScan     bool
		flagScanDir  string
		flagAddons   []string
	)
	cmd := &cobra.Command{
		Use:   "adopt [name]",
		Short: "Import an existing Odoo installation into odoonoir",
		Long: `Imports an already-installed Odoo instance without touching it:

  - nothing is moved, copied or rewritten: odoonoir only records the
    paths in its registry (read-only adoption)
  - the source dir (the one containing odoo-bin), odoo.conf, python
    venv, database, port and addons are auto-detected when possible
  - every database owned by the installation is discovered and tracked
  - instances that follow a non-standard layout can be described with
    flags; anything still unknown is asked in the wizard
  - --scan discovers running odoo processes and common install roots;
    --scan-dir <path> searches inside a specific directory instead
  - --check audits that the adopted instance can actually run

Typical usage:
  odoonoir adopt legacy --source /srv/odoo
  odoonoir adopt legacy --conf /etc/odoo/odoo.conf --db legacy --port 8069
  odoonoir adopt --scan
  odoonoir adopt --scan-dir /srv
  odoonoir adopt --scan-dir ~/workspace/odoo

The existing conf is used as-is for start/stop: its own pidfile and
logfile keys are honored, and no key is ever rewritten.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}
			opts.Addons = flagAddons
			if flagScanDir != "" {
				fi, err := os.Stat(flagScanDir)
				if err != nil {
					return fmt.Errorf("scan path %q: %w", flagScanDir, err)
				}
				if !fi.IsDir() {
					return fmt.Errorf("scan path %q is not a directory", flagScanDir)
				}
				flagScan = true
			}
			if flagScan {
				if flagNoPrompt {
					return fmt.Errorf("--scan requires an interactive terminal")
				}
				if err := adoptScanWizard(&opts, flagScanDir); err != nil {
					return err
				}
			}
			res, err := adopt.Detect(cfg, reg, opts)
			if err != nil && !flagNoPrompt {
				if err := adoptSourceWizard(&opts); err != nil {
					return err
				}
				res, err = adopt.Detect(cfg, reg, opts)
			}
			if err != nil {
				return err
			}
			inst := res.Inst
			if !flagNoPrompt {
				if err := adoptMissingWizard(inst, &opts); err != nil {
					return err
				}
				res, err = adopt.Detect(cfg, reg, opts)
				if err != nil {
					return err
				}
				inst = res.Inst
			}
			if err := validateAdopt(inst); err != nil {
				return err
			}

			fmt.Println(th.Infof("adopting %q", inst.Name))
			kv := [][2]string{
				{"source", inst.SourcePath},
			}
			if inst.ConfPath != "" {
				kv = append(kv, [2]string{"conf", inst.ConfPath})
			} else {
				kv = append(kv, [2]string{"conf", "(none — Odoo defaults)"})
			}
			if inst.VenvPath != "" {
				kv = append(kv, [2]string{"venv", inst.VenvPath})
			}
			if inst.PythonBin != "" {
				kv = append(kv, [2]string{"python", inst.PythonBin})
			}
			kv = append(kv,
				[2]string{"version", displayOr(inst.Version, "?")},
			)
			if inst.Branch != "" {
				kv = append(kv, [2]string{"branch", inst.Branch})
			}
			kv = append(kv,
				[2]string{"port", fmt.Sprint(inst.Port)},
				[2]string{"database", fmt.Sprintf("%s (user %s)", displayOr(inst.DBName, "?"), inst.DBUser)},
			)
			if len(res.DBs) > 0 {
				kv = append(kv, [2]string{"databases", ""})
			}
			for _, d := range res.DBs {
				mark := " "
				if d.Primary {
					mark = "*"
				}
				init := "not initialized"
				if d.Init {
					init = "initialized"
				}
				size := "?"
				if d.Size > 0 {
					size = humanSizeBytes(d.Size)
				}
				kv = append(kv, [2]string{"", fmt.Sprintf("%s%s  %s  %s", mark, d.Name, size, init)})
			}
			if len(res.DBs) == 0 && inst.DBName == "" {
				kv = append(kv, [2]string{"db", "(none found on the server)"})
			}
			if len(inst.AddonsPaths) > 0 {
				kv = append(kv, [2]string{"addons", strings.Join(inst.AddonsPaths, ", ")})
			}
			if inst.LogPath != "" {
				kv = append(kv, [2]string{"log", inst.LogPath})
			}
			if inst.PIDPath != "" {
				kv = append(kv, [2]string{"pidfile", inst.PIDPath})
			}
			if res.Running > 0 {
				kv = append(kv, [2]string{"running", fmt.Sprintf("yes (pid %d)", res.Running)})
			}
			fmt.Println(th.Panel("", th.KV(kv)))
			if flagCheck {
				p := ui.NewProgress(cmd.OutOrStdout(), "auditing adopted installation")
				paths := inst.ResolvePaths(instRoot(inst))
				for _, iss := range adopt.Audit(cfg, inst, paths) {
					mark := th.Success.Render(ui.CheckMark)
					if !iss.OK {
						mark = th.Error.Render(ui.CrossMark)
					}
					line := mark + " " + iss.Check
					if !iss.OK && iss.Hint != "" {
						line += th.Hintf(" — %s", iss.Hint)
					}
					p.Line(line)
				}
				p.Done()
			}
			for _, w := range res.Warn {
				fmt.Println(th.Warningf("%s", w))
			}
			if !flagNoPrompt {
				ok, err := prompt.Confirm("adopt this instance as described?", true)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("adopt aborted")
				}
			}
			if err := reg.Put(inst); err != nil {
				return err
			}
			fmt.Println(th.Successf("instance %q adopted", inst.Name))
			fmt.Println(th.Hintf("next steps:"))
			fmt.Println(th.Hintf("  odoonoir status %s   — runtime state", inst.Name))
			fmt.Println(th.Hintf("  odoonoir dash        — manage it from the dashboard"))
			fmt.Println(th.Hintf("  odoonoir update %s   — pull source + reinstall requirements", inst.Name))
			fmt.Println(th.Hintf("  odoonoir logs %s     — read its log", inst.Name))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.Source, "source", "", "dir that contains (or nests) odoo-bin")
	f.StringVar(&opts.Conf, "conf", "", "path to the existing odoo.conf")
	f.StringVar(&opts.Venv, "venv", "", "python venv dir (auto-detected when possible)")
	f.StringVar(&opts.Python, "python", "", "python interpreter for start/update (used when no venv)")
	f.StringSliceVar(&flagAddons, "addons", nil, "extra addons dirs (comma separated); defaults to the conf addons_path")
	f.StringVar(&opts.DBName, "db", "", "database name (default: conf db_name)")
	f.StringVar(&opts.DBUser, "db-user", "", "postgres role (default: conf db_user or odoo)")
	f.IntVar(&opts.Port, "port", 0, "HTTP port (default: conf http_port)")
	f.StringVar(&opts.Version, "version", "", "Odoo version, e.g. 16.0, 18.0 (auto-detected from release.py/git)")
	f.StringVar(&opts.Description, "description", "", "free-form description")
	f.StringVar(&opts.Root, "root", "", "odoonoir metadata root (default: the config instances dir)")
	f.BoolVar(&flagNoPrompt, "no-prompt", false, "never prompt interactively (flags only)")
	f.BoolVar(&flagCheck, "check", false, "audit the detected installation before adopting")
	f.BoolVar(&flagScan, "scan", false, "discover running odoo processes and common install roots")
	f.StringVar(&flagScanDir, "scan-dir", "", "search inside this directory for Odoo installs (implies --scan)")
	return cmd
}

// adoptScanWizard lists discovered Odoo installations and lets the user
// pick one to adopt. scanDir restricts the search to a directory ("" for
// the default scan: running processes + common install roots).
func adoptScanWizard(opts *adopt.Options, scanDir string) error {
	cands := adopt.ScanCandidates(scanDir)
	if len(cands) == 0 {
		return fmt.Errorf("no Odoo installations found — pass --source or --conf explicitly")
	}
	options := make([]huh.Option[int], 0, len(cands))
	for i, c := range cands {
		label := c.Hint
		if c.Source != "" {
			label += " · " + c.Source
		}
		if c.Conf != "" {
			label += " · " + c.Conf
		}
		options = append(options, huh.NewOption(label, i))
	}
	var picks []int
	groups := []*huh.Group{huh.NewGroup(
		huh.NewMultiSelect[int]().
			Title("Odoo installations found").
			Description("pick one to adopt (run --scan again for more)").
			Options(options...).
			Value(&picks),
	)}
	if opts.Name == "" {
		groups = append([]*huh.Group{huh.NewGroup(
			huh.NewInput().
				Title("Instance name").
				Description("how odoonoir will refer to this instance (e.g. legacy, odoo19)").
				Value(&opts.Name).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("a name is required")
					}
					if !adopt.ValidName(s) {
						return fmt.Errorf("use lowercase letters, digits and underscores")
					}
					return nil
				}),
		)}, groups...)
	}
	if err := huh.NewForm(groups...).WithKeyMap(huhCompatKeyMap()).Run(); err != nil {
		return err
	}
	if len(picks) == 0 {
		return fmt.Errorf("no installation selected")
	}
	c := cands[picks[0]]
	if c.Source != "" {
		opts.Source = c.Source
	}
	if c.Conf != "" {
		opts.Conf = c.Conf
	}
	if c.Port != 0 {
		opts.Port = c.Port
	}
	if c.Python != "" {
		opts.Python = c.Python
	}
	return nil
}

// adoptSourceWizard asks for the location of the source when detection
// failed outright (no --source, no --conf).
func adoptSourceWizard(opts *adopt.Options) error {
	home := os.Getenv("HOME")
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Odoo source directory").
			Description("the directory that contains odoo-bin (or an odoo/ subdir with it)").
			Value(&opts.Source).
			Suggestions([]string{filepath.Join(home, "odoo"), "/srv/odoo", "/opt/odoo"}).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("the source directory is required")
				}
				for _, bin := range []string{
					filepath.Join(s, "odoo-bin"),
					filepath.Join(s, "odoo", "odoo-bin"),
				} {
					if fi, err := os.Stat(bin); err == nil && !fi.IsDir() {
						return nil
					}
				}
				return fmt.Errorf("no odoo-bin found there — try the directory that contains it")
			}),
		huh.NewInput().
			Title("odoo.conf path").
			Description("optional — auto-detected; leave empty if there is none").
			Value(&opts.Conf).
			Suggestions([]string{"/etc/odoo/odoo.conf", filepath.Join(home, "odoo", "etc", "odoo.conf")}),
	))
	return form.WithKeyMap(huhCompatKeyMap()).Run()
}

// adoptMissingWizard asks for the facts that detection could not resolve.
// Plain text prompts are used (not a second huh form) so that the whole
// adopt flow only ever runs one interactive form — sequential huh forms
// are unreliable in a single process.
func adoptMissingWizard(inst *instance.Instance, opts *adopt.Options) error {
	if inst.Version == "" {
		version, err := prompt.Ask("Odoo version could not be detected (16.0/17.0/18.0/19.0)", "18.0")
		if err != nil {
			return err
		}
		opts.Version = version
	}
	if inst.DBName == "" {
		name, err := prompt.Ask("Database name (the postgres database this instance uses)", "")
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("a database name is required")
		}
		if err := db.IsValidName(name); err != nil {
			return err
		}
		opts.DBName = name
	}
	if inst.Port == 0 {
		port, err := prompt.AskInt("HTTP port (the port this instance listens on)", 8069)
		if err != nil {
			return err
		}
		opts.Port = port
	}
	if inst.PythonBin == "" {
		py, err := prompt.Ask("Python interpreter for start/update (empty to skip)", "")
		if err != nil {
			return err
		}
		opts.Python = py
	}
	return nil
}

// validateAdopt performs final consistency checks before registration.
func validateAdopt(inst *instance.Instance) error {
	if inst.Port == 0 {
		return fmt.Errorf("the instance port is unknown — pass --port or rerun without --no-prompt")
	}
	if inst.DBName == "" {
		return fmt.Errorf("the instance database is unknown — pass --db or rerun without --no-prompt")
	}
	if inst.Version == "" {
		return fmt.Errorf("the instance version is unknown — pass --version or rerun without --no-prompt")
	}
	all, err := reg.All()
	if err != nil {
		return err
	}
	for _, other := range all {
		if other.Name != inst.Name && other.Port == inst.Port {
			return fmt.Errorf("port %d is already used by instance %q", inst.Port, other.Name)
		}
		if other.Name == inst.Name {
			return fmt.Errorf("instance %q already exists", inst.Name)
		}
	}
	if inst.SourcePath == "" {
		return fmt.Errorf("no source directory resolved — pass --source")
	}
	return nil
}

func displayOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
