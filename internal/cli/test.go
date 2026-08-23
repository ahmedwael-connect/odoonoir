package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/ui"
	"github.com/ahmed/odoonoir/internal/updater"
)

// newTestCmd runs the odoo test suite of one or more modules.
func newTestCmd() *cobra.Command {
	var modules string
	cmd := &cobra.Command{
		Use:   "test <name> [db]",
		Short: "Run the test suite of modules (-u --test-enable)",
		Long: `Runs the tests of the given modules against a database of the instance
(odoo-bin -u <modules> --test-enable --stop-after-init), streaming the
output. The instance must be stopped. Without a database name the
primary database is used.

Perfect for the scaffolded tests of ` + "`odoonoir module new`" + `:
odoonoir test myapp -m my_module`,
		Example: `  odoonoir test myapp -m my_module                # primary database
  odoonoir test myapp -m my_module,sale_management  # several modules
  odoonoir test myapp sales -m my_module            # another database`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if modules == "" {
				return fmt.Errorf("specify the modules to test with -m/--modules (comma separated)")
			}
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			dbName, _, err := resolveDB(inst, dbArg(args, 1))
			if err != nil {
				return err
			}
			if err := requireStopped(inst); err != nil {
				return err
			}
			modList := strings.Split(modules, ",")
			fmt.Println(th.Infof("running tests of %s on %s", strings.Join(modList, ", "), dbName))
			prog := ui.NewProgress(cmd.OutOrStdout(), "odoo test")
			if err := updater.RunTests(cmd.Context(), inst, instRoot(inst), dbName, modList, func(l string) {
				prog.Line(l)
			}); err != nil {
				prog.Fail()
				return err
			}
			prog.Done()
			fmt.Println(th.Successf("tests of %s passed", strings.Join(modList, ", ")))
			return nil
		},
	}
	cmd.Flags().StringVarP(&modules, "modules", "m", "", "modules to test, comma separated (e.g. my_module,base)")
	return cmd
}
