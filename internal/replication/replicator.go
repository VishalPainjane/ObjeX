package replication

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VishalPainjane/objex/internal/auth"
	"github.com/VishalPainjane/objex/internal/cluster"
	"github.com/VishalPainjane/objex/internal/metrics"
)

// Replicator sends internal replica operations to peer nodes over HTTP.
type Replicator struct {
	internalToken string
	client        *http.Client
	logger        *slog.Logger
}

// NewReplicator creates an HTTP replicator for inter-node replica I/O.
func NewReplicator(internalToken string, client *http.Client, logger *slog.Logger) *Replicator {
	if client == nil {
		client = http.DefaultClient
	}
	return &Replicator{
		internalToken: internalToken,
		client:        client,
		logger:        logger,
	}
}

// PutReplicaInput describes a streaming replica write.
type PutReplicaInput struct {
	Bucket         string
	Key            string
	Version        int64
	ExpectedETag   string
	ContentType    string
	CustomMetadata map[string]string
	Size           int64
	OpenBody       func() (io.ReadCloser, error)
}

// WriteAckResult aggregates replica write acknowledgements.
type WriteAckResult struct {
	Acks     int
	Success  []string
	Failures []ReplicaError
}

// ReplicatePut streams an object to replica peers in parallel.
func (r *Replicator) ReplicatePut(ctx context.Context, replicas []cluster.Node, in PutReplicaInput) WriteAckResult {
	if len(replicas) == 0 {
		return WriteAckResult{}
	}
	start := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	result := WriteAckResult{Success: make([]string, 0, len(replicas))}

	for _, replica := range replicas {
		wg.Add(1)
		go func(node cluster.Node) {
			defer wg.Done()
			if err := r.putToNode(ctx, node, in); err != nil {
				mu.Lock()
				result.Failures = append(result.Failures, ReplicaError{NodeID: node.ID, Err: err})
				mu.Unlock()
				metrics.RecordReplication("put", "failure")
				if r.logger != nil {
					r.logger.Error("replica put failed",
						"bucket", in.Bucket,
						"key", in.Key,
						"version", in.Version,
						"replica", node.ID,
						"error", err,
					)
				}
				return
			}
			mu.Lock()
			result.Success = append(result.Success, node.ID)
			result.Acks++
			mu.Unlock()
		}(replica)
	}
	wg.Wait()

	if len(result.Failures) == 0 {
		metrics.RecordReplication("put", "success")
		metrics.RecordReplicationBytes(in.Size * int64(len(replicas)))
	} else if result.Acks > 0 {
		metrics.RecordReplicationBytes(in.Size * int64(result.Acks))
	}
	metrics.ObserveReplicationDuration("put", time.Since(start))
	return result
}

func (r *Replicator) putToNode(ctx context.Context, node cluster.Node, in PutReplicaInput) error {
	if err := injectFault(node.ID, "put"); err != nil {
		return err
	}
	if r.internalToken == "" {
		return fmt.Errorf("cluster internal token not configured")
	}
	body, err := in.OpenBody()
	if err != nil {
		return err
	}
	defer body.Close()

	path := "/" + url.PathEscape(in.Bucket) + "/" + objectPath(in.Key)
	target := nodeBaseURL(node.Address) + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, body)
	if err != nil {
		return err
	}
	req.Header.Set(auth.InternalTokenHeader, r.internalToken)
	req.Header.Set(auth.InternalOperationHeader, auth.OpReplicatePut)
	req.Header.Set(auth.InternalObjectVersion, strconv.FormatInt(in.Version, 10))
	req.Header.Set(auth.InternalExpectedETag, in.ExpectedETag)
	if in.ContentType != "" {
		req.Header.Set("Content-Type", in.ContentType)
	}
	for k, v := range in.CustomMetadata {
		req.Header.Set("x-amz-meta-"+k, v)
	}
	if in.Size > 0 {
		req.ContentLength = in.Size
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// ReplicateDelete applies tombstones on replica peers in parallel.
func (r *Replicator) ReplicateDelete(ctx context.Context, replicas []cluster.Node, bucket, key string, version int64) WriteAckResult {
	if len(replicas) == 0 {
		return WriteAckResult{}
	}
	start := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	result := WriteAckResult{Success: make([]string, 0, len(replicas))}

	for _, replica := range replicas {
		wg.Add(1)
		go func(node cluster.Node) {
			defer wg.Done()
			if err := r.deleteOnNode(ctx, node, bucket, key, version); err != nil {
				mu.Lock()
				result.Failures = append(result.Failures, ReplicaError{NodeID: node.ID, Err: err})
				mu.Unlock()
				metrics.RecordReplication("delete", "failure")
				if r.logger != nil {
					r.logger.Error("replica delete failed",
						"bucket", bucket,
						"key", key,
						"version", version,
						"replica", node.ID,
						"error", err,
					)
				}
				return
			}
			mu.Lock()
			result.Success = append(result.Success, node.ID)
			result.Acks++
			mu.Unlock()
		}(replica)
	}
	wg.Wait()

	if len(result.Failures) == 0 {
		metrics.RecordReplication("delete", "success")
	}
	metrics.ObserveReplicationDuration("delete", time.Since(start))
	return result
}

func (r *Replicator) deleteOnNode(ctx context.Context, node cluster.Node, bucket, key string, version int64) error {
	if err := injectFault(node.ID, "delete"); err != nil {
		return err
	}
	if r.internalToken == "" {
		return fmt.Errorf("cluster internal token not configured")
	}
	path := "/" + url.PathEscape(bucket) + "/" + objectPath(key)
	target := nodeBaseURL(node.Address) + path

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set(auth.InternalTokenHeader, r.internalToken)
	req.Header.Set(auth.InternalOperationHeader, auth.OpReplicateDelete)
	req.Header.Set(auth.InternalObjectVersion, strconv.FormatInt(version, 10))

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// FetchReplicaStates queries replicate-head on all nodes concurrently.
func (r *Replicator) FetchReplicaStates(ctx context.Context, nodes []cluster.Node, bucket, key string) []ReplicaState {
	states := make([]ReplicaState, len(nodes))
	var wg sync.WaitGroup
	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, n cluster.Node) {
			defer wg.Done()
			states[idx] = r.headOnNode(ctx, n, bucket, key)
		}(i, node)
	}
	wg.Wait()
	return states
}

func (r *Replicator) headOnNode(ctx context.Context, node cluster.Node, bucket, key string) ReplicaState {
	state := ReplicaState{NodeID: node.ID}
	if err := injectFault(node.ID, "head"); err != nil {
		state.Found = false
		return state
	}
	if r.internalToken == "" {
		return state
	}
	path := "/" + url.PathEscape(bucket) + "/" + objectPath(key)
	target := nodeBaseURL(node.Address) + path
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return state
	}
	req.Header.Set(auth.InternalTokenHeader, r.internalToken)
	req.Header.Set(auth.InternalOperationHeader, auth.OpReplicateHead)

	resp, err := r.client.Do(req)
	if err != nil {
		return state
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return state
	}
	if resp.StatusCode != http.StatusOK {
		return state
	}
	state.Found = true
	if v := resp.Header.Get(auth.InternalObjectVersion); v != "" {
		state.Version, _ = strconv.ParseInt(v, 10, 64)
	}
	state.ETag = strings.Trim(resp.Header.Get("ETag"), `"`)
	state.Size, _ = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	state.ContentType = resp.Header.Get("Content-Type")
	state.Deleted = resp.Header.Get(auth.InternalObjectDeleted) == "true"
	state.CustomMetadata = extractMetaHeaders(resp.Header)
	return state
}

// StreamFromNode opens a replicate-get stream from a peer.
func (r *Replicator) StreamFromNode(ctx context.Context, node cluster.Node, bucket, key string) (io.ReadCloser, int64, string, error) {
	if err := injectFault(node.ID, "get"); err != nil {
		return nil, 0, "", err
	}
	if r.internalToken == "" {
		return nil, 0, "", fmt.Errorf("cluster internal token not configured")
	}
	path := "/" + url.PathEscape(bucket) + "/" + objectPath(key)
	target := nodeBaseURL(node.Address) + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set(auth.InternalTokenHeader, r.internalToken)
	req.Header.Set(auth.InternalOperationHeader, auth.OpReplicateGet)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, 0, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	etag := strings.Trim(resp.Header.Get("ETag"), `"`)
	return resp.Body, size, etag, nil
}

func extractMetaHeaders(h http.Header) map[string]string {
	out := make(map[string]string)
	for k, vals := range h {
		if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") && len(vals) > 0 {
			out[strings.TrimPrefix(strings.ToLower(k), "x-amz-meta-")] = vals[0]
		}
	}
	return out
}

func nodeBaseURL(address string) string {
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return strings.TrimRight(address, "/")
	}
	return "http://" + address
}

func objectPath(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
