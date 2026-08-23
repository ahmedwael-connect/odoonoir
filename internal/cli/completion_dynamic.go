package cli

import (
	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/instance"
)

// completeInstanceNames provides tab-completion of registered instance names
// for the first positional argument of instance-taking commands. The registry
// is loaded lazily so completion works even outside the normal pre-run.
func completeInstanceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names, err := instanceNames()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func instanceNames() ([]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	r, err := instance.NewRegistry(cfg.RegistryPath())
	if err != nil {
		return nil, err
	}
	all, err := r.All()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(all))
	for _, inst := range all {
		names = append(names, inst.Name)
	}
	return names, nil
}

// withInstanceCompletion attaches instance-name completion to a command.
func withInstanceCompletion(cmd *cobra.Command) *cobra.Command {
	cmd.ValidArgsFunction = completeInstanceNames
	return cmd
}
