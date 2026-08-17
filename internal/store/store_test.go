package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"casecheck/internal/model"
)

func TestMissingLedgerLoadsAsEmptyVersion(t *testing.T) {
	ledgerStore, err := New(filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := ledgerStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ledger, model.NewLedger()) {
		t.Fatalf("ledger = %#v, want %#v", ledger, model.NewLedger())
	}
}

func TestSaveAndLoadRoundTripKeepsReportAndOptionalNotes(t *testing.T) {
	ledgerStore, err := New(filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	ledger := model.NewLedger()
	if _, err := ledger.OpenInspection("CASE-1", []string{"A", "B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Scan("CASE-1", "A", model.Present, nil); err != nil {
		t.Fatal(err)
	}
	note := ""
	if _, err := ledger.Scan("CASE-1", "B", model.Missing, &note); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Seal("CASE-1", time.Date(2026, time.August, 17, 12, 30, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := ledgerStore.Save(ledger); err != nil {
		t.Fatal(err)
	}
	loaded, err := ledgerStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, ledger) {
		t.Fatalf("loaded ledger differs:\nloaded=%#v\nwant=%#v", loaded, ledger)
	}
}

func TestLoadRejectsInvalidVersionAndTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"inspections":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerStore.Load(); err == nil || !strings.Contains(err.Error(), "unsupported ledger version") {
		t.Fatalf("Load() error = %v, want version error", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"inspections":[]} {}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerStore.Load(); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("Load() error = %v, want multiple-value error", err)
	}
}

func TestSaveCleansTemporaryFileWhenReplacementFails(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledgerStore.Save(model.NewLedger()); err == nil {
		t.Fatal("Save() succeeded when target was a directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".ledger.json.tmp-") {
			t.Fatalf("temporary file remained after failed save: %s", entry.Name())
		}
	}
}

func TestValidationFailureLeavesExistingLedgerUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	ledgerStore, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledgerStore.Save(model.NewLedger()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledgerStore.Save(model.Ledger{Version: 99}); err == nil {
		t.Fatal("Save() accepted an invalid ledger")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("invalid save changed the existing ledger")
	}
}
