# Benchmark Baseline

Recorded on developer machine (Windows, Go 1.25). Use as a relative baseline before distributed work — not production claims.

## Environment

- OS: Windows 10/11
- CPU: 13th Gen Intel Core i7-13620H (16 logical cores)
- Go: 1.25.5
- Storage: local filesystem, SQLite metadata
- Auth: SigV4 enabled

## Methodology

Run from the repo root:

```bash
go test -bench=. -benchmem ./internal/auth/
```

## Results (2026-08-11)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkCanonicalRequest` | 1567 | 504 | 12 |
| `BenchmarkVerifySignature` | 5444 | 4003 | 59 |

Auth benchmarks (`internal/auth/`): ~910k ops/sec canonical, ~180k ops/sec verify on the dev machine above.

### Quorum coordinator (local, no peer I/O)

Run from the repo root:

```bash
go test -bench=BenchmarkQuorumPut -benchmem ./internal/replication/
```

Compares coordinator PUT overhead with W=2 vs W=3 on a single-node test harness (remote replicas unreachable). Use for relative comparison only.

## Limitations

- Single-node, no network latency
- No concurrent load generator
- Windows filesystem may differ from Linux production
- `go test -race` on Windows requires `CGO_ENABLED=1`

Re-run benchmarks after significant changes and update this file.
