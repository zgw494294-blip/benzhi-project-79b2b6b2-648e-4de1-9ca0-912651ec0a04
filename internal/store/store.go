package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"casecheck/internal/model"
)

type Store struct {
	Path string
}

func New(path string) (Store, error) {
	path = filepath.Clean(path)
	if path == "." || path == "" {
		return Store{}, fmt.Errorf("ledger path must name a file")
	}
	return Store{Path: path}, nil
}

func (s Store) Load() (model.Ledger, error) {
	contents, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.NewLedger(), nil
		}
		return model.Ledger{}, fmt.Errorf("read ledger %q: %w", s.Path, err)
	}

	var ledger model.Ledger
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ledger); err != nil {
		return model.Ledger{}, fmt.Errorf("decode ledger %q: %w", s.Path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return model.Ledger{}, fmt.Errorf("decode ledger %q: multiple JSON values", s.Path)
		}
		return model.Ledger{}, fmt.Errorf("decode ledger %q: %w", s.Path, err)
	}
	if err := ledger.Validate(); err != nil {
		return model.Ledger{}, fmt.Errorf("validate ledger %q: %w", s.Path, err)
	}
	return ledger, nil
}

// Save writes ledger to disk as an atomic read-merge-write transaction. It
// acquires an exclusive file lock, reads the latest committed state, merges
// the supplied ledger with any inspections committed by concurrent writers,
// and replaces the ledger file atomically. Merging prevents the lost-update
// problem: a writer that loaded a stale snapshot still preserves inspections
// committed by other writers since its load.
func (s Store) Save(ledger model.Ledger) error {
	if err := ledger.Validate(); err != nil {
		return fmt.Errorf("validate ledger before save: %w", err)
	}

	lockFile, err := lockLedger(s.Path)
	if err != nil {
		return err
	}
	defer lockFile.Close()

	current, err := s.Load()
	if err != nil {
		return fmt.Errorf("load current ledger before save: %w", err)
	}
	merged := mergeLedgers(current, ledger)
	if err := merged.Validate(); err != nil {
		return fmt.Errorf("validate merged ledger before save: %w", err)
	}

	parent := filepath.Dir(s.Path)
	base := filepath.Base(s.Path)
	temporary, err := os.CreateTemp(parent, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create ledger temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(temporaryPath)
		}
	}()

	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(merged); err != nil {
		closeErr := temporary.Close()
		return errors.Join(fmt.Errorf("write ledger temporary file: %w", err), closeErr)
	}
	if err := temporary.Sync(); err != nil {
		closeErr := temporary.Close()
		return errors.Join(fmt.Errorf("sync ledger temporary file: %w", err), closeErr)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close ledger temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.Path); err != nil {
		return fmt.Errorf("replace ledger %q: %w", s.Path, err)
	}
	renamed = true
	return nil
}

// lockLedger opens a sibling lock file and acquires an advisory exclusive
// lock on it, serializing concurrent writers that share the same ledger path.
func lockLedger(path string) (*os.File, error) {
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open ledger lock %q: %w", lockPath, err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("lock ledger %q: %w", path, err)
	}
	return lockFile, nil
}

// mergeLedgers combines inspections from two ledger snapshots. Inspections in
// the updated snapshot take precedence over inspections in the current
// snapshot when they share the same case identifier; inspections only present
// in the current snapshot are preserved so that concurrent writes do not lose
// committed data.
func mergeLedgers(current, updated model.Ledger) model.Ledger {
	merged := model.Ledger{
		Version:     model.LedgerVersion,
		Inspections: make([]model.Inspection, 0, len(updated.Inspections)+len(current.Inspections)),
	}
	seen := make(map[string]struct{}, len(updated.Inspections))
	for index := range updated.Inspections {
		merged.Inspections = append(merged.Inspections, cloneInspection(updated.Inspections[index]))
		seen[updated.Inspections[index].CaseID] = struct{}{}
	}
	for index := range current.Inspections {
		if _, exists := seen[current.Inspections[index].CaseID]; !exists {
			merged.Inspections = append(merged.Inspections, cloneInspection(current.Inspections[index]))
		}
	}
	return merged
}

func cloneInspection(inspection model.Inspection) model.Inspection {
	clone := inspection
	clone.Manifest = append([]string(nil), inspection.Manifest...)
	clone.Observations = make([]model.Observation, len(inspection.Observations))
	for index := range inspection.Observations {
		clone.Observations[index] = inspection.Observations[index]
		clone.Observations[index].Note = cloneStringPointer(inspection.Observations[index].Note)
	}
	if inspection.Report != nil {
		report := cloneReport(*inspection.Report)
		clone.Report = &report
	}
	return clone
}

func cloneReport(report model.Report) model.Report {
	clone := report
	clone.Manifest = append([]string(nil), report.Manifest...)
	clone.Observations = make([]model.Observation, len(report.Observations))
	for index := range report.Observations {
		clone.Observations[index] = report.Observations[index]
		clone.Observations[index].Note = cloneStringPointer(report.Observations[index].Note)
	}
	return clone
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
