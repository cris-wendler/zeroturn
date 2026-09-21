//go:build !windows
// +build !windows

package verify

import (
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// exec.CommandContext kills the step's first process on cancellation too,
// and when it wins the race that process has been reaped by the time
// killGroup runs. The group still has to die, or Wait blocks on the pipe
// the survivors hold. Seen once in CI as a cancelled step taking its full
// five seconds.
func TestKillGroupReachesTheGroupAfterItsLeaderIsReaped(t *testing.T) {
	skipWithoutSh(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var child int
	for i := 0; i < 200 && child == 0; i++ {
		b, _ := ioutil.ReadFile(pidFile)
		child, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		time.Sleep(10 * time.Millisecond)
	}
	if child == 0 {
		t.Fatal("the child never reported its pid")
	}
	defer syscall.Kill(child, syscall.SIGKILL)

	cmd.Process.Kill()
	cmd.Process.Wait()
	killGroup(cmd)

	for i := 0; i < 200; i++ {
		if syscall.Kill(child, 0) != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a process in the step's group outlived killGroup")
}
