package model

import (
	"fmt"
	"strings"
	"time"
)

const LedgerVersion = 1

type Condition string

const (
	Present Condition = "present"
	Missing Condition = "missing"
	Damaged Condition = "damaged"
)

type Status string

const (
	Open   Status = "open"
	Sealed Status = "sealed"
)

type Result string

const (
	Cleared Result = "cleared"
	Hold    Result = "hold"
)

type Observation struct {
	AssetTag  string    `json:"asset_tag"`
	Condition Condition `json:"condition"`
	Note      *string   `json:"note"`
}

type Report struct {
	CaseID       string        `json:"case_id"`
	Manifest     []string      `json:"manifest"`
	Observations []Observation `json:"observations"`
	Result       Result        `json:"result"`
	SealedAt     string        `json:"sealed_at"`
}

type Inspection struct {
	CaseID       string        `json:"case_id"`
	Manifest     []string      `json:"manifest"`
	Observations []Observation `json:"observations"`
	Status       Status        `json:"status"`
	Report       *Report       `json:"report"`
}

type Ledger struct {
	Version     int          `json:"version"`
	Inspections []Inspection `json:"inspections"`
}

func NewLedger() Ledger {
	return Ledger{Version: LedgerVersion, Inspections: []Inspection{}}
}

func NewInspection(caseID string, manifest []string) (Inspection, error) {
	caseID = strings.TrimSpace(caseID)
	if caseID == "" {
		return Inspection{}, fmt.Errorf("case identifier must not be blank")
	}
	cleanManifest, err := validateManifest(manifest)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{
		CaseID:       caseID,
		Manifest:     cleanManifest,
		Observations: []Observation{},
		Status:       Open,
		Report:       nil,
	}, nil
}

func (l *Ledger) OpenInspection(caseID string, manifest []string) (Inspection, error) {
	if err := l.Validate(); err != nil {
		return Inspection{}, err
	}
	caseID = strings.TrimSpace(caseID)
	for _, inspection := range l.Inspections {
		if inspection.CaseID == caseID {
			return Inspection{}, fmt.Errorf("case %q already exists", caseID)
		}
	}
	inspection, err := NewInspection(caseID, manifest)
	if err != nil {
		return Inspection{}, err
	}
	l.Inspections = append(l.Inspections, inspection)
	return cloneInspection(inspection), nil
}

func (l *Ledger) Scan(caseID, assetTag string, condition Condition, note *string) (Inspection, error) {
	if err := l.Validate(); err != nil {
		return Inspection{}, err
	}
	caseID = strings.TrimSpace(caseID)
	assetTag = strings.TrimSpace(assetTag)
	if caseID == "" {
		return Inspection{}, fmt.Errorf("case identifier must not be blank")
	}
	if assetTag == "" {
		return Inspection{}, fmt.Errorf("asset tag must not be blank")
	}
	if !validCondition(condition) {
		return Inspection{}, fmt.Errorf("unsupported condition %q", condition)
	}
	if condition == Damaged && (note == nil || strings.TrimSpace(*note) == "") {
		return Inspection{}, fmt.Errorf("damaged observations require a non-blank note")
	}
	for index := range l.Inspections {
		inspection := &l.Inspections[index]
		if inspection.CaseID != caseID {
			continue
		}
		if inspection.Status != Open {
			return Inspection{}, fmt.Errorf("case %q is already sealed", caseID)
		}
		if !contains(inspection.Manifest, assetTag) {
			return Inspection{}, fmt.Errorf("asset %q is not in case %q manifest", assetTag, caseID)
		}
		for _, observation := range inspection.Observations {
			if observation.AssetTag == assetTag {
				return Inspection{}, fmt.Errorf("asset %q was already scanned", assetTag)
			}
		}
		inspection.Observations = append(inspection.Observations, Observation{
			AssetTag:  assetTag,
			Condition: condition,
			Note:      cloneStringPointer(note),
		})
		return cloneInspection(*inspection), nil
	}
	return Inspection{}, fmt.Errorf("case %q not found", caseID)
}

func (l *Ledger) Seal(caseID string, sealedAt time.Time) (Report, error) {
	if err := l.Validate(); err != nil {
		return Report{}, err
	}
	caseID = strings.TrimSpace(caseID)
	for index := range l.Inspections {
		inspection := &l.Inspections[index]
		if inspection.CaseID != caseID {
			continue
		}
		if inspection.Status != Open {
			return Report{}, fmt.Errorf("case %q is already sealed", caseID)
		}
		if len(inspection.Observations) != len(inspection.Manifest) {
			return Report{}, fmt.Errorf("case %q is incomplete: scanned %d of %d assets", caseID, len(inspection.Observations), len(inspection.Manifest))
		}
		if sealedAt.IsZero() {
			sealedAt = time.Now().UTC()
		}
		sealedAt = sealedAt.UTC()
		result := Cleared
		for _, observation := range inspection.Observations {
			if observation.Condition != Present {
				result = Hold
				break
			}
		}
		report := Report{
			CaseID:       inspection.CaseID,
			Manifest:     append([]string(nil), inspection.Manifest...),
			Observations: cloneObservations(inspection.Observations),
			Result:       result,
			SealedAt:     sealedAt.Format(time.RFC3339Nano),
		}
		if err := validateReport(report, *inspection); err != nil {
			return Report{}, err
		}
		inspection.Status = Sealed
		inspection.Report = &report
		return cloneReport(report), nil
	}
	return Report{}, fmt.Errorf("case %q not found", caseID)
}

func (l Ledger) Find(caseID string) (Inspection, error) {
	if err := l.Validate(); err != nil {
		return Inspection{}, err
	}
	caseID = strings.TrimSpace(caseID)
	for _, inspection := range l.Inspections {
		if inspection.CaseID == caseID {
			return cloneInspection(inspection), nil
		}
	}
	return Inspection{}, fmt.Errorf("case %q not found", caseID)
}

func (l Ledger) Validate() error {
	if l.Version != LedgerVersion {
		return fmt.Errorf("unsupported ledger version %d", l.Version)
	}
	seenCases := make(map[string]struct{}, len(l.Inspections))
	for index, inspection := range l.Inspections {
		if err := inspection.validate(); err != nil {
			return fmt.Errorf("inspection %d: %w", index, err)
		}
		if _, exists := seenCases[inspection.CaseID]; exists {
			return fmt.Errorf("duplicate case %q", inspection.CaseID)
		}
		seenCases[inspection.CaseID] = struct{}{}
	}
	return nil
}

func (i Inspection) validate() error {
	if strings.TrimSpace(i.CaseID) == "" {
		return fmt.Errorf("case identifier must not be blank")
	}
	if _, err := validateManifest(i.Manifest); err != nil {
		return err
	}
	if i.Status != Open && i.Status != Sealed {
		return fmt.Errorf("unsupported status %q", i.Status)
	}
	seenAssets := make(map[string]struct{}, len(i.Observations))
	for index, observation := range i.Observations {
		if err := validateObservation(observation); err != nil {
			return fmt.Errorf("observation %d: %w", index, err)
		}
		if !contains(i.Manifest, observation.AssetTag) {
			return fmt.Errorf("observation asset %q is not in manifest", observation.AssetTag)
		}
		if _, exists := seenAssets[observation.AssetTag]; exists {
			return fmt.Errorf("asset %q was scanned more than once", observation.AssetTag)
		}
		seenAssets[observation.AssetTag] = struct{}{}
	}
	if i.Status == Open && i.Report != nil {
		return fmt.Errorf("open inspection must not have a report")
	}
	if i.Status == Sealed {
		if i.Report == nil {
			return fmt.Errorf("sealed inspection must have a report")
		}
		if len(i.Observations) != len(i.Manifest) {
			return fmt.Errorf("sealed inspection must be complete")
		}
		if err := validateReport(*i.Report, i); err != nil {
			return err
		}
	}
	return nil
}

func validateManifest(manifest []string) ([]string, error) {
	if len(manifest) == 0 {
		return nil, fmt.Errorf("manifest must contain at least one asset tag")
	}
	clean := make([]string, len(manifest))
	seen := make(map[string]struct{}, len(manifest))
	for index, tag := range manifest {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return nil, fmt.Errorf("manifest asset %d must not be blank", index+1)
		}
		if _, exists := seen[tag]; exists {
			return nil, fmt.Errorf("manifest contains duplicate asset tag %q", tag)
		}
		seen[tag] = struct{}{}
		clean[index] = tag
	}
	return clean, nil
}

func validateObservation(observation Observation) error {
	if strings.TrimSpace(observation.AssetTag) == "" {
		return fmt.Errorf("asset tag must not be blank")
	}
	if !validCondition(observation.Condition) {
		return fmt.Errorf("unsupported condition %q", observation.Condition)
	}
	if observation.Condition == Damaged && (observation.Note == nil || strings.TrimSpace(*observation.Note) == "") {
		return fmt.Errorf("damaged observations require a non-blank note")
	}
	return nil
}

func validateReport(report Report, inspection Inspection) error {
	if report.CaseID != inspection.CaseID {
		return fmt.Errorf("report case identifier does not match inspection")
	}
	if !sameStrings(report.Manifest, inspection.Manifest) {
		return fmt.Errorf("report manifest does not match inspection")
	}
	if !sameObservations(report.Observations, inspection.Observations) {
		return fmt.Errorf("report observations do not match inspection")
	}
	if report.Result != Cleared && report.Result != Hold {
		return fmt.Errorf("unsupported report result %q", report.Result)
	}
	if _, err := time.Parse(time.RFC3339Nano, report.SealedAt); err != nil {
		return fmt.Errorf("invalid report seal time: %w", err)
	}
	derived := Cleared
	for _, observation := range report.Observations {
		if observation.Condition != Present {
			derived = Hold
			break
		}
	}
	if report.Result != derived {
		return fmt.Errorf("report result does not match observations")
	}
	return nil
}

func validCondition(condition Condition) bool {
	return condition == Present || condition == Missing || condition == Damaged
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameObservations(left, right []Observation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].AssetTag != right[index].AssetTag || left[index].Condition != right[index].Condition || !sameOptionalString(left[index].Note, right[index].Note) {
			return false
		}
	}
	return true
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func cloneInspection(inspection Inspection) Inspection {
	clone := inspection
	clone.Manifest = append([]string(nil), inspection.Manifest...)
	clone.Observations = cloneObservations(inspection.Observations)
	if inspection.Report != nil {
		report := cloneReport(*inspection.Report)
		clone.Report = &report
	}
	return clone
}

func cloneReport(report Report) Report {
	clone := report
	clone.Manifest = append([]string(nil), report.Manifest...)
	clone.Observations = cloneObservations(report.Observations)
	return clone
}

func cloneObservations(observations []Observation) []Observation {
	clone := make([]Observation, len(observations))
	for index, observation := range observations {
		clone[index] = observation
		clone[index].Note = cloneStringPointer(observation.Note)
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
