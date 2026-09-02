package sudo

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// RunWithPassword runs `sudo -S <args...>` feeding password via stdin.
// Returns combined output. Use for apt installs when user provided password via GUI wizard.
func RunWithPassword(password string, args ...string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("sudo password required")
	}
	// prepend -S to read from stdin
	sudoArgs := append([]string{"-S"}, args...)
	cmd := exec.Command("sudo", sudoArgs...)
	cmd.Stdin = strings.NewReader(password + "\n")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("sudo %s: %w\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String(), nil
}

// Check verifies sudo password is correct without side effects.
func Check(password string) error {
	_, err := RunWithPassword(password, "-v")
	return err
}

// AptUpdate runs apt-get update with sudo.
func AptUpdate(password string) (string, error) {
	return RunWithPassword(password, "apt-get", "update")
}

// AptInstall installs packages with sudo.
func AptInstall(password string, pkgs ...string) (string, error) {
	if len(pkgs) == 0 {
		return "", nil
	}
	args := append([]string{"apt-get", "install", "-y"}, pkgs...)
	return RunWithPassword(password, args...)
}
