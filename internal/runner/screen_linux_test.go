package runner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestTerminalLayoutAndRestoration(t *testing.T) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	master := os.NewFile(uintptr(fd), "ptmx")
	defer master.Close()
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	resize := func(rows, columns uint16) {
		t.Helper()
		if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: columns}); err != nil {
			t.Fatal(err)
		}
	}
	resize(12, 80)
	dir := t.TempDir()
	t.Setenv("RUNNER_TEST_HELPER", "1")
	t.Setenv("RUNNER_RUNTIME_DIR", filepath.Join(dir, "state"))
	cmd := helper("cli", "--manual", "--name", "tty", "--", os.Args[0], "-test.run=^TestHelperProcess$", "--", "exit")
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	output := &safeBuffer{}
	copied := make(chan struct{})
	go func() { _, _ = io.Copy(output, master); close(copied) }()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runner did not exit")
		}
		slave.Close()
		master.Close()
		<-copied
	})
	eventually(t, "first run footer", func() bool { return strings.Contains(output.String(), "exit 7") })
	if err := poke("tty"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "second run footer", func() bool { return strings.Count(output.String(), "exit 7") >= 2 })
	text := output.String()
	if strings.Count(text, "\x1b[?1049h") != 1 || strings.Count(text, "command failed") < 2 || strings.Count(text, "\x1b[2J") != 3 {
		t.Fatalf("runner did not reuse and clear its screen: %q", text)
	}
	if !strings.Contains(text, "\x1b[12;1H") {
		t.Fatalf("missing fixed output region: %q", text)
	}
	resize(8, 40)
	eventually(t, "layout follows terminal size", func() bool {
		return strings.Contains(output.String(), "\x1b[8;1H")
	})
	cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not stop")
	}
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	eventually(t, "terminal state restored", func() bool {
		text := output.String()
		return strings.Contains(text, "\x1b[?1000l") && strings.Contains(text, "\x1b[?1006l") && strings.Contains(text, "\x1b[?25h") && strings.Contains(text, "\x1b[?1049l")
	})
}
