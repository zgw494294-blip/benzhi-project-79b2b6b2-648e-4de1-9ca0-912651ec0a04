package model

import (
	"strings"
	"testing"
	"time"
)

func TestNewInspectionPreservesManifestOrderAndRejectsInvalidTags(t *testing.T) {
	inspection, err := NewInspection("CASE-1", []string{"  AMP-1 ", "CABLE-2"})
	if err != nil {
		t.Fatalf("NewInspection() error = %v", err)
	}
	if want := []string{"AMP-1", "CABLE-2"}; !sameStrings(inspection.Manifest, want) {
		t.Fatalf("manifest = %#v, want %#v", inspection.Manifest, want)
	}

	for _, manifest := range [][]string{{}, {"AMP-1", "AMP-1"}, {"AMP-1", "  "}} {
		if _, err := NewInspection("CASE-1", manifest); err == nil {
			t.Errorf("NewInspection(%#v) accepted invalid manifest", manifest)
		}
	}
}

func TestScanPreservesAbsentAndSuppliedNotes(t *testing.T) {
	ledger := NewLedger()
	if _, err := ledger.OpenInspection("CASE-1", []string{"A", "B", "C"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Scan("CASE-1", "A", Present, nil); err != nil {
		t.Fatal(err)
	}
	emptyNote := ""
	if _, err := ledger.Scan("CASE-1", "B", Missing, &emptyNote); err != nil {
		t.Fatal(err)
	}
	damageNote := "housing scratched"
	if _, err := ledger.Scan("CASE-1", "C", Damaged, &damageNote); err != nil {
		t.Fatal(err)
	}
	inspection, err := ledger.Find("CASE-1")
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Observations[0].Note != nil {
		t.Fatal("absent note was not retained as nil")
	}
	if inspection.Observations[1].Note == nil || *inspection.Observations[1].Note != "" {
		t.Fatal("supplied empty note was not retained")
	}
	if inspection.Observations[2].Note == nil || *inspection.Observations[2].Note != damageNote {
		t.Fatal("damage note was not retained")
	}
}

func TestDamagedObservationRequiresNonBlankNote(t *testing.T) {
	for _, note := range []*string{nil, stringPointer("  ")} {
		ledger := NewLedger()
		if _, err := ledger.OpenInspection("CASE-1", []string{"A"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.Scan("CASE-1", "A", Damaged, note); err == nil || !strings.Contains(err.Error(), "non-blank note") {
			t.Errorf("Scan() error = %v, want non-blank note error", err)
		}
	}
}

func TestSealDerivesResultAndLocksInspection(t *testing.T) {
	sealedAt := time.Date(2026, time.August, 17, 12, 30, 0, 0, time.UTC)
	ledger := NewLedger()
	if _, err := ledger.OpenInspection("CASE-1", []string{"A", "B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Seal("CASE-1", sealedAt); err == nil {
		t.Fatal("Seal() accepted incomplete inspection")
	}
	if _, err := ledger.Scan("CASE-1", "A", Present, nil); err != nil {
		t.Fatal(err)
	}
	note := "not returned"
	if _, err := ledger.Scan("CASE-1", "B", Missing, &note); err != nil {
		t.Fatal(err)
	}
	report, err := ledger.Seal("CASE-1", sealedAt)
	if err != nil {
		t.Fatal(err)
	}
	if report.Result != Hold || report.SealedAt != sealedAt.Format(time.RFC3339Nano) {
		t.Fatalf("report = %#v", report)
	}
	if _, err := ledger.Scan("CASE-1", "A", Present, nil); err == nil {
		t.Fatal("Scan() accepted sealed inspection")
	}
	if _, err := ledger.Seal("CASE-1", sealedAt); err == nil {
		t.Fatal("Seal() accepted repeated sealing")
	}
	if err := ledger.Validate(); err != nil {
		t.Fatalf("sealed ledger did not validate: %v", err)
	}
}

func TestSealClearsWhenAllAssetsArePresent(t *testing.T) {
	ledger := NewLedger()
	if _, err := ledger.OpenInspection("CASE-1", []string{"A", "B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Scan("CASE-1", "B", Present, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Scan("CASE-1", "A", Present, nil); err != nil {
		t.Fatal(err)
	}
	report, err := ledger.Seal("CASE-1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Result != Cleared {
		t.Fatalf("result = %q, want %q", report.Result, Cleared)
	}
}

func TestScanRejectsUnknownAndRepeatedAssets(t *testing.T) {
	ledger := NewLedger()
	if _, err := ledger.OpenInspection("CASE-1", []string{"A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Scan("CASE-1", "B", Present, nil); err == nil {
		t.Fatal("Scan() accepted unknown asset")
	}
	if _, err := ledger.Scan("CASE-1", "A", Present, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Scan("CASE-1", "A", Present, nil); err == nil {
		t.Fatal("Scan() accepted repeated asset")
	}
}

func stringPointer(value string) *string {
	return &value
}
