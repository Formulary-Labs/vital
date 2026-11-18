package health_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Formulary-Labs/vital/health"
)

func TestCompute_greenProgram(t *testing.T) {
	rs := &health.RunState{
		Program: "test",
		Coverage: &health.CoverageBlock{
			TotalControls:  100,
			EvidencedPct:   85,
			ImplementedPct: 10,
			GapPct:         5,
		},
	}
	s := health.Compute(rs)
	if s.Coverage.Status != health.StatusGreen {
		t.Errorf("coverage status = %q, want green", s.Coverage.Status)
	}
}

func TestCompute_redCoverage(t *testing.T) {
	rs := &health.RunState{
		Program: "test",
		Coverage: &health.CoverageBlock{
			TotalControls: 100,
			EvidencedPct:  30,
		},
	}
	s := health.Compute(rs)
	if s.Coverage.Status != health.StatusRed {
		t.Errorf("coverage status = %q, want red", s.Coverage.Status)
	}
	if s.OverallStatus != health.StatusRed {
		t.Errorf("overall status = %q, want red", s.OverallStatus)
	}
}

func TestCompute_criticalRisk(t *testing.T) {
	rs := &health.RunState{
		Program: "test",
		Risks: []health.RiskEntry{
			{Severity: "critical", Status: "open"},
		},
	}
	s := health.Compute(rs)
	if s.Risks.Status != health.StatusRed {
		t.Errorf("risks status = %q, want red", s.Risks.Status)
	}
	if s.Risks.Critical != 1 {
		t.Errorf("critical count = %d, want 1", s.Risks.Critical)
	}
}

func TestCompute_staleEvidence(t *testing.T) {
	stale := health.FlexTime(time.Now().Add(-100 * 24 * time.Hour))
	rs := &health.RunState{
		Program: "test",
		RunDate: &stale,
	}
	s := health.Compute(rs)
	if !s.Evidence.IsStale {
		t.Error("expected evidence to be stale")
	}
	if s.Evidence.Status != health.StatusRed {
		t.Errorf("evidence status = %q, want red", s.Evidence.Status)
	}
}

func TestCompute_greyCoverage(t *testing.T) {
	rs := &health.RunState{Program: "test"}
	s := health.Compute(rs)
	if s.Coverage.Status != health.StatusGrey {
		t.Errorf("coverage status = %q, want grey (no coverage data)", s.Coverage.Status)
	}
}

func TestLoad(t *testing.T) {
	rs := &health.RunState{
		Program:  "test",
		Coverage: &health.CoverageBlock{TotalControls: 50, EvidencedPct: 70},
	}
	data, _ := json.Marshal(rs)
	dir := t.TempDir()
	path := filepath.Join(dir, "latest.json")
	_ = os.WriteFile(path, data, 0o600)

	loaded, err := health.Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Program != "test" {
		t.Errorf("program = %q, want %q", loaded.Program, "test")
	}
}

func TestLoad_liveControlCoverageAndRiskRegister(t *testing.T) {
	path := filepath.Join("..", "testdata", "live-shape.json")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join("testdata", "live-shape.json")
	}
	loaded, err := health.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Program != "iso42001" {
		t.Errorf("program = %q", loaded.Program)
	}
	if loaded.Coverage == nil || loaded.Coverage.TotalControls != 68 {
		t.Fatalf("coverage = %+v, want TotalControls=68", loaded.Coverage)
	}
	if loaded.Coverage.EvidencedPct != 22 {
		t.Errorf("evidenced_pct = %v, want 22", loaded.Coverage.EvidencedPct)
	}
	if loaded.Coverage.OwnerGapCount != 2 || loaded.Coverage.EvidenceGapCount != 1 {
		t.Errorf("gaps owner=%d evidence=%d", loaded.Coverage.OwnerGapCount, loaded.Coverage.EvidenceGapCount)
	}
	if len(loaded.Risks) != 3 {
		t.Fatalf("risks len = %d, want 3", len(loaded.Risks))
	}
	s := health.Compute(loaded)
	if s.Coverage.Status == health.StatusGrey {
		t.Error("coverage should not be grey with control_coverage data")
	}
	if s.Risks.Status == health.StatusGrey {
		t.Error("risks should not be grey with risk_register.items")
	}
	if s.Risks.Critical != 1 || s.Risks.High != 1 || s.Risks.Open != 2 {
		t.Errorf("risks summary = %+v", s.Risks)
	}
}
