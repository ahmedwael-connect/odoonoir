package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/logmon"
)

// colorizeLogLine styles a raw odoo log line: the prefix (timestamp, pid)
// is dimmed and the severity token is rendered as a colored chip. Plain
// lines pass through unchanged.
func colorizeLogLine(line string) string {
	if th == nil || !th.Color {
		return line
	}
	for _, sev := range []string{"CRITICAL", "ERROR", "WARNING", "INFO", "DEBUG"} {
		idx := strings.Index(line, " "+sev+" ")
		if idx < 0 {
			continue
		}
		prefix := line[:idx]
		rest := line[idx+1:]
		sevWord := rest[:len(sev)]
		tail := rest[len(sev)+1:]
		return th.Dim.Render(prefix) + " " + th.SeverityChip(sevWord) + tail
	}
	return line
}

func newLogsCmd() *cobra.Command {
	var follow, errorsOnly bool
	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Tail instance logs",
		Long: `Tail the instance log file. Use --errors to show only errors and hints.

What you will see: the last 50 log lines, or with -f a live stream that
follows the file.`,
		Example: `  odoonoir logs myapp        # last 50 lines
  odoonoir logs -f myapp     # follow the live log
  odoonoir logs --errors myapp  # only errors with fix hints`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var inst *instance.Instance
			var err error
			if len(args) == 1 {
				inst, err = loadInstance(args[0])
			} else if isInteractive() {
				inst, err = pickInstance("logs of which instance?")
			} else {
				return fmt.Errorf("instance name required — `odoonoir logs <name>` (run in a terminal to pick interactively)")
			}
			if err != nil {
				return err
			}
			logPath := inst.ResolvePaths(instRoot(inst)).Log
			if errorsOnly {
				issues, err := logmon.Scan(logPath)
				if err != nil {
					return err
				}
				issues = logmon.ErrorsOnly(logmon.Dedupe(issues))
				if len(issues) == 0 {
					fmt.Println(th.Successf("no errors found in the log"))
					return nil
				}
				for _, l := range logmon.Format(issues) {
					fmt.Println(colorizeLogLine(l))
				}
				return nil
			}
			if !follow {
				lines, err := logmon.Tail(logPath, 50)
				if err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf("no log yet — instance %s has not been started (odoonoir start %s)", inst.Name, inst.Name)
					}
					return err
				}
				for _, l := range lines {
					fmt.Println(colorizeLogLine(l))
				}
				return nil
			}
			return followLog(logPath)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow the log as it grows")
	cmd.Flags().BoolVar(&errorsOnly, "errors", false, "show only detected errors with fix hints")
	return cmd
}

func followLog(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("log file does not exist yet: %s — start the instance first", path)
		}
		return err
	}
	defer f.Close()
	// seek to end to start following fresh content
	off, err := f.Seek(0, 2)
	if err != nil {
		return err
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	// a bufio.Scanner stops reading after the first io.EOF, so lines are
	// assembled manually from polled reads.
	carry := make([]byte, 0, 4096)
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			off += int64(n)
			data := append(carry, buf[:n]...)
			start := 0
			for i := 0; i < len(data); i++ {
				if data[i] == '\n' {
					fmt.Println(colorizeLogLine(string(data[start:i])))
					start = i + 1
				}
			}
			carry = append(carry[:0], data[start:]...)
		}
		switch {
		case err == nil:
			continue // more data pending; drain before waiting
		case !errors.Is(err, io.EOF):
			return err
		}
		<-tick.C
		// rotation: the path now points to a different file (mv-based
		// rotation) or the same file was truncated (copytruncate).
		fi, ferr := f.Stat()
		if ferr != nil {
			return ferr
		}
		rotated := false
		if pfi, perr := os.Stat(path); perr == nil {
			rotated = !os.SameFile(fi, pfi)
		}
		if !rotated && fi.Size() < off {
			rotated = true
		}
		if !rotated {
			continue
		}
		nf, err := os.Open(path)
		if err != nil {
			return err
		}
		_ = f.Close()
		f = nf
		off = 0
		carry = carry[:0]
	}
}
