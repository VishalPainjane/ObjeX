# Roadmap

## Done (baseline)

- S3 bucket and object API
- Streaming uploads, Range GET, custom metadata
- Multipart upload lifecycle
- Server-side copy
- AWS SigV4 authentication
- Presigned GET/PUT URLs
- Prometheus metrics
- Health endpoints
- Background cleanup jobs
- Docker image
- Integration, concurrency, and auth tests

## Done (Distributed Phase 1)

- Node identity (`OBJEX_NODE_ID`, `--node-id`)
- Static cluster membership configuration
- Rendezvous placement engine
- Cluster and placement debug endpoints
- Three-node Docker Compose

## Done (Distributed Phase 2)

- Primary-node HTTP forwarding (PUT/GET/HEAD/DELETE)
- Inter-node internal authentication (`OBJEX_CLUSTER_INTERNAL_TOKEN`)
- Bucket metadata fan-out on create
- Object replication (PUT/DELETE), version/generation, stale replica protection

## Done (Distributed Phase 3)

- Configurable N/R/W quorum (`OBJEX_REPLICATION_FACTOR`, `OBJEX_WRITE_QUORUM`, `OBJEX_READ_QUORUM`)
- Quorum writes (W acknowledgements) and quorum reads (R responses, newest version)
- Versioned tombstones for DELETE
- Durable hinted handoff (SQLite `replication_hints` + pinned payload staging)
- Read repair via shared hint infrastructure
- Quorum/replication/hint metrics
- 3-node quorum integration tests + fault injection for tests

## Done (Distributed Phase 4 — foundation)

- Peer health probes (`/health/live`) with runtime reachability tracking
- Recovery detection + callback (hint backoff reset, immediate hint/healing pass)
- Background healing scheduler for `replication_status=partial` objects
- `GET /cluster` reachable field + peer health metrics

## Next

- Multipart and copy quorum semantics across nodes
- SQL-level list pagination for very large buckets
- Scheduled integrity verification job
- Optional: skip unreachable peers in replication fan-out

## Explicitly deferred

**PostgreSQL** — until distributed metadata architecture is finalized.

**Advanced distributed:** erasure coding, automatic rebalancing, consensus/Raft.
