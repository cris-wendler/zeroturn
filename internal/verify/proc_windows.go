//go:build windows
// +build windows

package verify

import "os/exec"

// Windows has no process group equivalent that Go's standard library
// exposes, so cancellation terminates the started process only.
func setProcessGroup(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}
