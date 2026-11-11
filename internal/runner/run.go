package runner

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"
)

func request(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func run(c config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stops := make(chan os.Signal, 2)
	signal.Notify(stops, stopSignals()...)
	defer signal.Stop(stops)
	go func() {
		select {
		case <-stops:
			cancel()
		case <-ctx.Done():
		}
	}()
	triggers := make(chan struct{}, 1)
	pokes := make(chan os.Signal, 1)
	if sigs := pokeSignals(); len(sigs) > 0 {
		signal.Notify(pokes, sigs...)
		defer signal.Stop(pokes)
		go func() {
			for {
				select {
				case <-pokes:
					request(triggers)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	if c.name != "" {
		control, err := listenControl(c.name, triggers)
		if err != nil {
			return err
		}
		defer control.Close()
	}
	watchErrors := make(chan error, 1)
	if len(c.watch) > 0 {
		w, err := watch(c, triggers, watchErrors)
		if err != nil {
			return err
		}
		defer w.Close()
	}
	label := "runner"
	if c.name != "" {
		label += " " + c.name
	}
	screen := openScreen(fmt.Sprintf("%s · pid %d · %s", label, os.Getpid(), mode(c)))
	defer screen.Close()
	resize := time.NewTicker(200 * time.Millisecond)
	defer resize.Stop()
	var child *process
	var done <-chan struct{}
	defer func() {
		if child != nil {
			child.stop(c.stopTimeout)
		}
	}()
	start := func() {
		screen.resize()
		screen.begin()
		p, err := startProcess(c.command, c.directory)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s · start failed: %v · waiting\n", label, err)
			screen.failed()
			return
		}
		child, done = p, p.done
	}
	if !c.postpone {
		start()
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-watchErrors:
			return fmt.Errorf("watch: %w", err)
		case <-resize.C:
			screen.resize()
		case <-triggers:
			if child != nil {
				child.stop(c.stopTimeout)
				child, done = nil, nil
			}
			// Changes during shutdown belong to the execution we are about to start.
			select {
			case <-triggers:
			default:
			}
			if ctx.Err() != nil {
				return nil
			}
			start()
		case <-done:
			child.close()
			screen.finish(child.cmd.ProcessState.ExitCode(), time.Since(child.started))
			child, done = nil, nil
		}
	}
}

func mode(c config) string {
	if c.manual {
		return "manual"
	}
	return "watching"
}
