# Placement Algorithm — Phase 1

Distributed ObjeX Phase 1 maps each object to exactly one **primary** node using deterministic placement. No replication, migration, or remote I/O occurs in this phase.

## Object key

Placement input is the logical object identity:

```text
{bucket}/{key}
```

Node **IDs** (not addresses) participate in scoring. Addresses are membership metadata for future routing.

## Algorithms compared

| Criterion | Modulo `hash % N` | Consistent hashing (ring) | Rendezvous (HRW) |
|-----------|-------------------|---------------------------|------------------|
| Determinism | Yes (if nodes sorted) | Yes (with fixed ring) | Yes (sort nodes by ID) |
| Distribution | Good | Good (with virtual nodes) | Good |
| Node add | ~all keys remap | ~1/N keys move | ~1/(N+1) keys move |
| Node remove | ~all keys remap | Keys on removed node remap | Only removed node's keys remap |
| Implementation | Trivial | Ring + vnodes + complexity | Simple loop over nodes |
| Future replicas | Pick hash offsets (fragile) | Walk ring | Pick top-K scores naturally |

### Modulo hashing

`hash(object) % N` is easy but **rebalances almost everything** when `N` changes. That makes it a poor foundation for later migration and replication.

### Consistent hashing

Strong for large rings and gradual movement, but needs virtual nodes, ring management, and careful serialization for determinism. Higher implementation cost for Phase 1.

### Rendezvous hashing (selected)

For each object, score every node with `hash(object + nodeID)` and pick the highest score. With nodes sorted by ID and deterministic tie-breaking, all processes agree.

**Why chosen:** simple, fast, deterministic, good distribution, natural extension to top-K replicas later, and relatively low remap ratio on membership change.

## Membership ordering

Nodes are always sorted by `ID` before placement. Input order in configuration does not affect results.

## Membership changes (placement only)

When a node is added or removed, some objects would map to a different primary. Phase 1 **only measures** this; it does not move data.

Observed on 10,000 synthetic keys when adding `node-4` to a 3-node cluster: typically ~20–30% of keys change primary (expected ~25% for rendezvous).

## API

```go
type PlacementResult struct {
    Primary  Node
    Replicas []Node
}

func (p PlacementResult) ReplicaSet() []Node
```

`Locate(bucket, key)` returns the primary and `RF-1` replicas using rendezvous top-K selection. `RF` is configured via `OBJEX_REPLICATION_FACTOR`.

## Inspection

- `GET /debug/placement?bucket=...&key=...` — development endpoint (not S3)
- Debug-level logs on object operations when placement is wired

## Limitations (Phase 1)

- No replication or remote reads/writes
- No automatic migration when placement changes
- Membership is static configuration, not runtime health
- `/health` describes the local node only
