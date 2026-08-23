package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/logmon"
)

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor <name>",
		Short: "Diagnose instance health from the logs",
		Long: `Scans the instance log for known failure signatures
(port conflicts, database auth errors, missing modules, missing python deps)
and suggests fixes.

What you will see: each issue found with the log line, why it happens,
and the command to fix it.`,
		Example: `  odoonoir doctor myapp   # diagnose a failing instance from its log`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := loadInstance(args[0])
			if err != nil {
				return err
			}
			p := inst.ResolvePaths(instRoot(inst))
			if _, err := os.Stat(p.Log); os.IsNotExist(err) {
				fmt.Println(th.Warningf("instance %s has no log yet — it has never been started", inst.Name))
				fmt.Println(th.Hintf("start it with: odoonoir start %s", inst.Name))
				return nil
			}
			issues, err := logmon.Scan(p.Log)
			if err != nil {
				return err
			}
			issues = logmon.Dedupe(issues)
			fmt.Println(th.Dim.Render("scanning log: " + p.Log))
			if len(issues) == 0 {
				fmt.Println(th.Successf("no issues detected — the log looks clean"))
				return nil
			}
			errors := 0
			warnings := 0
			for _, it := range issues {
				if it.Severity == "error" {
					errors++
				} else {
					warnings++
				}
			}
			if errors > 0 {
				fmt.Println(th.Errorf("found %d errors and %d warnings:", errors, warnings))
			} else {
				fmt.Println(th.Warningf("found %d warnings:", warnings))
			}
			fmt.Println()
			for _, l := range logmon.Format(issues) {
				fmt.Println(l)
			}
			fmt.Println()
			if errors > 0 {
				fmt.Println(th.Hintf("suggestion: run `odoonoir logs --errors %s` after restarting to re-check", inst.Name))
			}
			return nil
		},
	}
	return cmd
}
