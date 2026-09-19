# vital

Four readings — coverage, risks, evidence, decisions. One overall status. `vital` computes; `exhibit` renders. Red means something needs attention before the next cycle.

```bash
go get github.com/Formulary-Labs/vital
```

## What it does

`vital` reads a program run state JSON and computes a `Snapshot` with traffic-light ratings across four health dimensions: coverage, risks, evidence, and decisions. `OverallStatus` is the worst status across all four dimensions.

## Usage

```go
import "github.com/Formulary-Labs/vital/health"

snapshot, err := health.Compute(runStateJSON)

fmt.Println(snapshot.OverallStatus) // "green", "yellow", "red", or "grey"
```

## Health dimensions and thresholds

| Dimension | Green | Yellow | Red | Grey |
|---|---|---|---|---|
| Coverage | Evidenced ≥ 80% | ≥ 60% | < 60% | No coverage data |
| Risks | No critical, ≤ 2 high open | > 2 high open | Any critical open | No risks logged |
| Evidence | No overdue, none due within 30 days | Due within 30 days | Any overdue or stale (> 90 days since last run) | No `run_date` |
| Decisions | No blockers, ≤ 3 deferred | > 3 deferred | Any blocker | — |

`OverallStatus` is `grey` only if all four dimensions return `grey` (insufficient data). Otherwise it is the worst non-grey status present.

## RunState input

```json
{
  "program": "my-program",
  "run_date": "2026-09-18",
  "updated_at": "2026-09-18T14:00:00Z",
  "coverage": {
    "evidenced_pct": 72.0,
    "implemented_pct": 84.0,
    "gap_pct": 16.0,
    "total": 114
  },
  "risks": [
    { "id": "RISK-001", "severity": "high", "status": "open" }
  ],
  "evidence_windows": [
    { "control_id": "A.8.3", "due_date": "2026-10-01", "status": "pending" }
  ],
  "blockers": [],
  "decisions": [
    { "id": "DEC-001", "status": "deferred" }
  ],
  "recommended_next_run": "2026-10-18"
}
```

`vital` reads both the legacy 1.1 schema and the standard 2.0 schema for the `coverage` block. Fields not present are treated as `grey` for their dimension.

## Output

```json
{
  "program": "my-program",
  "generated_at": "2026-09-18T14:05:00Z",
  "overall_status": "yellow",
  "coverage": {
    "total_controls": 114,
    "evidenced_pct": 72.0,
    "gap_pct": 16.0,
    "status": "yellow"
  },
  "risks": {
    "total": 8,
    "critical": 0,
    "high": 1,
    "open": 5,
    "status": "green"
  },
  "evidence": {
    "overdue_count": 0,
    "due_soon_count": 1,
    "is_stale": false,
    "status": "yellow"
  },
  "decisions": {
    "deferred_count": 1,
    "blocker_count": 0,
    "status": "green"
  },
  "blockers": [],
  "flags": []
}
```

`flags` lists data quality issues found during computation — missing fields, inconsistent schema version, or coverage data that appears stale relative to `updated_at`.

## Reading the output

`OverallStatus: "yellow"` with `coverage.status: "yellow"` and `evidence.status: "yellow"` means: evidenced coverage is between 60–80%, and at least one evidence window is due within 30 days. Neither condition is a blocker, but both require attention before the next audit cycle.

`OverallStatus: "red"` with any critical risk open, or any overdue evidence window, requires immediate attention.

## Pipeline context

`vital` runs after `titer` and `specimen` have updated their outputs for the current cycle. Its output feeds:

- `exhibit` — as the internal health section of a program dashboard (note: `exhibit` is the auditor-facing view; `vital` data is internal)
- The agent layer (regimen) — as the data source for daily briefs and weekly status updates

```bash
vital --run-state runs/my-program/latest.json > health-snapshot.json
```

## License

Apache License 2.0
