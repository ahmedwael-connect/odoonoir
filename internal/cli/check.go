package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/checker"
	"github.com/ahmed/odoonoir/internal/ui"
)

func newCheckCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "check [-v 17|18|19]",
		Short: "Audit the system for Odoo requirements",
		Long: `Audits the host system: python, node, git, postgres, system libraries.
Pass -v <major> to also enforce python version compatibility with that Odoo release.

What you will see: one line per requirement with ✓/✗ marks, plus a
summary of what is missing and how to fix it.`,
		Example: `  odoonoir check            # audit everything
  odoonoir check -v 19      # also check python version for Odoo 19`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			chk := checker.Run()
			if version != "" {
				chk.ForVersion(version)
			}
			failures := 0
			missing := 0
			rows := make([][]string, 0, len(chk.Results))
			for _, r := range chk.Results {
				found := r.Found
				if found == "" {
					found = "not found"
				}
				if r.Hint != "" {
					found = th.Hintf("%s — %s", found, r.Hint)
				}
				rows = append(rows, []string{checkMark(r.Severity), r.Check, found})
				if r.Severity == checker.ERROR {
					failures++
				}
				if r.Severity == checker.MISSING {
					missing++
				}
			}
			fmt.Println(th.Table([]string{"", "CHECK", "RESULT"}, rows, ui.WithTitle("system audit")))
			fmt.Println()
			switch {
			case failures > 0:
				fmt.Println(th.Errorf("result: FAIL (%d errors) — fix the errors above, then re-run", failures))
			case missing > 0:
				fmt.Println(th.Warningf("result: WARN (%d missing) — optional packages can be installed later", missing))
			default:
				fmt.Println(th.Successf("result: OK — system is ready for Odoo"))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&version, "version", "v", "", "check python compatibility against an Odoo major version (17, 18, 19)")
	return cmd
}

func checkMark(s checker.Severity) string {
	switch s {
	case checker.OK:
		return th.Success.Render(ui.CheckMark)
	case checker.WARN:
		return th.Warning.Render(ui.WarnMark)
	case checker.ERROR:
		return th.Error.Render(ui.CrossMark)
	default:
		return th.Muted.Render("·")
	}
}
