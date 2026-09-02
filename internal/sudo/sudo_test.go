package sudo

import "testing"

func TestRunWithPasswordEmpty(t *testing.T) {
	_, err := RunWithPassword("", "echo", "hi")
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestCheckInvalidPassword(t *testing.T) {
	// use wrong password, sudo -v should fail quickly without actually doing apt
	err := Check("definitely_wrong_password_123456")
	if err == nil {
		t.Log("sudo -v unexpectedly succeeded with wrong password (maybe passwordless sudo)")
	}
}
