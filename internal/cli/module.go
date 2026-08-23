package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/db"
	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/odoomod"
	"github.com/ahmed/odoonoir/internal/prompt"
	"github.com/ahmed/odoonoir/internal/ui"
)

var commonDeps = []string{
	"base", "mail", "sale_management", "purchase", "stock", "account",
	"product", "contacts", "website", "web", "crm", "project", "hr",
	"point_of_sale", "delivery", "l10n_generic_coa", "decimal_precision",
	"analytic", "snippets", "auth_signup", "portal",
}

var fieldTypes = []string{
	"char", "text", "html", "boolean", "integer", "float", "monetary",
	"date", "datetime", "selection", "many2one", "one2many", "many2many",
}

var licenses = []string{"LGPL-3", "MIT", "AGPL-3", "OPL-1"}

func newModuleCmd() *cobra.Command {
	var (
		flagInstance string
		flagDir      string
		flagNoPrompt bool
	)
	cmd := &cobra.Command{
		Use:   "module",
		Short: "Develop Odoo modules: scaffold, list, uninstall",
	}
	newCmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Interactively scaffold a complete Odoo module",
		Long: `Generates a production-ready module skeleton (manifest, models,
views, security, menus, wizard, tests) with guided prompts.
Targets an instance's custom_addons via --instance, or a directory with --dir.

What you will see: a series of prompts (module summary, license, model
name, fields); then the generated files are listed.`,
		Example: `  odoonoir module new mymod --instance myapp   # scaffold in myapp's custom_addons
  odoonoir module new mymod --dir ./addons     # scaffold into a directory
  odoonoir module new mymod --instance myapp --no-prompt  # all defaults`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := odoomod.ValidName(name); err != nil {
				return err
			}
			targetDir := flagDir
			major := "17"
			if flagInstance != "" {
				inst, err := loadInstance(flagInstance)
				if err != nil {
					return err
				}
				targetDir = inst.ResolvePaths(instRoot(inst)).Addons
				major = strings.Split(inst.Version, ".")[0]
			}
			if targetDir == "" {
				return fmt.Errorf("specify --instance <name> or --dir <path> for the module target")
			}

			m := &odoomod.Module{Name: name}
			if !flagNoPrompt {
				if err := runModuleWizard(m, major, name); err != nil {
					return err
				}
			} else {
				m.DisplayName = strings.ReplaceAll(name, "_", " ")
				m.Summary = "Adds " + m.DisplayName
				m.License = "LGPL-3"
				m.Version = major + ".0.1.0.0"
				m.Category = "Uncategorized"
				m.Depends = []string{"base", "mail"}
				m.Menus = true
				m.Wizard = false
				m.Tests = true
				m.Models = []odoomod.Model{{Name: name + ".item", Label: strings.Title(m.DisplayName), Fields: []odoomod.Field{{Name: "name", Label: "Name", Type: "char", Required: true}}}}
			}
			if len(m.Models) == 0 {
				return fmt.Errorf("module must contain at least one model")
			}
			odoomod.SortModels(m.Models)
			m.AddonsDir = targetDir
			if err := odoomod.Scaffold(m); err != nil {
				return err
			}
			fmt.Println(th.Successf("module %q scaffolded in %s", name, targetDir+"/"+name))
			fmt.Println(th.Hintf("next steps"))
			if flagInstance != "" {
				fmt.Println(th.Hintf("  1. restart the instance:  odoonoir restart %s", flagInstance))
				fmt.Println(th.Hintf("  2. install the module:    odoonoir update -i %s %s", name, flagInstance))
			} else {
				fmt.Println(th.Hintf("  1. add %s to an instance's addons_path", targetDir+"/"+name))
				fmt.Println(th.Hintf("  2. install the module:    odoonoir update -i %s <instance>", name))
			}
			return nil
		},
	}
	newCmd.Flags().StringVar(&flagInstance, "instance", "", "target instance (module lands in its custom_addons)")
	newCmd.Flags().StringVar(&flagDir, "dir", "", "target addons directory (alternative to --instance)")
	newCmd.Flags().BoolVar(&flagNoPrompt, "no-prompt", false, "scaffold with sensible defaults, one model, no prompts")
	cmd.AddCommand(newCmd)
	cmd.AddCommand(newModuleListCmd())
	cmd.AddCommand(newModuleUninstallCmd())
	return cmd
}

// newModuleListCmd queries ir_module_module for the installed/uninstalled
// modules of a database.
func newModuleListCmd() *cobra.Command {
	var flagInstalled bool
	var flagUninstalled bool
	cmd := &cobra.Command{
		Use:   "list <name> [db]",
		Short: "List the modules of a database (ir_module_module)",
		Long: `Lists the modules known to a database of the instance: name and state
(installed, uninstalled, to upgrade, to remove, ...). Without a database
name the primary database is used.`,
		Example: `  odoonoir module list myapp              # every module
  odoonoir module list myapp --installed   # only installed ones
  odoonoir module list myapp sales         # another database`,
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
			pg := db.New(cfg)
			if err := pg.ServerRunning(); err != nil {
				return err
			}
			initialized, err := pg.IsInitialized(dbName)
			if err != nil {
				return err
			}
			if !initialized {
				return fmt.Errorf("database %s has no Odoo tables — initialize it with: odoonoir init %s %s", dbName, inst.Name, dbName)
			}
			query := "SELECT name, state FROM ir_module_module"
			if flagInstalled {
				query += " WHERE state = 'installed'"
			} else if flagUninstalled {
				query += " WHERE state = 'uninstalled'"
			}
			query += " ORDER BY name"
			out, err := pg.Query(dbName, query)
			if err != nil {
				return err
			}
			rows := make([][]string, 0)
			for _, line := range strings.Split(out, "\n") {
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "|", 2)
				if len(parts) != 2 {
					continue
				}
				state := parts[1]
				cell := state
				switch state {
				case "installed":
					cell = th.Success.Render(ui.CheckMark + " " + state)
				case "uninstalled":
					cell = th.Muted.Render(state)
				case "to upgrade", "to remove":
					cell = th.Warning.Render(state)
				}
				rows = append(rows, []string{parts[0], cell})
			}
			if len(rows) == 0 {
				fmt.Println(th.Hintf("no modules found in %s", dbName))
				return nil
			}
			fmt.Println(th.Table([]string{"MODULE", "STATE"}, rows, ui.WithTitle("modules of "+dbName)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagInstalled, "installed", false, "only installed modules")
	cmd.Flags().BoolVar(&flagUninstalled, "uninstalled", false, "only uninstalled modules")
	return cmd
}

// newModuleUninstallCmd uninstalls a module through the odoo shell.
func newModuleUninstallCmd() *cobra.Command {
	var flagForce bool
	cmd := &cobra.Command{
		Use:   "uninstall <name> <module> [db]",
		Short: "Uninstall a module from a database",
		Long: `Uninstalls the module via the odoo shell (button_immediate_uninstall).
The instance must be stopped. Without a database name the primary
database is used.`,
		Example: `  odoonoir module uninstall myapp my_module
  odoonoir module uninstall myapp my_module sales`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			dbName, _, err := resolveDB(inst, dbArg(args, 2))
			if err != nil {
				return err
			}
			if err := requireStopped(inst); err != nil {
				return err
			}
			if !flagForce {
				fmt.Println(th.Warningf("uninstalling %q from %s — this removes its data", args[1], dbName))
				ok, err := prompt.Confirm("continue?", false)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("aborted")
				}
			}
			script := fmt.Sprintf(`mods = env['ir.module.module'].search([('name', 'in', ['%s'])])
if not mods:
    raise Exception("module not found: %s")
mods.button_immediate_uninstall()
print("OK: uninstalled %%d module(s) (%%s)" %% (len(mods), [m.name for m in mods]))
`, args[1], args[1])
			p := inst.ResolvePaths(instRoot(inst))
			py := installer.PythonFor(inst, p)
			shell := exec.Command(py, "-m", "odoo", "shell", "-c", p.Conf, "-d", dbName)
			shell.Stdin = strings.NewReader(script)
			shell.Dir = p.Source
			if out, err := shell.CombinedOutput(); err != nil {
				return fmt.Errorf("uninstall %s: %w\n%s", args[1], err, strings.TrimSpace(string(out)))
			} else {
				for _, line := range strings.Split(string(out), "\n") {
					if strings.HasPrefix(line, "OK:") {
						fmt.Println(th.Successf("%s", strings.TrimPrefix(line, "OK: ")))
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagForce, "force", false, "skip the confirmation prompt")
	return cmd
}

// runModuleWizard collects module metadata with a huh form group.
func runModuleWizard(m *odoomod.Module, major, name string) error {
	license := "LGPL-3"
	menus := true
	wizard := false
	tests := true
	addModels := true

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Display name").
				Value(&m.DisplayName).
				Suggestions([]string{strings.ReplaceAll(name, "_", " "), strings.Title(strings.ReplaceAll(name, "_", " "))}),
			huh.NewInput().Title("Summary (one line)").Value(&m.Summary),
			huh.NewInput().Title("Author").Value(&m.Author),
			huh.NewSelect[string]().Title("License").Options(
				optionForEach(licenses)...,
			).Value(&license),
			huh.NewInput().Title("Module version").Value(&m.Version).Suggestions([]string{major + ".0.1.0.0"}),
			huh.NewInput().Title("Category").Value(&m.Category).Suggestions([]string{"Uncategorized", "Sales", "Inventory", "Accounting", "Website"}),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().Title("Dependencies").
				Options(optionForEach(commonDeps)...).
				Limit(10).
				Filterable(true).
				Value(&m.Depends),
		),
		huh.NewGroup(
			huh.NewConfirm().Title("Generate application menu?").Value(&menus),
			huh.NewConfirm().Title("Add a wizard (transient model)?").Value(&wizard),
			huh.NewConfirm().Title("Add tests?").Value(&tests),
		),
		huh.NewGroup(
			huh.NewConfirm().Title("Add models interactively?").Value(&addModels),
		),
	)
	if m.DisplayName == "" {
		m.DisplayName = strings.ReplaceAll(name, "_", " ")
	}
	if err := form.WithKeyMap(huhCompatKeyMap()).Run(); err != nil {
		return err
	}
	m.License = license
	m.Menus = menus
	m.Wizard = wizard
	m.Tests = tests
	if strings.TrimSpace(m.Version) == "" {
		m.Version = major + ".0.1.0.0"
	}
	if addModels {
		models, err := askModelsHuh()
		if err != nil {
			return err
		}
		m.Models = models
	}
	return nil
}

func optionForEach(items []string) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(items))
	for _, it := range items {
		out = append(out, huh.NewOption(it, it))
	}
	return out
}

// askModelsHuh collects models and their fields with a huh form group.
func askModelsHuh() ([]odoomod.Model, error) {
	var models []odoomod.Model
	for {
		var more bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title("Add a model?").Value(&more),
		)).WithKeyMap(huhCompatKeyMap()).Run(); err != nil {
			return nil, err
		}
		if !more {
			break
		}
		modName := ""
		label := ""
		desc := ""
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Model technical name (e.g. library.book)").Value(&modName),
			huh.NewInput().Title("Model label (e.g. Book)").Value(&label),
			huh.NewInput().Title("Description (optional)").Value(&desc),
		)).WithKeyMap(huhCompatKeyMap()).Run(); err != nil {
			return nil, err
		}
		if !strings.Contains(modName, ".") || strings.TrimSpace(modName) == "" {
			return nil, fmt.Errorf("model name must be like 'library.book'")
		}
		if label == "" {
			label = strings.Title(modName)
		}
		var fields []odoomod.Field
		for {
			var addField bool
			if err := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().Title("Add a field?").Value(&addField),
			)).WithKeyMap(huhCompatKeyMap()).Run(); err != nil {
				return nil, err
			}
			if !addField {
				break
			}
			fName := ""
			fLabel := ""
			fType := "char"
			rel := ""
			required := false
			fieldsForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().Title("Field name (e.g. isbn)").Value(&fName),
					huh.NewInput().Title("Field label").Value(&fLabel),
					huh.NewSelect[string]().Title("Field type").Options(optionForEach(fieldTypes)...).Value(&fType),
				),
				huh.NewGroup(
					huh.NewInput().Title("Related model (e.g. res.partner)").
						Value(&rel).
						Inline(true),
					huh.NewConfirm().Title("Required?").Value(&required),
				).WithHideFunc(func() bool {
					return !(fType == "many2one" || fType == "one2many" || fType == "many2many")
				}),
			)
			if err := fieldsForm.WithKeyMap(huhCompatKeyMap()).Run(); err != nil {
				return nil, err
			}
			if fName == "" {
				return nil, fmt.Errorf("field name is required")
			}
			if fLabel == "" {
				fLabel = strings.Title(fName)
			}
			f := odoomod.Field{Name: fName, Label: fLabel, Type: fType}
			if fType == "many2one" || fType == "one2many" || fType == "many2many" {
				f.Relation = rel
			}
			if fType == "char" || fType == "integer" || fType == "float" || fType == "boolean" {
				f.Required = required
			}
			fields = append(fields, f)
		}
		models = append(models, odoomod.Model{
			Name: modName, Label: label, Description: desc, Fields: fields,
		})
	}
	return models, nil
}
