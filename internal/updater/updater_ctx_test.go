package updater

import (
	"context"
	"errors"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunStreamCancellation proves that cancelling ctx kills the running
// command and surfaces ctx.Err() — the behaviour the GUI relies on for its
// "cancel" button on long update/init/test operations.
func TestRunStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var lines atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- runStream(ctx, exec.CommandContext(ctx, "sleep", "30"),
			"", func(string) { lines.Add(1) })
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runStream did not abort after cancel")
	}
}

// TestRunStreamPassthrough verifies a command that exits non-zero still
// surfaces its real error when the context is not cancelled.
func TestRunStreamPassthrough(t *testing.T) {
	err := runStream(context.Background(), exec.Command("sh", "-c", "echo boom; exit 3"), "", func(string) {})
	if err == nil {
		t.Fatal("expected an error from failing command")
	}
}
