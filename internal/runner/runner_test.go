package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Real subprocesses exercise supervision, IPC, and filesystem events together.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("RUNNER_TEST_HELPER") != "1" {
		return
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(99)
	}
	switch args[0] {
	case "cli":
		if err := Main(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "append":
		f, err := os.OpenFile(args[1], os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(98)
		}
		fmt.Fprintln(f, strings.Join(args[2:], "|"))
		f.Close()
	case "tree":
		child := helper("sleep")
		if err := child.Start(); err != nil {
			os.Exit(97)
		}
		os.WriteFile(args[1], []byte(fmt.Sprint(child.Process.Pid)), 0600)
		time.Sleep(time.Minute)
	case "sleep":
		time.Sleep(time.Minute)
	case "exit":
		os.Exit(7)
	default:
		os.Exit(96)
	}
	os.Exit(0)
}

func helper(args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "RUNNER_TEST_HELPER=1")
	return cmd
}

type safeBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *safeBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }

func eventually(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out: " + description)
}

type instance struct {
	cmd  *exec.Cmd
	log  *safeBuffer
	done chan error
}

func launch(t *testing.T, directory string, args ...string) *instance {
	t.Helper()
	cmd := helper(append([]string{"cli"}, args...)...)
	cmd.Dir = directory
	log := &safeBuffer{}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	i := &instance{cmd, log, make(chan error, 1)}
	go func() { i.done <- cmd.Wait() }()
	t.Cleanup(func() {
		cmd.Process.Kill()
		select {
		case <-i.done:
		case <-time.After(5 * time.Second):
			t.Errorf("runner did not exit: %s", log.String())
		}
	})
	eventually(t, "runner ready: "+log.String(), func() bool { return strings.Contains(log.String(), " · pid ") })
	return i
}

func appendCommand(file string, args ...string) []string {
	return append([]string{os.Args[0], "-test.run=^TestHelperProcess$", "--", "append", file}, args...)
}

func lineCount(file string) int { data, _ := os.ReadFile(file); return bytes.Count(data, []byte("\n")) }

func TestManualPokeAndDuplicateName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_RUNTIME_DIR", filepath.Join(dir, "state"))
	t.Setenv("RUNNER_TEST_HELPER", "1")
	output := filepath.Join(dir, "output")
	args := append([]string{"--manual", "--postpone", "--name", "aoc", "--"}, appendCommand(output, "space here", "--flag", "$literal")...)
	i := launch(t, dir, args...)
	if lineCount(output) != 0 {
		t.Fatal("postponed command ran")
	}
	if err := poke("AOC"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "first poke", func() bool { return lineCount(output) == 1 })
	data, _ := os.ReadFile(output)
	if string(data) != "space here|--flag|$literal\n" {
		t.Fatalf("arguments changed: %q", data)
	}
	duplicate := helper(append([]string{"cli"}, args...)...)
	if out, err := duplicate.CombinedOutput(); err == nil || !strings.Contains(string(out), "already running") {
		t.Fatalf("duplicate instance: %s, %v", out, err)
	}
	if err := poke("aoc"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "second poke", func() bool { return lineCount(output) == 2 })
	if err := poke("missing"); err == nil {
		t.Fatal("missing instance was accepted")
	}
	if strings.Contains(i.log.String(), "failed") {
		t.Fatal(i.log.String())
	}
}

func TestWatchAtomicSaveAndDebounce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TEST_HELPER", "1")
	source := filepath.Join(dir, "solve.txt")
	output := filepath.Join(dir, "output")
	os.WriteFile(source, []byte("first"), 0600)
	args := append([]string{"--watch", source, "--debounce", "150ms", "--"}, appendCommand(output, "run")...)
	launch(t, dir, args...)
	eventually(t, "initial execution", func() bool { return lineCount(output) == 1 })
	for range 5 {
		os.WriteFile(source, []byte("changed"), 0600)
	}
	eventually(t, "debounced write", func() bool { return lineCount(output) >= 2 })
	time.Sleep(300 * time.Millisecond)
	if count := lineCount(output); count != 2 {
		t.Fatalf("expected one rerun for write burst, got %d executions", count)
	}
	for n := 3; n <= 4; n++ {
		tmp := filepath.Join(dir, "replacement")
		os.WriteFile(tmp, []byte("replacement"), 0600)
		if err := os.Rename(tmp, source); err != nil {
			t.Fatal(err)
		}
		eventually(t, "atomic replacement", func() bool { return lineCount(output) == n })
	}
}

func TestRecursiveWatchAndIgnores(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TEST_HELPER", "1")
	root := filepath.Join(dir, "src")
	os.Mkdir(root, 0700)
	output := filepath.Join(dir, "output")
	args := append([]string{"--watch", root, "--ignore", "*.log", "--"}, appendCommand(output, "run")...)
	launch(t, dir, args...)
	eventually(t, "initial execution", func() bool { return lineCount(output) == 1 })
	os.Mkdir(filepath.Join(root, "node_modules"), 0700)
	os.WriteFile(filepath.Join(root, "node_modules", "ignored"), []byte("x"), 0600)
	os.WriteFile(filepath.Join(root, "ignored.log"), []byte("x"), 0600)
	time.Sleep(300 * time.Millisecond)
	if lineCount(output) != 1 {
		t.Fatal("ignored paths triggered execution")
	}
	sub := filepath.Join(root, "new", "nested")
	os.MkdirAll(sub, 0700)
	eventually(t, "new directory", func() bool { return lineCount(output) >= 2 })
	time.Sleep(200 * time.Millisecond)
	before := lineCount(output)
	os.WriteFile(filepath.Join(sub, "solve.py"), []byte("x"), 0600)
	eventually(t, "file in new subtree", func() bool { return lineCount(output) > before })
}

func TestFailureWaitsForAnotherPoke(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_RUNTIME_DIR", filepath.Join(dir, "state"))
	t.Setenv("RUNNER_TEST_HELPER", "1")
	args := []string{"--manual", "--name", "failure", "--", os.Args[0], "-test.run=^TestHelperProcess$", "--", "exit"}
	i := launch(t, dir, args...)
	eventually(t, "failed command reported", func() bool { return strings.Contains(i.log.String(), "exit 7") })
	if err := poke("failure"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "failed command rerun", func() bool { return strings.Count(i.log.String(), "exit 7") == 2 })
}

func TestParse(t *testing.T) {
	for _, args := range [][]string{
		{"--manual"}, {"--", "echo"}, {"--manual", "--watch", ".", "--", "echo"},
		{"--manual", "--name", "../bad", "--", "echo"}, {"--manual", "--debounce", "-1s", "--", "echo"},
		{"--manual", "--ignore", "[", "--", "echo"},
	} {
		if _, err := parse(args); err == nil {
			t.Errorf("accepted invalid args: %v", args)
		}
	}
}
