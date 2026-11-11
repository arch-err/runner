//go:build linux || darwin || freebsd || openbsd || netbsd

package runner

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func processAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// A killed orphan can remain a zombie until the host's init reaps it.
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err == nil && strings.HasPrefix(string(data[strings.LastIndex(string(data), ")")+1:]), " Z ") {
		return false
	}
	return true
}

func TestUnixSignalAndGracefulExit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_RUNTIME_DIR", filepath.Join(dir, "state"))
	t.Setenv("RUNNER_TEST_HELPER", "1")
	output := filepath.Join(dir, "output")
	args := append([]string{"--manual", "--name", "signal", "--postpone", "--"}, appendCommand(output, "run")...)
	i := launch(t, dir, args...)
	if err := i.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	eventually(t, "signal execution", func() bool { return lineCount(output) == 1 })
	i.cmd.Process.Signal(syscall.SIGTERM)
	// Leave cmd.Wait's result for launch's cleanup.
	eventually(t, "control state removed", func() bool { p, _ := controlPath("signal"); _, err := os.Stat(p); return os.IsNotExist(err) })
}

func TestRestartAndShutdownKillDescendants(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_RUNTIME_DIR", filepath.Join(dir, "state"))
	t.Setenv("RUNNER_TEST_HELPER", "1")
	pidfile := filepath.Join(dir, "pid")
	args := []string{"--manual", "--name", "tree", "--stop-timeout", "100ms", "--", os.Args[0], "-test.run=^TestHelperProcess$", "--", "tree", pidfile}
	i := launch(t, dir, args...)
	readPID := func() int { data, _ := os.ReadFile(pidfile); n, _ := strconv.Atoi(string(data)); return n }
	eventually(t, "descendant started", func() bool { return readPID() > 0 })
	old := readPID()
	if err := poke("tree"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "new descendant", func() bool { return readPID() > 0 && readPID() != old })
	eventually(t, "old descendant stopped", func() bool { return !processAlive(old) })
	current := readPID()
	i.cmd.Process.Signal(syscall.SIGTERM)
	eventually(t, "descendant stopped on exit", func() bool { return !processAlive(current) })
}

func TestStopEscalatesToKill(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	p, err := startProcess([]string{"sh", "-c", `trap '' TERM; echo ready > "$1"; while :; do sleep 1; done`, "sh", ready}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	eventually(t, "TERM handler installed", func() bool { _, err := os.Stat(ready); return err == nil })
	started := time.Now()
	p.stop(100 * time.Millisecond)
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("stop took %s", elapsed)
	}
	if processAlive(p.cmd.Process.Pid) {
		t.Fatal("command survived stop")
	}
}

func TestNaturalExitCleansBackgroundDescendants(t *testing.T) {
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "pid")
	p, err := startProcess([]string{"sh", "-c", `sleep 60 & echo $! > "$1"`, "sh", pidfile}, dir)
	if err != nil {
		t.Fatal(err)
	}
	<-p.done
	p.close()
	data, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	eventually(t, "background child cleaned up", func() bool { return !processAlive(pid) })
}
