//go:build linux || darwin || freebsd || openbsd || netbsd

package runner

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func stopSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} }
func pokeSignals() []os.Signal { return []os.Signal{syscall.SIGUSR1} }

type processGroup struct{ pid int }

func prepareProcess(cmd *exec.Cmd) (processGroup, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return processGroup{}, nil
}

func (g *processGroup) attach(cmd *exec.Cmd) error { g.pid = cmd.Process.Pid; return nil }
func (g *processGroup) terminate()                 { _ = unix.Kill(-g.pid, unix.SIGTERM) }
func (g *processGroup) alive() bool                { return unix.Kill(-g.pid, 0) == nil }
func (g *processGroup) close() {
	if g.pid > 0 {
		_ = unix.Kill(-g.pid, unix.SIGKILL)
		g.pid = 0
	}
}

func lockFile(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
