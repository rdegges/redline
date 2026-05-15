package main

import (
	"github.com/spf13/cobra"
)

// Runners — thin glue layer between cobra commands and the
// implementations in scan.go / report.go / etc.
func runScan(_ *cobra.Command, _ []string) error       { return scanRun() }
func runCrawlOnly(_ *cobra.Command, _ []string) error  { return crawlOnlyRun() }
func runJudgeOnly(_ *cobra.Command, _ []string) error  { return judgeOnlyRun() }
func runEmbedOnly(_ *cobra.Command, _ []string) error  { return embedOnlyRun() }
func runReportOnly(_ *cobra.Command, _ []string) error { return reportOnlyRun() }
func runDoctor(c *cobra.Command, _ []string) error     { return doctorRun(c) }
func runModelsList(_ *cobra.Command, _ []string) error { return modelsListRun() }
func runModelsRecommend(c *cobra.Command, _ []string) error {
	return modelsRecommendRun(c)
}

// The crawl/judge/embed-only paths share the scan implementation but
// with appropriate flags pre-set.
func scanRun() error      { return scanRunImpl() }
func crawlOnlyRun() error { return scanCrawlOnly() }
func judgeOnlyRun() error { return scanRunImpl() } // judge against existing DB
func embedOnlyRun() error { return scanRunImpl() } // embed against existing DB
func reportOnlyRun() error {
	return reportRunImpl()
}

func scanCrawlOnly() error {
	sflags.DryRun = true
	return scanRunImpl()
}
