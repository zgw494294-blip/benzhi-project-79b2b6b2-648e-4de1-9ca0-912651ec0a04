package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"casecheck/internal/model"
	"casecheck/internal/store"
)

const defaultLedgerPath = "casecheck.json"

type tagValues []string

func (values *tagValues) String() string {
	return strings.Join(*values, ",")
}

func (values *tagValues) Set(value string) error {
	parts := strings.Split(value, ",")
	*values = append(*values, parts...)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, output io.Writer) int {
	if len(args) == 0 {
		return writeJSON(output, map[string]any{
			"ok":       true,
			"commands": []string{"open", "scan", "seal", "show", "smoke"},
			"ledger":   defaultLedgerPath,
		})
	}

	ledgerPath, remaining, err := parseGlobalLedger(args)
	if err != nil {
		return writeError(output, err)
	}
	if len(remaining) == 0 {
		return writeError(output, fmt.Errorf("a command is required"))
	}
	command := remaining[0]
	commandArgs := remaining[1:]
	switch command {
	case "open":
		return runOpen(commandArgs, ledgerPath, output)
	case "scan":
		return runScan(commandArgs, ledgerPath, output)
	case "seal":
		return runSeal(commandArgs, ledgerPath, output)
	case "show":
		return runShow(commandArgs, ledgerPath, output)
	case "smoke":
		return runSmoke(commandArgs, output)
	case "help", "--help", "-h":
		return writeJSON(output, map[string]any{
			"ok":       true,
			"commands": []string{"open", "scan", "seal", "show", "smoke"},
			"ledger":   defaultLedgerPath,
		})
	default:
		return writeError(output, fmt.Errorf("unknown command %q", command))
	}
}

func parseGlobalLedger(args []string) (string, []string, error) {
	if len(args) == 0 {
		return defaultLedgerPath, args, nil
	}
	if args[0] == "--ledger" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return "", nil, fmt.Errorf("--ledger requires a file path")
		}
		return args[1], args[2:], nil
	}
	if strings.HasPrefix(args[0], "--ledger=") {
		path := strings.TrimPrefix(args[0], "--ledger=")
		if strings.TrimSpace(path) == "" {
			return "", nil, fmt.Errorf("--ledger requires a file path")
		}
		return path, args[1:], nil
	}
	return defaultLedgerPath, args, nil
}

func runOpen(args []string, ledgerPath string, output io.Writer) int {
	fs := newFlagSet("open")
	caseID, caseAlias := caseFlags(fs)
	var manifest tagValues
	fs.Var(&manifest, "manifest", "ordered comma-separated asset tags")
	fs.Var(&manifest, "asset", "one asset tag; may be repeated")
	fs.StringVar(&ledgerPath, "ledger", ledgerPath, "ledger JSON file")
	if err := fs.Parse(args); err != nil {
		return writeError(output, err)
	}
	if fs.NArg() != 0 {
		return writeError(output, fmt.Errorf("open does not accept positional arguments"))
	}
	caseIDValue, err := chooseCaseID(*caseID, *caseAlias)
	if err != nil {
		return writeError(output, err)
	}
	if len(manifest) == 0 {
		return writeError(output, fmt.Errorf("at least one --manifest or --asset is required"))
	}
	ledgerStore, err := store.New(ledgerPath)
	if err != nil {
		return writeError(output, err)
	}
	ledger, err := ledgerStore.Load()
	if err != nil {
		return writeError(output, err)
	}
	inspection, err := ledger.OpenInspection(caseIDValue, []string(manifest))
	if err != nil {
		return writeError(output, err)
	}
	if err := ledgerStore.Save(ledger); err != nil {
		return writeError(output, err)
	}
	return writeJSON(output, map[string]any{"ok": true, "inspection": inspection})
}

func runScan(args []string, ledgerPath string, output io.Writer) int {
	fs := newFlagSet("scan")
	caseID, caseAlias := caseFlags(fs)
	assetTag := fs.String("asset-tag", "", "expected asset tag")
	assetAlias := fs.String("asset", "", "expected asset tag")
	condition := fs.String("condition", "", "present, missing, or damaged")
	note := fs.String("note", "", "optional observation note")
	fs.StringVar(&ledgerPath, "ledger", ledgerPath, "ledger JSON file")
	if err := fs.Parse(args); err != nil {
		return writeError(output, err)
	}
	if fs.NArg() != 0 {
		return writeError(output, fmt.Errorf("scan does not accept positional arguments"))
	}
	caseIDValue, err := chooseCaseID(*caseID, *caseAlias)
	if err != nil {
		return writeError(output, err)
	}
	if *assetTag != "" && *assetAlias != "" && strings.TrimSpace(*assetTag) != strings.TrimSpace(*assetAlias) {
		return writeError(output, fmt.Errorf("--asset-tag and --asset identify different assets"))
	}
	if *assetTag == "" {
		*assetTag = *assetAlias
	}
	if strings.TrimSpace(*assetTag) == "" {
		return writeError(output, fmt.Errorf("--asset-tag is required"))
	}
	var observationNote *string
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == "note" {
			value := *note
			observationNote = &value
		}
	})
	ledgerStore, err := store.New(ledgerPath)
	if err != nil {
		return writeError(output, err)
	}
	ledger, err := ledgerStore.Load()
	if err != nil {
		return writeError(output, err)
	}
	inspection, err := ledger.Scan(caseIDValue, *assetTag, model.Condition(strings.ToLower(strings.TrimSpace(*condition))), observationNote)
	if err != nil {
		return writeError(output, err)
	}
	if err := ledgerStore.Save(ledger); err != nil {
		return writeError(output, err)
	}
	return writeJSON(output, map[string]any{"ok": true, "inspection": inspection})
}

func runSeal(args []string, ledgerPath string, output io.Writer) int {
	fs := newFlagSet("seal")
	caseID, caseAlias := caseFlags(fs)
	fs.StringVar(&ledgerPath, "ledger", ledgerPath, "ledger JSON file")
	if err := fs.Parse(args); err != nil {
		return writeError(output, err)
	}
	if fs.NArg() != 0 {
		return writeError(output, fmt.Errorf("seal does not accept positional arguments"))
	}
	caseIDValue, err := chooseCaseID(*caseID, *caseAlias)
	if err != nil {
		return writeError(output, err)
	}
	ledgerStore, err := store.New(ledgerPath)
	if err != nil {
		return writeError(output, err)
	}
	ledger, err := ledgerStore.Load()
	if err != nil {
		return writeError(output, err)
	}
	report, err := ledger.Seal(caseIDValue, timeNow())
	if err != nil {
		return writeError(output, err)
	}
	if err := ledgerStore.Save(ledger); err != nil {
		return writeError(output, err)
	}
	return writeJSON(output, map[string]any{"ok": true, "report": report})
}

func runShow(args []string, ledgerPath string, output io.Writer) int {
	fs := newFlagSet("show")
	caseID, caseAlias := caseFlags(fs)
	fs.StringVar(&ledgerPath, "ledger", ledgerPath, "ledger JSON file")
	if err := fs.Parse(args); err != nil {
		return writeError(output, err)
	}
	if fs.NArg() != 0 {
		return writeError(output, fmt.Errorf("show does not accept positional arguments"))
	}
	caseIDValue, err := chooseCaseID(*caseID, *caseAlias)
	if err != nil {
		return writeError(output, err)
	}
	ledgerStore, err := store.New(ledgerPath)
	if err != nil {
		return writeError(output, err)
	}
	ledger, err := ledgerStore.Load()
	if err != nil {
		return writeError(output, err)
	}
	inspection, err := ledger.Find(caseIDValue)
	if err != nil {
		return writeError(output, err)
	}
	if inspection.Report == nil {
		return writeError(output, fmt.Errorf("case %q has not been sealed", caseIDValue))
	}
	return writeJSON(output, map[string]any{"ok": true, "report": *inspection.Report})
}

func runSmoke(args []string, output io.Writer) int {
	if len(args) != 0 {
		return writeError(output, fmt.Errorf("smoke does not accept arguments"))
	}
	temporaryDirectory, err := os.MkdirTemp("", "casecheck-smoke-")
	if err != nil {
		return writeError(output, fmt.Errorf("create smoke workspace: %w", err))
	}
	defer os.RemoveAll(temporaryDirectory)
	ledgerStore, err := store.New(filepath.Join(temporaryDirectory, "ledger.json"))
	if err != nil {
		return writeError(output, err)
	}
	ledger := model.NewLedger()
	manifest := []string{"MIC-01", "CABLE-02", "ADAPTER-03"}
	if _, err := ledger.OpenInspection("SMOKE-CASE", manifest); err != nil {
		return writeError(output, err)
	}
	if err := ledgerStore.Save(ledger); err != nil {
		return writeError(output, err)
	}
	for _, tag := range manifest {
		ledger, err = ledgerStore.Load()
		if err != nil {
			return writeError(output, err)
		}
		if _, err := ledger.Scan("SMOKE-CASE", tag, model.Present, nil); err != nil {
			return writeError(output, err)
		}
		if err := ledgerStore.Save(ledger); err != nil {
			return writeError(output, err)
		}
	}
	ledger, err = ledgerStore.Load()
	if err != nil {
		return writeError(output, err)
	}
	report, err := ledger.Seal("SMOKE-CASE", timeNow())
	if err != nil {
		return writeError(output, err)
	}
	if err := ledgerStore.Save(ledger); err != nil {
		return writeError(output, err)
	}
	ledger, err = ledgerStore.Load()
	if err != nil {
		return writeError(output, err)
	}
	inspection, err := ledger.Find("SMOKE-CASE")
	if err != nil {
		return writeError(output, err)
	}
	if inspection.Report == nil || inspection.Report.Result != model.Cleared || inspection.Report.Result != report.Result {
		return writeError(output, fmt.Errorf("smoke report did not clear the complete inspection"))
	}
	return writeJSON(output, map[string]any{"ok": true, "report": *inspection.Report})
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func caseFlags(flags *flag.FlagSet) (*string, *string) {
	return flags.String("case-id", "", "case identifier"), flags.String("case", "", "case identifier")
}

func chooseCaseID(primary, alias string) (string, error) {
	if primary != "" && alias != "" && strings.TrimSpace(primary) != strings.TrimSpace(alias) {
		return "", fmt.Errorf("--case-id and --case identify different cases")
	}
	if primary == "" {
		primary = alias
	}
	if strings.TrimSpace(primary) == "" {
		return "", fmt.Errorf("--case-id is required")
	}
	return primary, nil
}

func writeError(output io.Writer, err error) int {
	writeJSON(output, map[string]any{"ok": false, "error": err.Error()})
	return 1
}

func writeJSON(output io.Writer, value any) int {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return 1
	}
	return 0
}

var timeNow = func() time.Time {
	return time.Now().UTC()
}
