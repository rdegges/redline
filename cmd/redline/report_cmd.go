package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rdegges/redline/internal/store"
)

// reportRunImpl regenerates a report from an existing DB.
func reportRunImpl() error {
	db, err := store.Open(context.Background(), global.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	run, err := db.LatestRun(context.Background(), "")
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("no runs found in %s", global.DB)
	}
	// Determine output path/format.
	output := sflags.Output
	if output == "" {
		output = "./redline-report"
	}
	wantJSON := false
	wantMD := false
	wantCSV := false
	for _, f := range sflags.Format {
		switch f {
		case "json":
			wantJSON = true
		case "markdown":
			wantMD = true
		case "csv":
			wantCSV = true
		}
	}
	// Always pull JSON + MD from DB cache.
	if wantJSON || (!wantMD && !wantCSV) {
		b, err := db.GetReport(context.Background(), run.ID, "json")
		if err != nil {
			return err
		}
		if b == nil {
			return fmt.Errorf("no JSON report stored for run %s", run.ID)
		}
		if err := writeOutput(output, "report.json", b); err != nil {
			return err
		}
	}
	if wantMD || (!wantJSON && !wantCSV) {
		b, err := db.GetReport(context.Background(), run.ID, "markdown")
		if err != nil {
			return err
		}
		if b == nil {
			return fmt.Errorf("no Markdown report stored for run %s", run.ID)
		}
		if err := writeOutput(output, "report.md", b); err != nil {
			return err
		}
	}
	if wantCSV {
		b, err := db.GetReport(context.Background(), run.ID, "csv")
		if err != nil {
			return err
		}
		if b == nil {
			return fmt.Errorf("no CSV report stored for run %s", run.ID)
		}
		if err := writeOutput(output, "report.csv", b); err != nil {
			return err
		}
	}
	return nil
}

func writeOutput(target, name string, b []byte) error {
	if isDir(target) || filepath.Ext(target) == "" {
		_ = os.MkdirAll(target, 0o755)
		return os.WriteFile(filepath.Join(target, name), b, 0o644)
	}
	return os.WriteFile(target, b, 0o644)
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
