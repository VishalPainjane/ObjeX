# ObjeX — AI Context

Self-hosted S3-compatible object storage written in **Go**.

**Maintainer:** Vishal Painjane (painjanevishal2204@gmail.com)  
**Module:** `github.com/VishalPainjane/objex`

---

## Project Layout

```
cmd/objex/              # Main binary
internal/
├── api/                # S3 HTTP handler, routing, cluster forwarding
├── auth/               # AWS SigV4, presign, internal cluster token
├── cluster/            # Membership, placement, proxy, peer health
├── config/             # Env + cluster JSON config
├── jobs/               # Orphan cleanup, multipart cleanup, integrity verify
├── metadata/sqlite/    # Buckets, objects, multipart, hints, credentials
├── metrics/            # Prometheus
├── object/             # Domain service (PUT/GET/DELETE, multipart, copy)
├── quorum/             # N/R/W quorum math
├── replication/        # Coordinator, replicator, hints worker, healing
├── s3/                 # XML responses + batch delete parsing
├── storage/filesystem/ # Content-addressable blob store
└── validation/         # Bucket/key validators
configs/                # Sample cluster JSON
docs/
├── architecture/       # Technical design docs
└── showcase/           # Portfolio case study + GitHub Pages site
```

---

## Run Locally

```bash
go run ./cmd/objex
# S3 API: http://localhost:9000
# Health:  http://localhost:9000/health/ready
# Metrics: http://localhost:9000/metrics
```

Docker: `docker compose up -d`

Cluster: `docker compose -f docker-compose.cluster.yml up -d`

---

## S3 API (port 9000)

SigV4 on all routes except `/health*`, `/metrics`, `/cluster` (debug).

| Method | Path | Action |
|--------|------|--------|
| GET | `/` | List buckets |
| PUT/DELETE/HEAD/GET | `/{bucket}` | Bucket ops / list objects |
| PUT/GET/HEAD/DELETE | `/{bucket}/{key}` | Object ops |
| POST | `/{bucket}?delete` | Batch delete (XML body) |
| GET | `/api/presign/{bucket}/{key}` | Presigned URL helper |

Multipart: `?uploads`, `?uploadId`, `?partNumber`. Copy: `x-amz-copy-source`.

---

## Cluster / Replication

- Static membership via `OBJEX_CLUSTER_CONFIG` JSON
- Rendezvous hashing → primary + replica set
- Non-primary nodes forward object requests to primary (`cluster.Proxy`)
- `replication.Coordinator`: quorum PUT/DELETE, versioned tombstones, hints, read repair
- `cluster.PeerHealthTracker` + healing worker for partial replicas

Config: `OBJEX_REPLICATION_FACTOR`, `OBJEX_WRITE_QUORUM`, `OBJEX_READ_QUORUM`

---

## Storage

Physical path: `{base}/{bucket}/{L1}/{L2}/{sha256}.blob` where hash = SHA256(`bucket/key`).

Atomic write: `.tmp` → rename. ETag = MD5 hex (multipart: `{md5}-{n}`).

---

## Tests

```bash
go test ./...
go test -race ./...
```

Integration tests spin up `httptest` servers with in-memory SQLite + temp blob dirs. Cluster tests use multi-node env helpers in `internal/api/cluster_env_test.go`.

---

## CI

`.github/workflows/ci.yml` — test, vet, build, race on every push/PR.

---

## Roadmap

See [docs/architecture/roadmap.md](docs/architecture/roadmap.md).

Deferred: presigned POST, aws-chunked uploads, PostgreSQL metadata, erasure coding.
