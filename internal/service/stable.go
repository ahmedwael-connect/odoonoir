package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/ahmed/odoonoir/internal/logmon"
	"github.com/ahmed/odoonoir/internal/proc"
)

// waitForStable checks the process is still alive shortly after start and
// returns a diagnostic (log tail) when it crashed immediately. The sleep is
// ctx-aware so GUI cancellation works during the check window.
func waitForStable(ctx context.Context, mgr *proc.Manager, logPath string) error {
	t := time.NewTimer(4 * time.Second)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
	}
	status, _, err := mgr.Status()
	if err != nil {
		return err
	}
	if status == instance.StatusRunning {
		return nil
	}
	lines, tailErr := logmon.Tail(logPath, 15)
	if tailErr != nil || len(lines) == 0 {
		return fmt.Errorf("instance crashed shortly after starting — run `odoonoir doctor <name>` for hints")
	}
	return fmt.Errorf("instance crashed shortly after starting:\n%s", strings.Join(lines, "\n"))
}
