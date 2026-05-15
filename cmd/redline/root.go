package main

import (
	"github.com/spf13/cobra"
)

type globalFlags struct {
	DB        string
	LogLevel  string
	LogFormat string
	NoColor   bool
	Yes       bool
}

var global = &globalFlags{}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "redline",
		Short:         "Messaging-coherence + GEO content auditor",
		Long:          "redline audits a website's content against canonical brand messaging and GEO target prompts, and emits an agent-actionable edit plan.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&global.DB, "db", "./redline.db", "Path to SQLite database")
	cmd.PersistentFlags().StringVar(&global.LogLevel, "log-level", "info", "Log level: debug,info,warn,error")
	cmd.PersistentFlags().StringVar(&global.LogFormat, "log-format", "text", "Log format: text,json")
	cmd.PersistentFlags().BoolVar(&global.NoColor, "no-color", false, "Disable ANSI color")
	cmd.PersistentFlags().BoolVarP(&global.Yes, "yes", "y", false, "Skip confirmation prompts")
	cmd.AddCommand(
		scanCmd(),
		crawlCmd(),
		judgeCmd(),
		embedCmd(),
		reportCmd(),
		doctorCmd(),
		modelsCmd(),
		versionCmd(),
	)
	return cmd
}
