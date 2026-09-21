//go:build !windows
// +build !windows

package verify

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the step in its own process group so that
// cancellation reaches the whole tree rather than only the first process.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// Setpgid makes the group id the pid. Asking the kernel for it fails
	// once the leader has been reaped, while the group is still running.
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
