# ObjeX | Interview Case Study

**Author:** Vishal Painjane · [GitHub](https://github.com/VishalPainjane) · painjanevishal2204@gmail.com

Use this doc for technical interviews, portfolio reviews, and recruiter screens.

---

## Elevator pitch (30 seconds)

> **ObjeX** is a self-hosted, S3-compatible object storage system I built in Go. It implements the core S3 API with AWS Signature V4, multipart uploads, and presigned URLs. I extended it with **distributed replication**: configurable quorum reads/writes (N/R/W), rendezvous hashing placement, **hinted handoff**, **read repair**, peer health monitoring, and background healing for partial replicas. Same class of problems MinIO and Ceph solve at a smaller scale.

---

## Resume bullets (pick 2–3)

- Built **S3-compatible object storage** (PUT/GET/HEAD/DELETE, multipart, batch delete, presigned URLs) with **AWS SigV4** and `aws-cli` compatibility.
- Designed **content-addressable blob storage** (SHA-256 paths), atomic writes, orphan cleanup, and scheduled integrity verification.
- Implemented **distributed replication in Go**: rendezvous placement, primary forwarding, **quorum consistency**, durable replication hints, peer health probes, and background healing.
- Shipped **Docker Compose** (single-node + 3-node cluster), **GitHub Actions CI**, Prometheus metrics, and integration tests with fault injection.

---

## Architecture (2-minute whiteboard)

```
S3 clients (aws-cli, SDKs)
        │
        ▼
   ObjeX API (Go)
   SigV4 middleware
        │
   ┌────┴────┐
   ▼         ▼
 SQLite    Filesystem
(metadata) (blob files)
        │
   (cluster mode)
        ▼
 Replication coordinator
 → quorum PUT/DELETE
 → hints + read repair
 → peer health + healing
```

**Key design choice:** metadata and bytes are separate. SQLite for indexes, hashed filesystem paths for blobs. Cluster mode adds a replication layer without changing the S3 client contract.

---

## Live demo script (5 minutes)

### Single node (2 min)

```bash
docker compose up -d --build
aws --endpoint-url http://localhost:9000 s3 mb s3://demo
echo "hello" > /tmp/hello.txt
aws --endpoint-url http://localhost:9000 s3 cp /tmp/hello.txt s3://demo/
aws --endpoint-url http://localhost:9000 s3 ls s3://demo/
curl http://localhost:9000/health/ready
```

### Cluster (3 min)

```bash
docker compose -f docker-compose.cluster.yml up -d --build
curl http://localhost:9001/cluster | jq .
# Upload via node 1, read via node 2. Show forwarding and replication.
```

Talking points: placement primary, write quorum, what happens when a replica is down (hints), `/metrics` counters.

---

## Deep-dive Q&A

| Question | Answer |
|----------|--------|
| Why Go? | Single static binary, great concurrency for replication fan-out, fits systems portfolio. |
| How is the physical path chosen? | `sha256(bucket/key)` → `{bucket}/{aa}/{bb}/{hash}.blob`. No path traversal, even distribution. |
| What consistency model? | Configurable N/R/W quorum; versioned tombstones; read repair on GET. |
| What if a node dies mid-write? | Write quorum must succeed; failed replicas get durable hints with staged payloads. |
| How do you test distribution? | Multi-node `httptest` matrix, fault injection (refused peers), hint delivery + healing tests. |

---

## Links

- Repo: https://github.com/VishalPainjane/ObjeX
- Architecture docs: [docs/architecture/](../architecture/)
- Run tests: `go test ./...`
