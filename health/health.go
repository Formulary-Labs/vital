// Package health implements the program health snapshot for vital.
//
// vital reads a run state JSON file (produced by the prompt-repo agent layer
// or assembled from titer + specimen outputs) and computes a structured
// health snapshot: coverage, risk posture, evidence currency, and open
// decisions/blockers.
//
// The snapshot is the Go equivalent of program_dashboard_renderer.py's
// data layer — deterministic computation only; rendering (HTML) is left to
// the agent layer or exhibit.
package health

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Status is a traffic-light health rating.
type Status string

//nolint:revive // Status constants are self-documenting.
const (
	StatusGreen  Status = "green"
	StatusYellow Status = "yellow"
	StatusRed    Status = "red"
	StatusGrey   Status = "grey" // insufficient data
)

// Snapshot is the full program health snapshot produced by vital.
type Snapshot struct {
	Program       string         `json:"program"`
	GeneratedAt   time.Time      `json:"generated_at"`
	OverallStatus Status         `json:"overall_status"`
	Coverage      CoverageHealth `json:"coverage"`
	Risks         RiskHealth     `json:"risks"`
	Evidence      EvidenceHealth `json:"evidence"`
	Decisions     DecisionHealth `json:"decisions"`
	Blockers      []string       `json:"blockers,omitempty"`
	Flags         []string       `json:"flags,omitempty"`
}

// CoverageHealth summarizes control coverage status.
type CoverageHealth struct {
	TotalControls    int     `json:"total_controls"`
	EvidencedPct     float64 `json:"evidenced_pct"`
	ImplementedPct   float64 `json:"implemented_pct"`
	GapPct           float64 `json:"gap_pct"`
	OwnerGapCount    int     `json:"owner_gap_count"`
	EvidenceGapCount int     `json:"evidence_gap_count"`
	Status           Status  `json:"status"`
}

// RiskHealth summarizes risk register status.
type RiskHealth struct {
	Total    int    `json:"total"`
	Critical int    `json:"critical"`
	High     int    `json:"high"`
	Open     int    `json:"open"`
	Accepted int    `json:"accepted"`
	Status   Status `json:"status"`
}

// EvidenceHealth summarizes evidence currency.
type EvidenceHealth struct {
	OverdueCount int        `json:"overdue_count"`
	DueSoonCount int        `json:"due_soon_count"` // within 30 days
	LastRunDate  *time.Time `json:"last_run_date,omitempty"`
	DaysSinceRun *int       `json:"days_since_run,omitempty"`
	IsStale      bool       `json:"is_stale"` // past recommended_next_run
	Status       Status     `json:"status"`
}

// DecisionHealth summarizes pending decisions and blockers.
type DecisionHealth struct {
	DeferredCount int    `json:"deferred_count"`
	BlockerCount  int    `json:"blocker_count"`
	Status        Status `json:"status"`
}

// flexTime is a time.Time wrapper whose JSON unmarshaler accepts both
// RFC 3339 ("2026-01-15T00:00:00Z") and date-only ("2026-01-15") formats.
type flexTime time.Time

func (ft *flexTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			*ft = flexTime(t)
			return nil
		}
	}
	return fmt.Errorf("cannot parse time %q: expected RFC3339 or YYYY-MM-DD", s)
}

func (ft flexTime) Time() time.Time { return time.Time(ft) }

// RunState is the minimal subset of a program run JSON that vital reads.
// The full run JSON schema is defined by the prompt-repo agent layer and
// is not reproduced here — vital reads only what it needs.
type RunState struct {
	Program   string    `json:"program"`
	UpdatedAt *flexTime `json:"updated_at,omitempty"`
	RunDate   *flexTime `json:"run_date,omitempty"`

	// Coverage — both schema variants (1.1 legacy and 2.0 standard).
	Coverage *CoverageBlock `json:"coverage,omitempty"`

	// Risks — array of risk entries.
	Risks []RiskEntry `json:"risks,omitempty"`

	// Evidence windows.
	EvidenceWindows []EvidenceWindow `json:"evidence_windows,omitempty"`

	// Blockers and deferred decisions.
	Blockers  []string `json:"blockers,omitempty"`
	Decisions []struct {
		Deferred bool   `json:"deferred,omitempty"`
		Text     string `json:"text,omitempty"`
	} `json:"decisions,omitempty"`

	// Metadata.
	RecommendedNextRun *flexTime `json:"recommended_next_run,omitempty"`
}

// CoverageBlock handles both schema 1.1 and 2.0 coverage shapes.
type CoverageBlock struct {
	// Schema 2.0 fields.
	TotalControls    int     `json:"total_controls,omitempty"`
	EvidencedPct     float64 `json:"evidenced_pct,omitempty"`
	ImplementedPct   float64 `json:"implemented_pct,omitempty"`
	GapPct           float64 `json:"gap_pct,omitempty"`
	OwnerGapCount    int     `json:"owner_gap_count,omitempty"`
	EvidenceGapCount int     `json:"evidence_gap_count,omitempty"`

	// Schema 1.1 fields (legacy).
	Total       int `json:"total,omitempty"`
	Evidenced   int `json:"evidenced,omitempty"`
	Implemented int `json:"implemented,omitempty"`
	Gaps        int `json:"gaps,omitempty"`
}

// RiskEntry is a minimal risk record from the run state.
type RiskEntry struct {
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

// EvidenceWindow is an upcoming evidence event from the run state.
type EvidenceWindow struct {
	DueDate time.Time `json:"due_date"`
}

// Load reads a run state JSON from disk.
func Load(path string) (*RunState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading run state %q: %w", path, err)
	}
	var rs RunState
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("parsing run state %q: %w", path, err)
	}
	return &rs, nil
}

// Compute builds a Snapshot from the run state.
func Compute(rs *RunState) Snapshot {
	now := time.Now().UTC()
	s := Snapshot{
		Program:     rs.Program,
		GeneratedAt: now,
	}

	s.Coverage = computeCoverage(rs)
	s.Risks = computeRisks(rs)
	s.Evidence = computeEvidence(rs, now)
	s.Decisions = computeDecisions(rs)
	s.Blockers = rs.Blockers

	s.OverallStatus = aggregate(s.Coverage.Status, s.Risks.Status, s.Evidence.Status, s.Decisions.Status)

	// Surface flags from grey/missing data.
	if s.Coverage.TotalControls == 0 {
		s.Flags = append(s.Flags, "[DATA UNAVAILABLE: coverage — run titer to populate]")
	}
	if s.Evidence.LastRunDate == nil {
		s.Flags = append(s.Flags, "[DATA UNAVAILABLE: run_date — run_state has no run_date field]")
	}

	sort.Strings(s.Flags)
	return s
}

func computeCoverage(rs *RunState) CoverageHealth {
	if rs.Coverage == nil {
		return CoverageHealth{Status: StatusGrey}
	}
	c := rs.Coverage
	ch := CoverageHealth{
		TotalControls:    coalesce(c.TotalControls, c.Total),
		OwnerGapCount:    c.OwnerGapCount,
		EvidenceGapCount: coalesce(c.EvidenceGapCount, c.Gaps),
	}
	if ch.TotalControls > 0 {
		ch.EvidencedPct = coalesceF(c.EvidencedPct, pct(c.Evidenced, ch.TotalControls))
		ch.ImplementedPct = coalesceF(c.ImplementedPct, pct(c.Implemented, ch.TotalControls))
		ch.GapPct = coalesceF(c.GapPct, pct(c.Gaps, ch.TotalControls))
	}

	switch {
	case ch.EvidencedPct >= 80:
		ch.Status = StatusGreen
	case ch.EvidencedPct >= 60:
		ch.Status = StatusYellow
	default:
		ch.Status = StatusRed
	}
	return ch
}

func computeRisks(rs *RunState) RiskHealth {
	rh := RiskHealth{}
	for _, r := range rs.Risks {
		rh.Total++
		switch r.Severity {
		case "critical":
			rh.Critical++
		case "high":
			rh.High++
		}
		switch r.Status {
		case "open":
			rh.Open++
		case "accepted":
			rh.Accepted++
		}
	}

	switch {
	case rh.Critical > 0:
		rh.Status = StatusRed
	case rh.High > 2:
		rh.Status = StatusYellow
	case len(rs.Risks) == 0:
		rh.Status = StatusGrey
	default:
		rh.Status = StatusGreen
	}
	return rh
}

func computeEvidence(rs *RunState, now time.Time) EvidenceHealth {
	eh := EvidenceHealth{}

	runDate := rs.RunDate
	if runDate == nil {
		runDate = rs.UpdatedAt
	}
	if runDate != nil {
		t := runDate.Time()
		eh.LastRunDate = &t
		days := int(now.Sub(t).Hours() / 24)
		eh.DaysSinceRun = &days
		if rs.RecommendedNextRun != nil && now.After(rs.RecommendedNextRun.Time()) {
			eh.IsStale = true
		} else if days > 90 {
			eh.IsStale = true
		}
	}

	for _, w := range rs.EvidenceWindows {
		if w.DueDate.Before(now) {
			eh.OverdueCount++
		} else if w.DueDate.Before(now.Add(30 * 24 * time.Hour)) {
			eh.DueSoonCount++
		}
	}

	switch {
	case eh.OverdueCount > 0 || eh.IsStale:
		eh.Status = StatusRed
	case eh.DueSoonCount > 0:
		eh.Status = StatusYellow
	case runDate == nil:
		eh.Status = StatusGrey
	default:
		eh.Status = StatusGreen
	}
	return eh
}

func computeDecisions(rs *RunState) DecisionHealth {
	dh := DecisionHealth{}
	dh.BlockerCount = len(rs.Blockers)
	for _, d := range rs.Decisions {
		if d.Deferred {
			dh.DeferredCount++
		}
	}

	switch {
	case dh.BlockerCount > 0:
		dh.Status = StatusRed
	case dh.DeferredCount > 3:
		dh.Status = StatusYellow
	default:
		dh.Status = StatusGreen
	}
	return dh
}

// aggregate returns the worst status across a list.
func aggregate(statuses ...Status) Status {
	worst := StatusGreen
	order := map[Status]int{StatusGreen: 0, StatusGrey: 1, StatusYellow: 2, StatusRed: 3}
	for _, s := range statuses {
		if order[s] > order[worst] {
			worst = s
		}
	}
	return worst
}

func coalesce(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

func coalesceF(a, b float64) float64 {
	if a != 0 {
		return a
	}
	return b
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
