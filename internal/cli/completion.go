package cli

import (
	"github.com/spf13/cobra"
)

// newCompletionCmd exposes cobra-generated shell completion scripts.
func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate a shell completion script",
		Long:      "Generate the shell completion script for odoonoir and print it to stdout.\n\n  bash:  odoonoir completion bash > /etc/bash_completion.d/odoonoir\n  zsh:   odoonoir completion zsh > \"${fpath[1]}/_odoonoir\"\n  fish:  odoonoir completion fish > ~/.config/fish/completions/odoonoir.fish",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			root.SetOut(cmd.OutOrStdout())
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			}
			return nil
		},
	}
}
