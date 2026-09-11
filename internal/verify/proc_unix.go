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
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil && pgid > 0 {
		syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}
	cmd.Process.Kill()
}
