package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/service"
	"github.com/ahmed/odoonoir/internal/ui"
)

// Version is the CLI build version (overridable via -ldflags).
var Version = "0.24.0"

var (
	cfg         *config.Config
	reg         *instance.Registry
	svc         *service.Service
	th          *ui.Theme
	rootCmd     *cobra.Command
	noColor     bool
	warnWriter  = os.Stderr
	errorWriter = os.Stderr
)

func init() {
	rootCmd = &cobra.Command{
		Use:     "odoonoir",
		Short:   "Odoo instance manager — install, run and develop Odoo like a pro",
		Version: Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			th = ui.NewTheme(!noColor && ui.ColorEnabled(os.Stdout, nil))
			var err error
			cfg, err = config.Load()
			if err != nil {
				return err
			}
			reg, err = instance.NewRegistry(cfg.RegistryPath())
			if err != nil {
				return err
			}
			svc, err = service.New(cfg)
			return err
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)
	attachHelpTemplate(rootCmd)
	rootCmd.AddCommand(
		newCheckCmd(),
		newCreateCmd(),
		withInstanceCompletion(newListCmd()),
		withInstanceCompletion(newStatusCmd()),
		withInstanceCompletion(newStartCmd()),
		withInstanceCompletion(newStopCmd()),
		withInstanceCompletion(newRestartCmd()),
		withInstanceCompletion(newInitCmd()),
		withInstanceCompletion(newBackupCmd()),
		withInstanceCompletion(newRestoreCmd()),
		withInstanceCompletion(newDropCmd()),
		newDbCmd(),
		withInstanceCompletion(newConfigCmd()),
		withInstanceCompletion(newLogsCmd()),
		withInstanceCompletion(newDoctorCmd()),
		withInstanceCompletion(newModuleCmd()),
		withInstanceCompletion(newUpdateCmd()),
		withInstanceCompletion(newRemoveCmd()),
		withInstanceCompletion(newShellCmd()),
		withInstanceCompletion(newTestCmd()),
		withInstanceCompletion(newCloneCmd()),
		withInstanceCompletion(newRenameCmd()),
		withInstanceCompletion(newAdoptCmd()),
		withInstanceCompletion(newEditCmd()),
		newCompletionCmd(),
		newDashCmd(),
		newPSCmd(),
		newSelfUpdateCmd(),
		withInstanceCompletion(newWatchCmd()),
	)
}

// Execute runs the CLI.
func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t := ui.NewTheme(!noColor && ui.ColorEnabled(os.Stderr, nil))
		fmt.Fprintf(os.Stderr, "%s\n", t.Errorf("%v", err))
		os.Exit(1)
	}
}

// loadInstance fetches an instance from the registry by name.
func loadInstance(name string) (*instance.Instance, error) {
	return reg.Get(name)
}

// instRoot returns the instance's storage root: per-instance override when
// set, otherwise the global instances directory.
func instRoot(inst *instance.Instance) string {
	if inst.Root != "" {
		return inst.Root
	}
	return cfg.InstancesDir()
}

// warn prints a yellow-ish warning line.
func warn(format string, args ...any) {
	if th != nil {
		fmt.Fprintln(warnWriter, th.Warningf(format, args...))
		return
	}
	fmt.Fprintf(warnWriter, "warning: "+format+"\n", args...)
}
