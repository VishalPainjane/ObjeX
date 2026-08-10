# ObjeX — Self-Hosted S3-Compatible Object Storage

**Author:** [Vishal Painjane](https://github.com/VishalPainjane)  
**Stack:** Go · SQLite · filesystem storage · optional multi-node cluster  
**License:** MIT

ObjeX is a self-hosted blob store with an **AWS S3-compatible API** (SigV4), multipart uploads, presigned URLs, and an optional **distributed mode** with quorum replication, hinted handoff, read repair, and peer health monitoring.

| | |
|---|---|
| **Try locally** | `docker compose up -d` → S3 API at http://localhost:9000 |
| **3-node cluster** | `docker compose -f docker-compose.cluster.yml up -d` |
| **Case study** | [docs/showcase/case-study.md](docs/showcase/case-study.md) |
| **Architecture** | [docs/architecture/](docs/architecture/) |
| **Project site** | https://vishalpainjane.github.io/ObjeX/ *(enable GitHub Pages → Actions)* |

---

## Features

- S3 API: PUT / GET / HEAD / DELETE, listing, Range GET, batch delete, CopyObject
- Multipart uploads (initiate, parts, complete, abort, list)
- AWS Signature V4 + presigned GET/PUT
- Content-addressable blob layout (SHA-256 paths), atomic writes, MD5 ETags
- Background jobs: orphan cleanup, abandoned multipart cleanup, integrity verification
- **Cluster mode:** rendezvous placement, N/R/W quorum, replication hints, read repair, healing
- Prometheus metrics (`/metrics`), health endpoints, Docker + Compose

---

## Quick start (single node)

```bash
git clone https://github.com/VishalPainjane/ObjeX.git
cd ObjeX
docker compose up -d --build
```

Default dev credentials (change in production):

| | |
|---|---|
| Access key | `OBXTESTKEY00000001` |
| Secret key | `testsecretkeythatislongenoughforhmacsha256test` |
| Endpoint | `http://localhost:9000` |

```bash
# Create bucket + upload (AWS CLI)
aws --endpoint-url http://localhost:9000 s3 mb s3://photos
aws --endpoint-url http://localhost:9000 s3 cp hello.txt s3://photos/
```

Or use the helper script: `.\scripts\demo.ps1` (Windows) / `./scripts/demo.sh` (Linux/macOS).

---

## Development

```bash
go test ./...
go run ./cmd/objex
```

Environment variables (common):

| Variable | Default | Purpose |
|----------|---------|---------|
| `OBJEX_HTTP_ADDRESS` | `:9000` | Listen address |
| `OBJEX_DATA_DIR` | `./data` | SQLite + blobs root |
| `OBJEX_ACCESS_KEY_ID` | — | S3 access key |
| `OBJEX_SECRET_ACCESS_KEY` | — | S3 secret |
| `OBJEX_CLUSTER_CONFIG` | — | JSON cluster membership file |

See [docs/architecture/architecture.md](docs/architecture/architecture.md) and [docs/architecture/distributed-architecture.md](docs/architecture/distributed-architecture.md).

---

## Cluster demo (3 nodes)

```bash
docker compose -f docker-compose.cluster.yml up -d --build
# Node 1: http://localhost:9001  (/cluster for status)
# Node 2: http://localhost:9002
# Node 3: http://localhost:9003
```

---

## Showcase / interviews

Pin this repo and link the [case study](docs/showcase/case-study.md) on your resume. Enable **GitHub Pages** (Settings → Pages → Source: GitHub Actions) to publish the [landing page](docs/showcase/index.html).

---

## Contact

**Vishal Painjane** — painjanevishal2204@gmail.com  
GitHub: https://github.com/VishalPainjane
