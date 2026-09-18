// vital produces a program health snapshot from a run state JSON.
//
// Usage:
//
//	vital --run-state runs/myprogram/latest.json [--format json|md]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Formulary-Labs/vital/health"
	"github.com/Formulary-Labs/substrate/exit"
	"github.com/Formulary-Labs/substrate/provenance"
)

const version = "0.1.0"

func main() {
	var (
		runStateFlag = flag.String("run-state", "", "Path to run state JSON (required)")
		programFlag  = flag.String("program", "", "Program slug (inferred from run state if empty)")
		fmtFlag      = flag.String("format", "json", "Output format: json (default), md")
		quietFlag    = flag.Bool("quiet", false, "Suppress progress output")
		versionFlag  = flag.Bool("version", false, "Print version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *versionFlag {
		fmt.Printf("vital version %s\n", version)
		os.Exit(exit.OK)
	}

	if *runStateFlag == "" {
		fmt.Fprintln(os.Stderr, `{"error": "--run-state is required", "code": 2}`)
		flag.Usage()
		os.Exit(exit.ToolError)
	}

	rs, err := health.Load(*runStateFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"error": %q, "code": 2}`+"\n", err.Error())
		os.Exit(exit.ToolError)
	}

	if *programFlag != "" {
		rs.Program = *programFlag
	}

	snapshot := health.Compute(rs)

	switch *fmtFlag {
	case "md":
		printMD(snapshot, *quietFlag)
	default:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snapshot); err != nil {
			fmt.Fprintf(os.Stderr, `{"error": %q, "code": 2}`+"\n", err.Error())
			os.Exit(exit.ToolError)
		}
	}

	if rs.Program != "" {
		_ = provenance.Write("logs/provenance.jsonl", provenance.Entry{
			Spec:        "functions/program-dashboard-spec.md",
			Output:      *runStateFlag,
			OutputType:  "other",
			Program:     rs.Program,
			Purpose:     fmt.Sprintf("vital: health snapshot for %s (overall: %s)", rs.Program, snapshot.OverallStatus),
			Reusability: provenance.Instance,
			QualityGate: provenance.Pass,
			Tool:        "vital",
			ToolVersion: version,
		})
	}
}

func printMD(s health.Snapshot, quiet bool) {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "# Program Health: %s\n\n", s.Program)
	fmt.Fprintf(sb, "**Generated:** %s  \n", s.GeneratedAt.Format("2006-01-02 15:04 UTC"))
	fmt.Fprintf(sb, "**Overall:** %s\n\n", statusEmoji(s.OverallStatus))

	fmt.Fprintf(sb, "## Coverage — %s\n\n", statusEmoji(s.Coverage.Status))
	fmt.Fprintf(sb, "| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(sb, "| Total controls | %d |\n", s.Coverage.TotalControls)
	fmt.Fprintf(sb, "| Evidenced | %.0f%% |\n", s.Coverage.EvidencedPct)
	fmt.Fprintf(sb, "| Implemented | %.0f%% |\n", s.Coverage.ImplementedPct)
	fmt.Fprintf(sb, "| Gap | %.0f%% |\n", s.Coverage.GapPct)
	fmt.Fprintf(sb, "| Owner gaps | %d |\n", s.Coverage.OwnerGapCount)
	fmt.Fprintf(sb, "| Evidence gaps | %d |\n\n", s.Coverage.EvidenceGapCount)

	fmt.Fprintf(sb, "## Risks — %s\n\n", statusEmoji(s.Risks.Status))
	fmt.Fprintf(sb, "| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(sb, "| Total | %d |\n", s.Risks.Total)
	fmt.Fprintf(sb, "| Critical | %d |\n", s.Risks.Critical)
	fmt.Fprintf(sb, "| High | %d |\n", s.Risks.High)
	fmt.Fprintf(sb, "| Open | %d |\n", s.Risks.Open)
	fmt.Fprintf(sb, "| Accepted | %d |\n\n", s.Risks.Accepted)

	fmt.Fprintf(sb, "## Evidence — %s\n\n", statusEmoji(s.Evidence.Status))
	fmt.Fprintf(sb, "| Metric | Value |\n|---|---|\n")
	if s.Evidence.LastRunDate != nil {
		fmt.Fprintf(sb, "| Last run | %s |\n", s.Evidence.LastRunDate.Format("2006-01-02"))
	}
	if s.Evidence.DaysSinceRun != nil {
		fmt.Fprintf(sb, "| Days since run | %d |\n", *s.Evidence.DaysSinceRun)
	}
	fmt.Fprintf(sb, "| Stale | %v |\n", s.Evidence.IsStale)
	fmt.Fprintf(sb, "| Overdue | %d |\n", s.Evidence.OverdueCount)
	fmt.Fprintf(sb, "| Due soon (30d) | %d |\n\n", s.Evidence.DueSoonCount)

	if len(s.Blockers) > 0 {
		fmt.Fprintf(sb, "## Blockers\n\n")
		for _, b := range s.Blockers {
			fmt.Fprintf(sb, "- %s\n", b)
		}
		fmt.Fprintln(sb)
	}

	if len(s.Flags) > 0 {
		fmt.Fprintf(sb, "## Flags\n\n")
		for _, f := range s.Flags {
			fmt.Fprintf(sb, "- %s\n", f)
		}
		fmt.Fprintln(sb)
	}

	_ = quiet
	fmt.Print(sb.String())
}

func statusEmoji(s health.Status) string {
	switch s {
	case health.StatusGreen:
		return "GREEN"
	case health.StatusYellow:
		return "YELLOW"
	case health.StatusRed:
		return "RED"
	default:
		return "GREY (insufficient data)"
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `vital — program health snapshot

Usage:
  vital --run-state <path> [flags]

Flags:
  --run-state string   Path to run state JSON (required)
  --program string     Program slug (inferred from run state if empty)
  --format string      Output format: json (default), md
  --quiet              Suppress progress output
  --version            Print version and exit

Examples:
  vital --run-state runs/iso42001/latest.json
  vital --run-state runs/iso42001/latest.json --format md
  decay --from runs/2026-Q2.json --to runs/2026-Q3.json | vital --format md`)
}
