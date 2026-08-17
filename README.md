# CaseCheck

CaseCheck is a small Go command-line tool for recording the condition of assets returned in a touring road case. An inspection starts with an ordered manifest, receives one observation for every expected tag, and ends as an immutable `cleared` or `hold` report.

The application uses only the Go standard library. Its local ledger is a versioned JSON file named `casecheck.json` by default. Each write is made through a sibling temporary file that is synced, closed, and atomically renamed into place.

## Commands

Use `--ledger PATH` before the command or with any command to select a ledger file. Output is JSON on both success and failure.

```text
casecheck open --case-id CASE-7 --manifest AMP-1,CABLE-2 --asset MIC-3
casecheck scan --case-id CASE-7 --asset-tag AMP-1 --condition present
casecheck scan --case-id CASE-7 --asset-tag CABLE-2 --condition damaged --note "connector bent"
casecheck scan --case-id CASE-7 --asset-tag MIC-3 --condition missing
casecheck seal --case-id CASE-7
casecheck show --case-id CASE-7
casecheck smoke
```

`--asset` is an alias for `--asset-tag` on `scan`. On `open`, repeat `--asset` or provide a comma-separated `--manifest`; the supplied order is retained. Conditions are `present`, `missing`, and `damaged`. A damaged observation needs a non-blank note. For other conditions, leaving out `--note` is different from supplying an empty note.

## Development

```text
go test ./...
go run ./cmd/casecheck smoke
```
