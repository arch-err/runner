package runner

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCombinedFlagsAndInference(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"solve.sh", "input.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c, err := parse([]string{"-mn", "aoc", "./solve.sh"})
	if err != nil || !c.manual || c.name != "aoc" {
		t.Fatalf("combined flags: %#v %v", c, err)
	}
	for _, args := range [][]string{{"-C", dir, "./solve.sh"}, {"-C", dir, "python", "solve.sh"}, {"-C", dir, "./solve.sh", "./solve.sh"}} {
		c, err := parse(args)
		if err != nil || len(c.watch) != 1 {
			t.Fatalf("inference %v: %#v %v", args, c, err)
		}
		if !reflect.DeepEqual(c.command, args[2:]) {
			t.Fatal("command arguments were changed")
		}
	}
	if _, err := parse([]string{"-C", dir, "python", "solve.sh", "input.txt"}); err == nil {
		t.Fatal("ambiguous files accepted")
	}
	c, err = parse([]string{"-C", dir, "-wsolve.sh", "--", "echo", "-mn"})
	if err != nil || !reflect.DeepEqual(c.command, []string{"echo", "-mn"}) {
		t.Fatalf("attached value or command flags: %#v %v", c, err)
	}
}
