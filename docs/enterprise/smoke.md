# Enterprise Smoke Coverage

Enterprise smoke tests cover representative agency workflows end-to-end:

- workspace context resolution
- approval request/approve flow
- governed execution for high-risk command
- fail-closed denied path when secret governance does not grant access

Smoke suite location:

- `/Users/Bayram/Developer/meta-marketing-cli/internal/enterprise/tests/smoke/enterprise_smoke_test.go`

Run locally:

```bash
go test ./internal/enterprise/tests/smoke -count=1 -v
```

CI coverage:

- existing `go test ./...` pipeline includes enterprise smoke package
- smoke tests therefore gate merges and protect against enterprise regressions
