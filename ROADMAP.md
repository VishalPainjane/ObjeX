# Roadmap

ObjeX is maintained by [Vishal Painjane](https://github.com/VishalPainjane).

Detailed technical roadmap: [docs/architecture/roadmap.md](docs/architecture/roadmap.md)

## Shipped

- S3 object API (PUT/GET/HEAD/DELETE, list, Range, batch delete, copy)
- Multipart uploads
- AWS SigV4 + presigned GET/PUT
- SQLite metadata + filesystem blobs
- Cluster: placement, quorum, hints, read repair, peer health, healing
- Docker Compose (single + 3-node)
- CI (test, vet, race), Prometheus metrics
- Integrity verification job

## Next

- Presigned POST (browser form uploads)
- `aws-chunked` streaming decoder
- PostgreSQL metadata backend
- Helm chart for Kubernetes

## Out of scope (for now)

- Web UI / user management
- Full S3 ACL model
- Erasure coding / automatic rebalancing
