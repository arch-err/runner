# runner

Rerun a command when files change or you poke it. Runs in any terminal pane.

```sh
runner --watch solve.py -- python solve.py --input sample.txt
runner --watch src --watch Cargo.toml -- cargo run --release

runner --manual --name aoc -- python solve.py --input sample.txt
runner poke aoc
```

Runs immediately unless `--postpone` is set. File changes are debounced; a trigger
stops the previous process tree before starting again. In a terminal, the header
and status footer stay at the top and bottom of the pane. Each run clears the
output area and starts from the top; the footer shows exit status and elapsed
time. Ctrl-C restores your terminal and exits. Redirected output stays plain.
Commands receive no stdin.

Use `--directory PATH` to set the working
directory, and repeat `--ignore GLOB` to exclude basenames or relative paths.
Directories are watched recursively; `.git`, `.hg`, `.svn`, `node_modules`,
`target`, and `dist` are ignored. Explicitly watched files bypass these ignores.
Keep generated output outside watched paths to avoid triggering a loop.
`--clear` is accepted for compatibility; terminal runs always redraw.

Commands and arguments pass through unchanged. For pipes or redirection, launch
`sh -c '...'` or `pwsh -Command '...'` explicitly. See `runner --help` for options
and [Neovim integration](docs/neovim.md) for autosave with on-demand execution.

## Build

With Nix, `nix build` builds the package, `nix run . -- --help` runs it, and
`nix develop` provides Go, a C compiler, Just, and nixfmt. `nix flake check`
builds the package and runs its integration tests.

Requires Go 1.24 or newer. No C compiler is needed for the binaries.

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
