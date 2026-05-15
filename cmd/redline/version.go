package main

import (
	"fmt"
	"runtime"

	"github.com/rdegges/redline/internal/version"
	"github.com/spf13/cobra"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			v, c, d := version.Info()
			fmt.Fprintf(cmd.OutOrStdout(),
				"version: %s\ncommit: %s\nbuilt: %s\ngo: %s\nos: %s/%s\n",
				v, c, d, runtime.Version(), runtime.GOOS, runtime.GOARCH,
			)
			return nil
		},
	}
}
