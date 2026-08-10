# Quorum Protocol — Phase 3

Phase 3 adds **configurable read/write quorum** on top of Phase 2B replication.

## Parameters

| Symbol | Meaning | Config |
|--------|---------|--------|
| **N** | Replicas in the placement set (includes primary) | `OBJEX_REPLICATION_FACTOR` |
| **W** | Writes succeed after W durable acknowledgements | `OBJEX_WRITE_QUORUM` |
| **R** | Reads succeed after R replica responses | `OBJEX_READ_QUORUM` |

**N is replication factor. W and R are quorum thresholds, not replica counts.**

Default for a 3-node cluster: `N=3`, `W=2`, `R=2`.

## Overlap rule

Default mode requires:

```text
W + R > N
```

For `N=3, W=2, R=2`: every successful write quorum and successful read quorum overlap in at least one replica.

This overlap is **necessary but not sufficient** for strong consistency. ObjeX also requires:

- monotonic per-object versions assigned at the primary
- version comparison on reads (newest wins)
- tombstones for deletes (prevents resurrection)
- stale replica rejection on writes

We do **not** claim linearizability or serializability. The model is **single-object, quorum-based, version-aware consistency**.

## Write algorithm (primary-coordinated)

1. Determine placement (N nodes)
2. Assign `version = max(stored)+1` at primary (serialization point per object)
3. Write locally (counts as 1 ACK)
4. Stream `replicate-put` to other nodes in parallel
5. Collect ACKs until `acks >= W` or all nodes responded
6. On `acks >= W`: success; enqueue durable hints for failed nodes
7. On `acks < W`: failure (partial writes may exist; hints not created for success path)

Primary local object bytes are the replay source for replica streaming and hint delivery.

## Read algorithm (correctness-first)

For initial implementation with `N=3`:

1. Issue `replicate-head` to **all N nodes** concurrently (bounded timeout)
2. Wait until at least **R** successful metadata responses **or** all nodes responded
3. If `< R` successes → availability error
4. Select the **highest version** among responses (tombstone versions participate)
5. If winner is tombstone → Not Found
6. Stream object body from the winning node (local or `replicate-get` peer)
7. Enqueue durable repair hints for replicas with `version < winner`

We query all N nodes (not exactly R) so a stale minority cannot hide a newer version elsewhere.

## Delete / tombstones

DELETE writes a **tombstone** at `version = previous+1` with `deleted=true`. Tombstones replicate via `replicate-delete` (tombstone put). W quorum required.

A stale replica with an older object version loses to a newer tombstone on read.

## Hinted handoff vs read repair

| Mechanism | Trigger | Action |
|-----------|---------|--------|
| Hinted handoff | Write quorum reached but replica failed | Deliver missing `replicate-put` / tombstone |
| Read repair | Read found stale replica | Deliver winning version to stale node |

Both use the same durable `replication_hints` table and background worker.

## Hint payload

Put/repair hints from the local primary **stage a pinned copy** of the object bytes under `{base}/_hints/{target}/{bucket}/{version}/…` at enqueue time. The `source_path` column stores this path so delivery survives later overwrites on the live object blob.

- Orphan cleanup skips `_hints/` and treats pending `source_path` values as protected.
- After successful delivery the worker deletes the staged file.
- Tombstone hints carry no payload (version + delete only).

## Failure injection (tests only)

`replication.SetFaultInjector` allows tests to simulate per-node timeouts/errors without killing processes.

## Deferred

- Multipart / copy quorum
- LIST quorum consistency
- Failure detection / automatic healing
- Erasure coding
