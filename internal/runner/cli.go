package runner

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"
)

const help = `Run a command again when files change or you poke it.

  runner --watch solve.py -- python solve.py --input sample.txt
  runner --manual --name aoc -- python solve.py
  runner poke aoc

Options:
  --watch PATH       Watch a file or directory; repeat for multiple paths
  --manual           Run only on pokes or SIGUSR1 (Unix)
  --name NAME        Give this instance a name for runner poke
  --postpone         Wait for a trigger before the first run
  --clear            Accepted for compatibility; terminal runs always redraw
  --debounce TIME    Group file changes (default 100ms)
  --stop-timeout TIME  Allow graceful shutdown (default 1s)
  --ignore GLOB      Ignore a basename or relative path; repeat as needed
  --directory PATH  Working directory for the command and watched paths
  --help             Show this help

Commands run directly. Use sh -c '...' or pwsh -Command '...' for shell syntax.
SIGUSR1 triggers a run on Unix. Ctrl-C stops the command and exits.
`

type stringsFlag []string

func (s *stringsFlag) String() string     { return fmt.Sprint([]string(*s)) }
func (s *stringsFlag) Set(v string) error { *s = append(*s, v); return nil }

type config struct {
	watch, ignore           stringsFlag
	manual, postpone, clear bool
	name, directory         string
	debounce, stopTimeout   time.Duration
	command                 []string
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func parse(args []string) (config, error) {
	var c config
	f := flag.NewFlagSet("runner", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.Var(&c.watch, "watch", "")
	f.Var(&c.ignore, "ignore", "")
	f.BoolVar(&c.manual, "manual", false, "")
	f.BoolVar(&c.postpone, "postpone", false, "")
	f.BoolVar(&c.clear, "clear", false, "")
	f.StringVar(&c.name, "name", "", "")
	f.StringVar(&c.directory, "directory", ".", "")
	f.DurationVar(&c.debounce, "debounce", 100*time.Millisecond, "")
	f.DurationVar(&c.stopTimeout, "stop-timeout", time.Second, "")
	if err := f.Parse(args); err != nil {
		return c, err
	}
	c.command = f.Args()
	if len(c.command) == 0 {
		return c, errors.New("provide a command after -- (see runner --help)")
	}
	if c.manual == (len(c.watch) > 0) {
		return c, errors.New("choose either --manual or one or more --watch paths")
	}
	if c.manual && c.name == "" && len(pokeSignals()) == 0 {
		return c, errors.New("--manual requires --name on this platform so runner poke can trigger it")
	}
	if c.name != "" && !validName.MatchString(c.name) {
		return c, errors.New("name must be 1–64 letters, digits, hyphens or underscores, starting with a letter or digit")
	}
	if c.debounce < 0 || c.stopTimeout < 0 {
		return c, errors.New("durations cannot be negative")
	}
	for _, pattern := range c.ignore {
		if _, err := path.Match(pattern, ""); err != nil {
			return c, fmt.Errorf("ignore pattern %q: %w", pattern, err)
		}
	}
	var err error
	c.directory, err = filepath.Abs(c.directory)
	if err != nil {
		return c, err
	}
	info, err := os.Stat(c.directory)
	if err != nil {
		return c, err
	}
	if !info.IsDir() {
		return c, errors.New("--directory must be a directory")
	}
	return c, nil
}

func Main(args []string) error {
	if len(args) == 0 {
		fmt.Print(help)
		return nil
	}
	if args[0] == "poke" {
		if len(args) != 2 || !validName.MatchString(args[1]) {
			return errors.New("usage: runner poke NAME")
		}
		return poke(args[1])
	}
	c, err := parse(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Print(help)
		return nil
	}
	if err != nil {
		return err
	}
	return run(c)
}
