package cli

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/installer"
	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/proc"
	"github.com/ahmed/odoonoir/internal/ui"
)

// procInfo captures per-process runtime data read from /proc.
type procInfo struct {
	pid    int
	uptime time.Duration
	memKB  int64
}

// readProcInfo reads /proc/<pid>/stat (start time) and /proc/<pid>/status
// (VmRSS). Returns zero values when the process is gone or unreadable.
func readProcInfo(pid int) procInfo {
	var info procInfo
	info.pid = pid
	if stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		fields := strings.Fields(string(stat))
		// field 22 (index 21) = starttime in clock ticks since boot
		if len(fields) > 21 {
			if ticks, err := strconv.ParseInt(fields[21], 10, 64); err == nil {
				if boot, err := readUptime(); err == nil {
					info.uptime = boot - time.Duration(ticks)*clockTick()
				}
			}
		}
	}
	if status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid)); err == nil {
		for _, line := range strings.Split(string(status), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						info.memKB = kb
					}
				}
				break
			}
		}
	}
	return info
}

// clockTick returns the kernel clock tick rate (usually 100 Hz).
func clockTick() time.Duration {
	return time.Second / 100
}

// readUptime reads /proc/uptime (seconds since boot, float).
func readUptime() (time.Duration, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected /proc/uptime format")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(secs * 1e9), nil
}

func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// newPSCmd prints one row per instance with live process data.
func newPSCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "Overview of all instances: pid, uptime, memory",
		Long: `A process-table style overview of every registered instance: status,
pid, uptime and resident memory (from /proc) when running, plus port,
database and log path.`,
		Example: `  odoonoir ps`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			instances, err := reg.All()
			if err != nil {
				return err
			}
			if len(instances) == 0 {
				fmt.Println(th.Hintf("no instances registered — create one with: odoonoir create <name> -v 18"))
				return nil
			}
rows := make([][]string, 0, len(instances))
		for _, inst := range instances {
			p := inst.ResolvePaths(instRoot(inst))
			mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
				status, pid, err := mgr.Status()
				if err != nil {
					status = instance.StatusUnknown
				}
				statusStr := string(status)
				uptime, mem := "-", "-"
				if status == instance.StatusRunning {
					statusStr = fmt.Sprintf("%s (pid %d)", string(status), pid)
					info := readProcInfo(pid)
					if info.uptime > 0 {
						uptime = humanDuration(info.uptime)
					}
					if info.memKB > 0 {
						mem = fmt.Sprintf("%.1f MiB", float64(info.memKB)/1024)
					}
				}
				rows = append(rows, []string{
					inst.Name,
					th.StatusPill(statusStr),
					uptime,
					mem,
					fmt.Sprint(inst.Port),
					inst.DBName,
					p.Log,
				})
			}
			fmt.Println(th.Table(
				[]string{"NAME", "STATUS", "UPTIME", "MEMORY", "PORT", "DATABASE", "LOG"},
				rows,
				ui.WithTitle("processes"),
			))
			return nil
		},
	}
}

// newWatchCmd shows a live-updating status + log tail for an instance.
func newWatchCmd() *cobra.Command {
	var tail int
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "watch <name>",
		Short: "Live status and log tail of an instance (q to quit)",
		Long: `Refreshes the instance status and the tail of its log file every few
seconds (q or Ctrl-C to quit). Errors in the log are colored like in
` + "`odoonoir logs`" + `. The log is read incrementally, so it stays cheap
even for huge files.`,
		Example: `  odoonoir watch myapp
  odoonoir watch myapp --tail 20 --interval 500ms`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			insts, err := targetInstances(cmd, args, false, "watch which instance?")
			if err != nil {
				return err
			}
inst := insts[0]
		p := inst.ResolvePaths(instRoot(inst))
		mgr := proc.New(p, installer.PythonFor(inst, p), p.Conf, inst.LongpollPort)
			if !isInteractive() {
				return fmt.Errorf("watch needs a terminal — run it interactively or use: odoonoir logs -f %s", inst.Name)
			}
			state, err := term.MakeRaw(os.Stdin.Fd())
			if err != nil {
				return err
			}
			defer term.Restore(os.Stdin.Fd(), state)

			keys := make(chan byte, 1)
			readErr := make(chan error, 1)
			go func() {
				buf := make([]byte, 1)
				for {
					if _, err := os.Stdin.Read(buf); err != nil {
						readErr <- err
						return
					}
					select {
					case keys <- buf[0]:
					default:
					}
				}
			}()
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(stop)

			var lastOffset int64
			render := func() {
				if th.Color {
					fmt.Print("\033[2J\033[H")
				} else {
					fmt.Print("\n")
				}
				status, pid, err := mgr.Status()
				if err != nil {
					status = instance.StatusUnknown
				}
				statusStr := string(status)
				rows := [][2]string{
					{"instance", inst.Name},
					{"status", th.StatusPill(statusStr)},
					{"port", fmt.Sprint(inst.Port)},
					{"database", inst.DBName},
					{"log", p.Log},
				}
				if status == instance.StatusRunning {
					info := readProcInfo(pid)
					rows = append(rows, [2]string{"pid", fmt.Sprint(pid)})
					if info.uptime > 0 {
						rows = append(rows, [2]string{"uptime", humanDuration(info.uptime)})
					}
					if info.memKB > 0 {
						rows = append(rows, [2]string{"memory", fmt.Sprintf("%.1f MiB", float64(info.memKB)/1024)})
					}
				}
				fmt.Println(th.Panel("status", th.KV(rows)))
				if lines, _, err := logmon.TailFrom(p.Log, tail, &lastOffset); err == nil && len(lines) > 0 {
					colored := make([]string, 0, len(lines))
					for _, l := range lines {
						colored = append(colored, colorizeLogLine(l))
					}
					fmt.Println(th.Panel("log tail", strings.Join(colored, "\n")))
				}
				fmt.Println(th.Hintf("q to quit"))
			}
			render()
			tick := time.NewTicker(interval)
			defer tick.Stop()
			for {
				select {
				case <-tick.C:
					render()
				case k := <-keys:
					if k == 'q' {
						fmt.Println()
						return nil
					}
				case <-stop:
					fmt.Println()
					return nil
				case err := <-readErr:
					_ = err
					fmt.Println()
					return nil
				}
			}
		},
	}
	cmd.Flags().IntVar(&tail, "tail", 15, "number of log lines to show on the first render")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval (e.g. 500ms, 2s)")
	return cmd
}
