# ObjeX Architecture

ObjeX is a single-process S3-compatible object store. Multi-node clusters share static membership, deterministic placement, primary forwarding, and object replication.

```
Client → SigV4 / Internal Token → Metrics → HTTP API → Replication Coordinator → Object Service → Metadata + Storage
                                              ↓ (non-primary)                         ↓ (replicate-put/delete)
                                         Forward to primary                         Peer nodes
```

## Components

| Layer | Package | Responsibility |
|-------|---------|----------------|
| Cluster | `internal/cluster` | Membership, placement, proxy, peer sync |
| Replication | `internal/replication` | Coordinator, HTTP replicator |
| Auth | `internal/auth` | SigV4, presign, inter-node internal token |
| Metrics | `internal/metrics` | Prometheus HTTP and storage metrics |
| API | `internal/api` | S3 HTTP routing, XML responses, cluster endpoints |
| Service | `internal/object` | Business rules, ETags, multipart, copy |
| Metadata | `internal/metadata/sqlite` | Buckets, objects, credentials, multipart |
| Storage | `internal/storage/filesystem` | Content-addressed blobs |
| Jobs | `internal/jobs` | Orphan blob and abandoned multipart cleanup |

## Authentication

S3 routes require SigV4 except `/health*`, `/metrics`, `/cluster`, `/debug/placement`. Inter-node requests use `X-ObjeX-Internal-Token` when `OBJEX_CLUSTER_INTERNAL_TOKEN` is set. See `docs/authentication.md` and `docs/distributed-architecture.md`.

## Request flow (PUT object)

1. SigV4 middleware validates signature and timestamp.
2. Handler streams body to object service.
3. Service writes blob (tmp → rename).
4. Service commits metadata in SQLite transaction.
5. On metadata failure, blob is deleted.

## Future distributed work

Interfaces `metadata.Store` and `storage.Storage` allow a future placement layer to wrap or replace local SQLite and filesystem without changing the HTTP API.
