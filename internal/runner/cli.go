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
	"strings"
	"time"
)

const help = `Run a command again when files change or you poke it.

  runner ./solve.sh
  runner -w solve.py -- python solve.py --input sample.txt
  runner -m -n aoc -- python solve.py
  runner poke aoc

Options:
  -w, --watch PATH        Watch a file or directory; repeat for multiple paths
  -m, --manual            Run only on pokes or SIGUSR1 (Unix)
  -n, --name NAME         Give this instance a name for runner poke
  -p, --postpone          Wait for a trigger before the first run
  -c, --clear             Accepted for compatibility; terminal runs always redraw
  -d, --debounce TIME     Group file changes (default 100ms)
  -t, --stop-timeout TIME Allow graceful shutdown (default 1s)
  -i, --ignore GLOB       Ignore a basename or relative path; repeat as needed
  -C, --directory PATH    Working directory for the command and watched paths
  -h, --help              Show this help

Boolean flags can be grouped: runner -mn aoc ./solve.sh
Without -m or -w, exactly one existing file in the command is watched.
Scroll: j/k or arrows (lines), Ctrl-U/D (half-pages), Page Up/Down (pages).
gg/Home: top. G/End: bottom and follow. Mouse wheel also scrolls.
Each run clears its history; up to 10,000 rows are retained. Ctrl-C or q exits.

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
	// Aliases share the same flag.Value, including repeatable paths and globs.
	for short, long := range map[string]string{
		"w": "watch", "m": "manual", "n": "name", "p": "postpone",
		"c": "clear", "d": "debounce", "t": "stop-timeout",
		"i": "ignore", "C": "directory",
	} {
		f.Var(f.Lookup(long).Value, short, "")
	}
	expanded, err := expandFlags(f, args)
	if err != nil {
		return c, err
	}
	if err := f.Parse(expanded); err != nil {
		return c, err
	}
	c.command = f.Args()
	if len(c.command) == 0 {
		return c, errors.New("provide a command after -- (see runner --help)")
	}
	if c.manual && len(c.watch) > 0 {
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
	if !c.manual && len(c.watch) == 0 {
		files := make(map[string]string)
		for _, arg := range c.command {
			if len(arg) == 0 || arg[0] == '-' {
				continue
			}
			candidate := arg
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(c.directory, candidate)
			}
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				resolved, err := filepath.EvalSymlinks(candidate)
				if err != nil {
					return c, err
				}
				files[resolved] = arg
			}
		}
		if len(files) != 1 {
			return c, errors.New("cannot infer one file to watch; use --watch PATH or --manual")
		}
		for _, arg := range files {
			c.watch = append(c.watch, arg)
		}
	}
	return c, nil
}

// Expand runner flags only; the command and its arguments remain untouched.
func expandFlags(f *flag.FlagSet, args []string) ([]string, error) {
	var expanded []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" || len(arg) < 2 || arg[0] != '-' {
			return append(expanded, args[index:]...), nil
		}
		name := arg[1:]
		if name[0] == '-' {
			name = name[1:]
		}
		if at := strings.IndexByte(name, '='); at >= 0 {
			name = name[:at]
		}
		if option := f.Lookup(name); option != nil {
			expanded = append(expanded, arg)
			boolean, ok := option.Value.(interface{ IsBoolFlag() bool })
			if !strings.Contains(arg, "=") && !(ok && boolean.IsBoolFlag()) && index+1 < len(args) {
				index++
				expanded = append(expanded, args[index])
			}
			continue
		}
		if strings.HasPrefix(arg, "--") || arg == "-h" {
			expanded = append(expanded, arg)
			continue
		}
		for at := 1; at < len(arg); at++ {
			short := string(arg[at])
			option := f.Lookup(short)
			if short == "h" {
				expanded = append(expanded, "-h")
				continue
			}
			if option == nil {
				return nil, fmt.Errorf("unknown short flag -%s", short)
			}
			expanded = append(expanded, "-"+short)
			boolean, ok := option.Value.(interface{ IsBoolFlag() bool })
			if ok && boolean.IsBoolFlag() {
				continue
			}
			if at+1 < len(arg) {
				expanded = append(expanded, strings.TrimPrefix(arg[at+1:], "="))
			} else if index+1 < len(args) {
				index++
				expanded = append(expanded, args[index])
			}
			break
		}
	}
	return expanded, nil
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
