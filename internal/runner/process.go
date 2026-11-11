package runner

import (
	"os"
	"os/exec"
	"time"
)

type process struct {
	cmd   *exec.Cmd
	done  chan result
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
	started := time.Now()
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
	p := &process{cmd: cmd, done: make(chan result, 1), group: group}
	go func() {
		code := 0
		if err := cmd.Wait(); err != nil {
			code = cmd.ProcessState.ExitCode()
		}
		p.done <- result{code, time.Since(started)}
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
