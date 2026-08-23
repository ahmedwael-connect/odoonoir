package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/tui"
)

// newDashCmd returns the `odoonoir dash` command — a full-screen TUI dashboard.
func newDashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dash",
		Aliases: []string{"dashboard", "tui"},
		Short:   "Interactive TUI dashboard for your instances",
		Long: `Interactive TUI dashboard for your instances.

Navigate the list with the arrow keys, press enter for instance details,
[l] for live logs, [s]/[x]/[r] to start, stop or restart, and [q] to quit.`,
		Example: `  odoonoir dash   # open the dashboard (arrows to navigate, q to quit)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isInteractive() {
				return fmt.Errorf("dash requires an interactive terminal — run it in a real terminal, not a pipe or script")
			}
			return tui.Run(tui.Deps{
				Cfg: cfg,
				Reg: reg,
				Th:  th,
			})
		},
	}
	return cmd
}

// isInteractive reports whether stdin is a real terminal (termios check —
// /dev/null is a char device but not a terminal).
func isInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd())
}
