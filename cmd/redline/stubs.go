package main

import (
	"github.com/spf13/cobra"
)

// Command builders for the v1 subcommand surface.

func scanCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "scan",
		Short: "Crawl, judge, embed, and write a report (full pipeline)",
		RunE:  runScan,
	}
	addScanFlags(c)
	return c
}

func crawlCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "crawl",
		Short: "Run only the crawl stage",
		RunE:  runCrawlOnly,
	}
	addCrawlFlags(c)
	return c
}

func judgeCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "judge",
		Short: "Run only the LLM judge stage",
		RunE:  runJudgeOnly,
	}
	addJudgeFlags(c)
	return c
}

func embedCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "embed",
		Short: "Run only the embedding + dedup stage",
		RunE:  runEmbedOnly,
	}
	addEmbedFlags(c)
	return c
}

func reportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "report",
		Short: "Regenerate a report from an existing DB",
		RunE:  runReportOnly,
	}
	addReportFlags(c)
	return c
}

func doctorCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose runtime, DB, and provider readiness (optionally inspect a run)",
		RunE:  runDoctor,
	}
	c.Flags().Bool("json", false, "Emit results as NDJSON")
	c.Flags().String("run", "", "Also inspect a redline run (ID, or 'latest' for the most recent)")
	return c
}

func modelsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "models",
		Short: "Manage local Ollama models required by redline",
	}
	c.AddCommand(
		&cobra.Command{Use: "list", Short: "List configured models", RunE: runModelsList},
		&cobra.Command{Use: "recommend", Short: "Print recommended models per RAM tier", RunE: runModelsRecommend},
	)
	return c
}
