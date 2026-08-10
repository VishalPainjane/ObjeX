# Observability

## Metrics endpoint

`GET /metrics` — Prometheus exposition format (no authentication required).

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `objex_http_requests_total` | Counter | `operation`, `method`, `status` | HTTP requests |
| `objex_http_request_duration_seconds` | Histogram | `operation`, `method` | Request latency |
| `objex_http_errors_total` | Counter | `operation`, `status` | HTTP 4xx/5xx |
| `objex_upload_bytes_total` | Counter | — | Bytes uploaded (PUT) |
| `objex_download_bytes_total` | Counter | — | Bytes downloaded (GET) |
| `objex_storage_bytes_used` | Gauge | — | Total stored bytes (from metadata) |
| `objex_objects_total` | Gauge | — | Total object count (from metadata) |
| `objex_multipart_operations_total` | Counter | `operation` | Multipart ops |
| `objex_cluster_nodes` | Gauge | — | Configured cluster size |
| `objex_cluster_node_info` | Gauge | `node_id`, `address` | Static members (bounded) |
| `objex_placement_operations_total` | Counter | `operation` | Placement calculations |
| `objex_cluster_forward_total` | Counter | `operation` | Inter-node forwards |
| `objex_replication_operations_total` | Counter | `operation`, `result` | Replication ops |
| `objex_replication_failures_total` | Counter | `operation` | Replication failures |
| `objex_replication_bytes_total` | Counter | — | Bytes replicated |
| `objex_replication_duration_seconds` | Histogram | `operation` | Replication latency |
| `objex_quorum_writes_total` | Counter | `result` | Quorum write outcomes |
| `objex_quorum_reads_total` | Counter | `result` | Quorum read outcomes |
| `objex_quorum_failures_total` | Counter | `operation` | Quorum not met |
| `objex_read_repair_total` | Counter | — | Read repairs scheduled |
| `objex_stale_replica_detected_total` | Counter | — | Stale replicas on read |
| `objex_hints_created_total` | Counter | `kind` | Hints enqueued |
| `objex_hints_delivered_total` | Counter | `kind` | Hints delivered |
| `objex_hints_failed_total` | Counter | `kind` | Hint delivery failures |
| `objex_hints_pending` | Gauge | — | Pending hints |
| `objex_hint_delivery_duration_seconds` | Histogram | — | Hint delivery latency |
| `objex_cluster_peer_reachable` | Gauge | `node_id` | Runtime peer reachability |
| `objex_cluster_peer_recovery_total` | Counter | `node_id` | Peer recovery events |
| `objex_healing_scans_total` | Counter | — | Healing scan iterations |
| `objex_healing_objects_total` | Counter | — | Objects processed by healing |

## Label policy

Operation labels are **bounded** (`PUT_OBJECT`, `GET_OBJECT`, `LIST_OBJECTS`, etc.). Raw object keys, bucket names, and user IDs are **never** used as metric labels.

Storage gauges sync from SQLite bucket aggregates every 30 seconds — not on every HTTP request.
