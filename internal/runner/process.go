package runner

import (
	"os"
	"os/exec"
	"time"
)

type process struct {
	cmd   *exec.Cmd
	done  chan struct{}
	group processGroup
}

func startProcess(args []string, directory string) (*process, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = directory
	// This runner is for noninteractive commands; output retains the pane's TTY.
	cmd.Stdin = nil
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	group, err := prepareProcess(cmd)
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		group.close()
		return nil, err
	}
	if err := group.attach(cmd); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		group.close()
		return nil, err
	}
	p := &process{cmd: cmd, done: make(chan struct{}), group: group}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *process) close() { p.group.close() }

func (p *process) stop(timeout time.Duration) {
	p.group.terminate()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) && p.group.alive() {
		time.Sleep(10 * time.Millisecond)
	}
	p.group.close()
	<-p.done
}
