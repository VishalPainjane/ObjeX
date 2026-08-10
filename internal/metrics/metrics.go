package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_http_requests_total",
		Help: "Total HTTP requests by operation, method, and status.",
	}, []string{"operation", "method", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "objex_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation", "method"})

	httpErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_http_errors_total",
		Help: "Total HTTP error responses by operation and status.",
	}, []string{"operation", "status"})

	uploadBytesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "objex_upload_bytes_total",
		Help: "Total bytes uploaded via PUT operations.",
	})

	downloadBytesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "objex_download_bytes_total",
		Help: "Total bytes downloaded via GET operations.",
	})

	storageBytesUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "objex_storage_bytes_used",
		Help: "Total stored object bytes from metadata.",
	})

	objectsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "objex_objects_total",
		Help: "Total object count from metadata.",
	})

	multipartOpsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_multipart_operations_total",
		Help: "Multipart operations by type.",
	}, []string{"operation"})

	clusterNodesGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "objex_cluster_nodes",
		Help: "Number of nodes in static cluster membership.",
	})

	clusterNodeInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "objex_cluster_node_info",
		Help: "Static cluster membership (1 = configured member).",
	}, []string{"node_id", "address"})

	placementOpsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_placement_operations_total",
		Help: "Placement operations by type.",
	}, []string{"operation"})

	clusterForwardTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_cluster_forward_total",
		Help: "Inter-node request forwards by operation.",
	}, []string{"operation"})

	replicationOpsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_replication_operations_total",
		Help: "Replication operations by type and result.",
	}, []string{"operation", "result"})

	replicationFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_replication_failures_total",
		Help: "Replication failures by operation.",
	}, []string{"operation"})

	replicationBytesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "objex_replication_bytes_total",
		Help: "Total bytes replicated to peer nodes.",
	})

	replicationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "objex_replication_duration_seconds",
		Help:    "Replication operation duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	quorumWritesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_quorum_writes_total",
		Help: "Quorum write operations by result.",
	}, []string{"result"})

	quorumReadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_quorum_reads_total",
		Help: "Quorum read operations by result.",
	}, []string{"result"})

	quorumFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_quorum_failures_total",
		Help: "Quorum failures by operation.",
	}, []string{"operation"})

	readRepairTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "objex_read_repair_total",
		Help: "Read repairs scheduled.",
	})

	staleReplicaTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "objex_stale_replica_detected_total",
		Help: "Stale replicas detected during reads.",
	})

	hintsCreatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_hints_created_total",
		Help: "Replication hints created by kind.",
	}, []string{"kind"})

	hintsDeliveredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_hints_delivered_total",
		Help: "Replication hints delivered by kind.",
	}, []string{"kind"})

	hintsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_hints_failed_total",
		Help: "Replication hint delivery failures by kind.",
	}, []string{"kind"})

	hintsPendingGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "objex_hints_pending",
		Help: "Pending replication hints.",
	})

	hintDeliveryDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "objex_hint_delivery_duration_seconds",
		Help:    "Hint delivery duration in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	peerReachableGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "objex_cluster_peer_reachable",
		Help: "Runtime peer reachability from health probes (1=reachable).",
	}, []string{"node_id"})

	peerRecoveryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "objex_cluster_peer_recovery_total",
		Help: "Peer recovery events detected by health monitor.",
	}, []string{"node_id"})

	healingScansTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "objex_healing_scans_total",
		Help: "Background healing scan iterations.",
	})

	healingObjectsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "objex_healing_objects_total",
		Help: "Objects processed by healing scans.",
	})
)

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// SetStorageStats updates gauge metrics from metadata aggregates.
func SetStorageStats(objectCount, totalBytes int64) {
	objectsTotal.Set(float64(objectCount))
	storageBytesUsed.Set(float64(totalBytes))
}

// ClusterMember describes a node for cluster metrics (bounded label cardinality).
type ClusterMember struct {
	ID      string
	Address string
}

// RecordMultipart records a multipart operation.
func RecordMultipart(op string) {
	multipartOpsTotal.WithLabelValues(op).Inc()
}

// RecordPlacement records a placement calculation.
func RecordPlacement(operation string) {
	placementOpsTotal.WithLabelValues(operation).Inc()
}

// RecordClusterForward records an inter-node HTTP forward.
func RecordClusterForward(operation string) {
	clusterForwardTotal.WithLabelValues(operation).Inc()
}

// RecordReplication records a replication operation result.
func RecordReplication(operation, result string) {
	replicationOpsTotal.WithLabelValues(operation, result).Inc()
	if result == "failure" {
		replicationFailuresTotal.WithLabelValues(operation).Inc()
	}
}

// RecordReplicationBytes adds replicated byte volume.
func RecordReplicationBytes(n int64) {
	if n > 0 {
		replicationBytesTotal.Add(float64(n))
	}
}

// ObserveReplicationDuration records replication latency.
func ObserveReplicationDuration(operation string, d time.Duration) {
	replicationDuration.WithLabelValues(operation).Observe(d.Seconds())
}

// RecordQuorumWrite records a quorum write attempt.
func RecordQuorumWrite(required, acks, replicaCount int, operation string) {
	result := "success"
	if acks < required {
		result = "failure"
	}
	quorumWritesTotal.WithLabelValues(result).Inc()
	_ = operation
}

// RecordQuorumRead records a quorum read attempt.
func RecordQuorumRead(required, responses, replicaCount int, operation string) {
	result := "success"
	if responses < required {
		result = "failure"
	}
	quorumReadsTotal.WithLabelValues(result).Inc()
	_ = operation
}

// RecordQuorumFailure increments quorum failure counter.
func RecordQuorumFailure(operation string) {
	quorumFailuresTotal.WithLabelValues(operation).Inc()
}

// RecordReadRepair increments read repair counter.
func RecordReadRepair() {
	readRepairTotal.Inc()
}

// RecordStaleReplica increments stale replica detection counter.
func RecordStaleReplica() {
	staleReplicaTotal.Inc()
}

// RecordHintCreated records hint creation.
func RecordHintCreated(kind string) {
	hintsCreatedTotal.WithLabelValues(kind).Inc()
}

// RecordHintDelivered records successful hint delivery.
func RecordHintDelivered(kind string) {
	hintsDeliveredTotal.WithLabelValues(kind).Inc()
}

// RecordHintFailed records failed hint delivery.
func RecordHintFailed(kind string) {
	hintsFailedTotal.WithLabelValues(kind).Inc()
}

// SetHintsPending updates pending hint gauge.
func SetHintsPending(n int64) {
	hintsPendingGauge.Set(float64(n))
}

// ObserveHintDelivery records hint delivery latency.
func ObserveHintDelivery(d time.Duration) {
	hintDeliveryDuration.Observe(d.Seconds())
}

// SetPeerReachable updates runtime peer reachability gauge.
func SetPeerReachable(nodeID string, reachable bool) {
	v := 0.0
	if reachable {
		v = 1
	}
	peerReachableGauge.WithLabelValues(nodeID).Set(v)
}

// RecordPeerRecovery increments peer recovery counter.
func RecordPeerRecovery(nodeID string) {
	peerRecoveryTotal.WithLabelValues(nodeID).Inc()
}

// RecordHealingScan increments healing scan counter.
func RecordHealingScan() {
	healingScansTotal.Inc()
}

// RecordHealingObjects adds objects processed in a healing scan.
func RecordHealingObjects(n int) {
	if n > 0 {
		healingObjectsTotal.Add(float64(n))
	}
}

// SetClusterMembership updates cluster gauge metrics from static membership.
func SetClusterMembership(nodes []ClusterMember) {
	clusterNodesGauge.Set(float64(len(nodes)))
	for _, n := range nodes {
		clusterNodeInfo.WithLabelValues(n.ID, n.Address).Set(1)
	}
}

// Middleware records HTTP metrics with bounded operation labels.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		op := ClassifyOperation(r)
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK, bytes: 0}
		next.ServeHTTP(wrapped, r)

		status := wrapped.status
		statusLabel := http.StatusText(status)
		if statusLabel == "" {
			statusLabel = "unknown"
		}
		method := r.Method
		httpRequestsTotal.WithLabelValues(op, method, statusLabel).Inc()
		httpRequestDuration.WithLabelValues(op, method).Observe(time.Since(start).Seconds())
		if isMultipartOperation(op) {
			RecordMultipart(op)
		}
		if status >= 400 {
			httpErrorsTotal.WithLabelValues(op, statusLabel).Inc()
		}
		if op == "PUT_OBJECT" && wrapped.bytes > 0 {
			uploadBytesTotal.Add(float64(wrapped.bytes))
		}
		if op == "GET_OBJECT" && wrapped.bytes > 0 {
			downloadBytesTotal.Add(float64(wrapped.bytes))
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

// ClassifyOperation maps a request to a bounded operation label.
func ClassifyOperation(r *http.Request) string {
	path := r.URL.Path
	if path == "/metrics" {
		return "METRICS"
	}
	if path == "/cluster" {
		return "CLUSTER"
	}
	if path == "/debug/placement" {
		return "PLACEMENT_DEBUG"
	}
	if strings.HasPrefix(path, "/health") {
		return "HEALTH"
	}
	if strings.HasPrefix(path, "/api/presign/") {
		return "PRESIGN"
	}

	q := r.URL.Query()
	method := r.Method

	if path == "/" || path == "" {
		if method == http.MethodGet {
			return "LIST_BUCKETS"
		}
		return "OTHER"
	}

	trimmed := strings.TrimPrefix(path, "/")
	if !strings.Contains(trimmed, "/") {
		switch method {
		case http.MethodPut:
			return "CREATE_BUCKET"
		case http.MethodDelete:
			return "DELETE_BUCKET"
		case http.MethodGet:
			if q.Has("uploads") {
				return "LIST_MULTIPART_UPLOADS"
			}
			return "LIST_OBJECTS"
		case http.MethodHead:
			return "HEAD_BUCKET"
		}
		return "OTHER"
	}

	switch method {
	case http.MethodPut:
		if q.Has("partNumber") && q.Has("uploadId") {
			return "MULTIPART_UPLOAD_PART"
		}
		if r.Header.Get("x-amz-copy-source") != "" {
			return "COPY_OBJECT"
		}
		return "PUT_OBJECT"
	case http.MethodGet:
		if q.Has("uploadId") {
			return "LIST_PARTS"
		}
		return "GET_OBJECT"
	case http.MethodHead:
		return "HEAD_OBJECT"
	case http.MethodDelete:
		if q.Has("uploadId") {
			return "ABORT_MULTIPART"
		}
		return "DELETE_OBJECT"
	case http.MethodPost:
		if q.Has("uploads") {
			return "INITIATE_MULTIPART"
		}
		if q.Has("uploadId") {
			return "COMPLETE_MULTIPART"
		}
	}
	return "OTHER"
}

func isMultipartOperation(op string) bool {
	switch op {
	case "INITIATE_MULTIPART", "COMPLETE_MULTIPART", "ABORT_MULTIPART", "MULTIPART_UPLOAD_PART", "LIST_PARTS", "LIST_MULTIPART_UPLOADS":
		return true
	default:
		return false
	}
}
