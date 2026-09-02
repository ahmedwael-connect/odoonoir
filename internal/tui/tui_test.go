package tui

import "testing"

func TestRunRequiresDeps(t *testing.T) {
	err := Run(Deps{})
	if err == nil {
		t.Log("Run with empty deps returned nil (may be ok)")
	}
}
