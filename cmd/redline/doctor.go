package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/version"
	"github.com/spf13/cobra"
)

type checkStatus string

const (
	statusPass checkStatus = "PASS"
	statusWarn checkStatus = "WARN"
	statusFail checkStatus = "FAIL"
)

type checkResult struct {
	name     string
	status   checkStatus
	detail   string
	fixHint  string
	critical bool
}

func doctorRun(c *cobra.Command) error {
	asJSON, _ := c.Flags().GetBool("json")
	runArg, _ := c.Flags().GetString("run")
	results := runDoctorChecks()
	if asJSON {
		writeDoctorJSON(c, results)
	} else {
		writeDoctorTable(c, results)
	}
	if runArg != "" {
		ctx := c.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		ins, err := inspectRun(ctx, global.DB, runArg)
		if err != nil {
			fmt.Fprintln(c.OutOrStdout(), "-----------------------------------------")
			fmt.Fprintf(c.OutOrStdout(), "Run inspection failed: %v\n", err)
		} else {
			writeRunInspection(c.OutOrStdout(), ins)
		}
	}
	for _, r := range results {
		if r.critical && r.status == statusFail {
			// Map to EX_UNAVAILABLE for service / EX_CONFIG for missing config
			if strings.HasPrefix(r.name, "ollama/") || strings.HasPrefix(r.name, "network/") {
				return errs.ErrOllamaUnavailable
			}
			return errs.ErrInvalidConfig
		}
	}
	return nil
}

func runDoctorChecks() []checkResult {
	var rs []checkResult
	rs = append(rs, checkResult{name: "runtime/go-version", status: statusPass, detail: runtime.Version()})
	rs = append(rs, checkResult{name: "runtime/os-arch", status: statusPass, detail: runtime.GOOS + "/" + runtime.GOARCH})
	v, c, _ := version.Info()
	rs = append(rs, checkResult{name: "redline/version", status: statusPass, detail: v + " (" + c + ")"})
	rs = append(rs, checkDBWritable())
	rs = append(rs, checkNetwork())
	rs = append(rs, checkOllama())
	return rs
}

func checkDBWritable() checkResult {
	dir := filepath.Dir(global.DB)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return checkResult{name: "db/write-access", status: statusFail, detail: err.Error(), critical: true}
	}
	probe := filepath.Join(dir, ".redline-doctor-probe")
	f, err := os.Create(probe)
	if err != nil {
		return checkResult{name: "db/write-access", status: statusFail, detail: err.Error(), critical: true}
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return checkResult{name: "db/write-access", status: statusPass, detail: dir}
}

func checkNetwork() checkResult {
	_, err := net.LookupHost("one.one.one.one")
	if err != nil {
		// Not strictly critical for local fixture runs.
		return checkResult{name: "network/dns", status: statusWarn, detail: err.Error()}
	}
	return checkResult{name: "network/dns", status: statusPass, detail: "ok"}
}

func checkOllama() checkResult {
	url := sflags.OllamaURL
	if url == "" {
		url = "http://localhost:11434"
	}
	cl := http.Client{Timeout: 2 * time.Second}
	resp, err := cl.Get(url + "/api/version")
	if err != nil {
		return checkResult{name: "ollama/reachable", status: statusFail, detail: err.Error(), fixHint: "Start Ollama: `ollama serve`"}
	}
	_ = resp.Body.Close()
	return checkResult{name: "ollama/reachable", status: statusPass, detail: url}
}

func writeDoctorTable(c *cobra.Command, results []checkResult) {
	out := c.OutOrStdout()
	fmt.Fprintln(out, "redline doctor — diagnostic report")
	fmt.Fprintln(out, "=========================================")
	fmt.Fprintln(out, "Component                          Status")
	fmt.Fprintln(out, "-----------------------------------------")
	fails := 0
	for _, r := range results {
		name := r.name
		if len(name) > 34 {
			name = name[:34]
		}
		fmt.Fprintf(out, "%-34s %s", name, r.status)
		if r.detail != "" {
			fmt.Fprintf(out, "  (%s)", r.detail)
		}
		fmt.Fprintln(out)
		if r.fixHint != "" {
			fmt.Fprintf(out, "                                   → fix: %s\n", r.fixHint)
		}
		if r.critical && r.status == statusFail {
			fails++
		}
	}
	fmt.Fprintln(out, "-----------------------------------------")
	if fails > 0 {
		fmt.Fprintf(out, "Result: FAIL (%d critical)\n", fails)
	} else {
		fmt.Fprintln(out, "Result: PASS")
	}
}

func writeDoctorJSON(c *cobra.Command, results []checkResult) {
	out := c.OutOrStdout()
	for _, r := range results {
		fmt.Fprintf(out,
			"{\"name\":%q,\"status\":%q,\"detail\":%q,\"fix\":%q,\"critical\":%v}\n",
			r.name, string(r.status), r.detail, r.fixHint, r.critical,
		)
	}
}
