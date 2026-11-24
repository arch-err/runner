# runner

Rerun a command when files change or you poke it. Runs in any terminal pane.

```sh
runner --watch solve.py -- python solve.py --input sample.txt
runner --watch src --watch Cargo.toml -- cargo run --release

runner --manual --name aoc -- python solve.py --input sample.txt
runner poke aoc
```

Short flags: `-w` watch, `-m` manual, `-n` name, `-p` postpone, `-d` debounce,
`-t` stop timeout, `-i` ignore, `-C` directory, `-c` clear, and `-h` help.
Long and short flags can be mixed; repeat `-w` or `-i` for multiple values.

Boolean flags can be combined: `runner -mn aoc ./solve.sh`. A value-taking flag
consumes the rest of its group or the next argument (`-nNAME` or `-n NAME`).
Runner stops parsing options at the command or `--`.

With no mode flags, exactly one distinct existing regular file in the command is
watched automatically: `runner ./solve.sh` or `runner python solve.py`.
Multiple files (for example a script plus an input file) require explicit `-w`
or `-m`. Paths are resolved relative to `-C`; executable names are not searched
on PATH and shell snippets are not parsed.

In a terminal, `j`/`k` or arrows scroll by line, Ctrl-U/D by half-page, and
Page Up/Down by page. `gg`/Home goes to the oldest retained output; `G`/End
follows the latest output again. The mouse wheel also scrolls. While
scrolled, new output leaves the view in place. Each run resets the buffer, which
retains up to 10,000 history rows. Ctrl-C or q exits. Hold Shift for terminal text
selection if your terminal reserves the mouse while reporting wheel events.

Runs immediately unless `--postpone` is set. File changes are debounced; a trigger
stops the previous process tree before starting again. In a terminal, the header
and status footer stay at the top and bottom of the pane. Each run clears the
output area and starts from the top; the footer shows exit status and elapsed
time. Ctrl-C restores your terminal and exits. Redirected output stays plain.
Commands receive no stdin. Unix commands retain a TTY for output; Windows
output is captured through pipes.

Use `--directory PATH` to set the working
directory, and repeat `--ignore GLOB` to exclude basenames or relative paths.
Directories are watched recursively; `.git`, `.hg`, `.svn`, `node_modules`,
`target`, and `dist` are ignored. Explicitly watched files bypass these ignores.
Keep generated output outside watched paths to avoid triggering a loop.
`--clear` is accepted for compatibility; terminal runs always redraw.

Commands and arguments pass through unchanged. For pipes or redirection, launch
`sh -c '...'` or `pwsh -Command '...'` explicitly. See `runner --help` for options
and [Neovim integration](docs/neovim.md) for autosave with on-demand execution.

## Download

Get a binary from [Releases](https://git.3rr.dev/arch-err/runner/releases).
On Windows, run it from Git Bash with Bash explicitly:

```sh
./runner-windows-amd64.exe -w script.sh -- bash ./script.sh
```

## Build

With Nix, `nix build` builds the package, `nix run . -- --help` runs it, and
`nix develop` provides Go, a C compiler, Just, and nixfmt. `nix flake check`
builds the package and runs its integration tests.

Requires Go 1.24.2 or newer. No C compiler is needed for the binaries.

```sh
go build -o runner ./cmd/runner
go install ./cmd/runner
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o runner.exe ./cmd/runner
```

`just test` runs the race-enabled tests (requires a C compiler); `just check`
checks formatting and runs vet. `just build` and `just windows` write to `dist/`.

On Unix, `kill -USR1 PID` also triggers a run. Windows uses Job Objects for
process cleanup and attempts Ctrl-Break before forced termination. Unix uses
process groups and TERM followed by KILL after `--stop-timeout` (default 1s).
Commands that deliberately escape their process group are outside Unix cleanup.

Named pokes use an authenticated loopback connection and per-user state under
the OS cache directory. Names are case-insensitive and unique per user. Override
the state location with `RUNNER_RUNTIME_DIR` in both terminals if needed. Restarting
an instance recovers stale state after a crash. Windows binaries are cross-built;
native Windows runtime verification is still needed.

CI checks formatting, runs vet and race-enabled tests, and cross-builds Linux,
macOS and Windows binaries for amd64 and arm64. Push a version tag (for example
`git tag v0.1.0 && git push origin v0.1.0`) to publish those binaries and
`SHA256SUMS` as a Forgejo release. `just release` builds the same assets locally.

CI uses the restricted `k8s` runner without external dependency downloads.
After changing dependencies, run `just ci-deps`, upload the generated archive
with an authenticated PUT to the printed Forgejo package URL, and commit
`.forgejo/vendor.sha256` with the dependency changes.
