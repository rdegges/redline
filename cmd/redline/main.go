// Command redline is the CLI entry point.
package main

import (
	"fmt"
	"os"

	"github.com/rdegges/redline/internal/errs"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "redline:", err)
		os.Exit(errs.ExitCode(err))
	}
}
