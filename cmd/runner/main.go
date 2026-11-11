package main

import (
	"fmt"
	"os"

	"git.3rr.dev/arch-err/runner/internal/runner"
)

func main() {
	if err := runner.Main(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "runner:", err)
		os.Exit(1)
	}
}
