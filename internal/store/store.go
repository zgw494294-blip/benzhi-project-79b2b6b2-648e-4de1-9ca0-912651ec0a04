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

func (s Store) Save(ledger model.Ledger) error {
	if err := ledger.Validate(); err != nil {
		return fmt.Errorf("validate ledger before save: %w", err)
	}
	lock, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open ledger lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock ledger: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	current, err := s.Load()
	if err != nil {
		return err
	}
	cases := make(map[string]struct{}, len(ledger.Inspections))
	for _, inspection := range ledger.Inspections {
		cases[inspection.CaseID] = struct{}{}
	}
	for _, inspection := range current.Inspections {
		if _, exists := cases[inspection.CaseID]; !exists {
			ledger.Inspections = append(ledger.Inspections, inspection)
		}
	}
	if err := ledger.Validate(); err != nil {
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
	if err := encoder.Encode(ledger); err != nil {
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
