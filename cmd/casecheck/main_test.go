package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"casecheck/internal/model"
)

func TestRunPersistsCompleteWorkflowAndShowsReport(t *testing.T) {
	ledgerPath := filepath.Join(t.TempDir(), "ledger.json")
	var output bytes.Buffer
	if code := run([]string{"--ledger", ledgerPath, "open", "--case-id", "ROAD-7", "--manifest", "AMP-1,CABLE-2", "--asset", "MIC-3"}, &output); code != 0 {
		t.Fatalf("open exit code = %d, output = %s", code, output.String())
	}
	for _, args := range [][]string{
		{"--ledger", ledgerPath, "scan", "--case-id", "ROAD-7", "--asset-tag", "AMP-1", "--condition", "present"},
		{"--ledger", ledgerPath, "scan", "--case-id", "ROAD-7", "--asset-tag", "CABLE-2", "--condition", "present", "--note", "clean"},
		{"--ledger", ledgerPath, "scan", "--case-id", "ROAD-7", "--asset-tag", "MIC-3", "--condition", "present"},
		{"--ledger", ledgerPath, "seal", "--case-id", "ROAD-7"},
	} {
		output.Reset()
		if code := run(args, &output); code != 0 {
			t.Fatalf("run(%v) exit code = %d, output = %s", args, code, output.String())
		}
	}
	output.Reset()
	if code := run([]string{"show", "--ledger", ledgerPath, "--case-id", "ROAD-7"}, &output); code != 0 {
		t.Fatalf("show exit code = %d, output = %s", code, output.String())
	}
	var response struct {
		OK     bool         `json:"ok"`
		Report model.Report `json:"report"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("show output is not JSON: %v", err)
	}
	if !response.OK || response.Report.Result != model.Cleared || len(response.Report.Observations) != 3 {
		t.Fatalf("show response = %#v", response)
	}
}

func TestRunReturnsJSONErrorForInvalidScan(t *testing.T) {
	var output bytes.Buffer
	if code := run([]string{"scan", "--case-id", "ROAD-7", "--asset-tag", "AMP-1", "--condition", "damaged"}, &output); code == 0 {
		t.Fatalf("scan exit code = %d, output = %s", code, output.String())
	}
	var response map[string]any
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("error output is not JSON: %v", err)
	}
	if response["ok"] != false || response["error"] == nil {
		t.Fatalf("error response = %#v", response)
	}
}
