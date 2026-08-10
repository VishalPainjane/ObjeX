package replication

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/VishalPainjane/objex/internal/cluster"
	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/metrics"
	"github.com/VishalPainjane/objex/internal/object"
	"github.com/VishalPainjane/objex/internal/quorum"
)

var (
	// ErrQuorumNotMet is returned when W or R is not satisfied.
	ErrQuorumNotMet = errors.New("quorum not met")
)

// Coordinator orchestrates quorum-aware replicated object operations on the primary node.
type Coordinator struct {
	localNodeID string
	membership  cluster.Membership
	placer      cluster.Placer
	svc         *object.Service
	meta        metadata.Store
	replicator  *Replicator
	quorum      quorum.Config
	logger      *slog.Logger
}

// NewCoordinator wires replication for the local primary node.
func NewCoordinator(localNodeID string, membership cluster.Membership, placer cluster.Placer, svc *object.Service, meta metadata.Store, replicator *Replicator, q quorum.Config, logger *slog.Logger) *Coordinator {
	return &Coordinator{
		localNodeID: localNodeID,
		membership:  membership,
		placer:      placer,
		svc:         svc,
		meta:        meta,
		replicator:  replicator,
		quorum:      q,
		logger:      logger,
	}
}

// Quorum returns the configured N/R/W thresholds.
func (c *Coordinator) Quorum() quorum.Config {
	return c.quorum
}

// Enabled reports whether replication is active (RF > 1 with a replicator).
func (c *Coordinator) Enabled() bool {
	return c != nil && c.placer != nil && c.replicator != nil && c.placer.ReplicationFactor() > 1
}

// PutObject stores on the primary and replicates to peers when RF > 1.
func (c *Coordinator) PutObject(ctx context.Context, in object.PutObjectInput) (string, error) {
	if c == nil || c.svc == nil {
		return "", fmt.Errorf("coordinator not configured")
	}
	if !c.Enabled() {
		return c.svc.PutObject(ctx, in)
	}

	placement, err := c.placer.Locate(in.BucketName, in.Key)
	if err != nil {
		return "", err
	}
	if placement.Primary.ID != c.localNodeID {
		return "", ErrNotPrimary
	}

	version, err := c.svc.NextObjectVersion(ctx, in.BucketName, in.Key)
	if err != nil {
		return "", err
	}

	etag, err := c.svc.PutObjectWithVersion(ctx, in, version, "partial")
	if err != nil {
		return "", err
	}

	acks := 1
	var remoteResult WriteAckResult
	if len(placement.Replicas) > 0 {
		rc, size, err := c.svc.OpenStoredObject(ctx, in.BucketName, in.Key)
		if err != nil {
			return "", err
		}
		rc.Close()

		bucket := in.BucketName
		key := in.Key
		remoteResult = c.replicator.ReplicatePut(ctx, placement.Replicas, PutReplicaInput{
			Bucket:         in.BucketName,
			Key:            in.Key,
			Version:        version,
			ExpectedETag:   etag,
			ContentType:    in.ContentType,
			CustomMetadata: in.CustomMetadata,
			Size:           size,
			OpenBody: func() (io.ReadCloser, error) {
				rc, _, err := c.svc.OpenStoredObject(ctx, bucket, key)
				return rc, err
			},
		})
		acks += remoteResult.Acks
	}

	metrics.RecordQuorumWrite(c.quorum.W, acks, len(placement.ReplicaSet()), "put")
	if !c.quorum.WriteSatisfied(acks) {
		if c.logger != nil {
			c.logger.Error("write quorum not met",
				"bucket", in.BucketName,
				"key", in.Key,
				"version", version,
				"required", c.quorum.W,
				"acks", acks,
			)
		}
		metrics.RecordQuorumFailure("put")
		return "", object.NewReplicationError(ErrQuorumNotMet)
	}

	for _, f := range remoteResult.Failures {
		if err := c.enqueueHint(ctx, metadata.HintKindPut, f.NodeID, in.BucketName, in.Key, version, etag, in.ContentType, in.CustomMetadata, c.localNodeID); err != nil && c.logger != nil {
			c.logger.Warn("hint enqueue failed", "target", f.NodeID, "error", err)
		}
	}

	if err := c.svc.SetReplicationStatus(ctx, in.BucketName, in.Key, "replicated"); err != nil && c.logger != nil {
		c.logger.Warn("replication status update failed", "error", err)
	}
	return etag, nil
}

// DeleteObject removes the object using versioned tombstones and write quorum.
func (c *Coordinator) DeleteObject(ctx context.Context, bucket, key string) error {
	if c == nil || c.svc == nil {
		return fmt.Errorf("coordinator not configured")
	}
	if !c.Enabled() {
		return c.svc.DeleteObject(ctx, bucket, key)
	}

	placement, err := c.placer.Locate(bucket, key)
	if err != nil {
		return err
	}
	if placement.Primary.ID != c.localNodeID {
		return ErrNotPrimary
	}

	existing, err := c.svc.GetObjectMetadata(ctx, bucket, key)
	if err != nil {
		var oe *object.Error
		if errors.As(err, &oe) && oe.Code == object.CodeNoSuchKey {
			return nil
		}
		return err
	}
	if existing == nil {
		return nil
	}

	version, err := c.svc.NextObjectVersion(ctx, bucket, key)
	if err != nil {
		return err
	}

	if err := c.svc.PutTombstone(ctx, bucket, key, version); err != nil {
		return err
	}

	acks := 1
	var remoteResult WriteAckResult
	if len(placement.Replicas) > 0 {
		remoteResult = c.replicator.ReplicateDelete(ctx, placement.Replicas, bucket, key, version)
		acks += remoteResult.Acks
	}

	metrics.RecordQuorumWrite(c.quorum.W, acks, len(placement.ReplicaSet()), "delete")
	if !c.quorum.WriteSatisfied(acks) {
		metrics.RecordQuorumFailure("delete")
		return object.NewReplicationError(ErrQuorumNotMet)
	}

	for _, f := range remoteResult.Failures {
		if err := c.enqueueHint(ctx, metadata.HintKindTombstone, f.NodeID, bucket, key, version, "", "", nil, c.localNodeID); err != nil && c.logger != nil {
			c.logger.Warn("tombstone hint enqueue failed", "target", f.NodeID, "error", err)
		}
	}
	return nil
}

// GetObject performs a quorum read and returns the newest version.
func (c *Coordinator) GetObject(ctx context.Context, bucket, key string, verifyIntegrity bool) (*object.GetObjectResult, error) {
	if c == nil || c.svc == nil {
		return nil, fmt.Errorf("coordinator not configured")
	}
	if !c.Enabled() || !c.quorum.Enabled() {
		return c.svc.GetObject(ctx, bucket, key, verifyIntegrity)
	}

	placement, err := c.placer.Locate(bucket, key)
	if err != nil {
		return nil, err
	}

	winning, responses, err := c.quorumRead(ctx, placement.ReplicaSet(), bucket, key)
	if err != nil {
		return nil, err
	}
	if !winning.Found || winning.Deleted {
		return nil, object.NotFoundKey()
	}

	c.scheduleReadRepair(ctx, bucket, key, winning, responses)

	var body io.ReadCloser
	var size int64
	if winning.NodeID == c.localNodeID {
		rc, sz, err := c.svc.OpenStoredObject(ctx, bucket, key)
		if err != nil {
			return nil, err
		}
		body, size = rc, sz
	} else {
		node, ok := c.nodeByID(winning.NodeID)
		if !ok {
			return nil, object.NewQuorumReadError(fmt.Errorf("winning node unknown"))
		}
		rc, sz, _, err := c.replicator.StreamFromNode(ctx, node, bucket, key)
		if err != nil {
			return nil, object.NewQuorumReadError(err)
		}
		body, size = rc, sz
	}

	obj, err := c.svc.GetObjectMetadata(ctx, bucket, key)
	lastMod := time.Now().UTC()
	contentType := winning.ContentType
	custom := winning.CustomMetadata
	if err == nil && obj != nil && !obj.Deleted {
		lastMod = obj.UpdatedAt
		if contentType == "" {
			contentType = obj.ContentType
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return &object.GetObjectResult{
		Body:           body,
		Size:           size,
		ContentType:    contentType,
		ETag:           winning.ETag,
		LastModified:   lastMod,
		CustomMetadata: custom,
	}, nil
}

// HeadObject performs quorum metadata read.
func (c *Coordinator) HeadObject(ctx context.Context, bucket, key string) (*metadata.Object, error) {
	if c == nil || c.svc == nil {
		return nil, fmt.Errorf("coordinator not configured")
	}
	if !c.Enabled() || !c.quorum.Enabled() {
		return c.svc.HeadObject(ctx, bucket, key)
	}

	placement, err := c.placer.Locate(bucket, key)
	if err != nil {
		return nil, err
	}

	winning, responses, err := c.quorumRead(ctx, placement.ReplicaSet(), bucket, key)
	if err != nil {
		return nil, err
	}
	if !winning.Found || winning.Deleted {
		return nil, object.NotFoundKey()
	}

	c.scheduleReadRepair(ctx, bucket, key, winning, responses)

	return &metadata.Object{
		BucketName:     bucket,
		Key:            key,
		Size:           winning.Size,
		ContentType:    winning.ContentType,
		ETag:           winning.ETag,
		Version:        winning.Version,
		CustomMetadata: winning.CustomMetadata,
		Deleted:        false,
	}, nil
}

func (c *Coordinator) quorumRead(ctx context.Context, nodes []cluster.Node, bucket, key string) (ReplicaState, []ReplicaState, error) {
	states := c.collectReplicaStates(ctx, nodes, bucket, key)
	var found []ReplicaState
	for _, s := range states {
		if s.Found {
			found = append(found, s)
		}
	}

	metrics.RecordQuorumRead(c.quorum.R, len(found), len(nodes), "get")
	if !c.quorum.ReadSatisfied(len(found)) {
		metrics.RecordQuorumFailure("get")
		return ReplicaState{}, states, object.NewQuorumReadError(ErrQuorumNotMet)
	}

	winning, ok := PickNewestState(found)
	if !ok {
		metrics.RecordQuorumFailure("get")
		return ReplicaState{}, states, object.NewQuorumReadError(ErrQuorumNotMet)
	}
	return winning, states, nil
}

func (c *Coordinator) collectReplicaStates(ctx context.Context, nodes []cluster.Node, bucket, key string) []ReplicaState {
	states := make([]ReplicaState, 0, len(nodes))
	var remotes []cluster.Node
	for _, n := range nodes {
		if n.ID == c.localNodeID {
			states = append(states, c.localReplicaState(ctx, bucket, key))
		} else {
			remotes = append(remotes, n)
		}
	}
	if len(remotes) > 0 {
		remoteStates := c.replicator.FetchReplicaStates(ctx, remotes, bucket, key)
		states = append(states, remoteStates...)
	}
	return states
}

func (c *Coordinator) localReplicaState(ctx context.Context, bucket, key string) ReplicaState {
	found, obj, err := c.svc.LocalReplicaState(ctx, bucket, key)
	if err != nil || !found || obj == nil {
		return ReplicaState{NodeID: c.localNodeID, Found: false}
	}
	return ReplicaState{
		NodeID:         c.localNodeID,
		Found:          true,
		Version:        obj.Version,
		ETag:           obj.ETag,
		Size:           obj.Size,
		ContentType:    obj.ContentType,
		Deleted:        obj.Deleted,
		CustomMetadata: obj.CustomMetadata,
	}
}

func (c *Coordinator) scheduleReadRepair(ctx context.Context, bucket, key string, winning ReplicaState, states []ReplicaState) {
	for _, s := range states {
		if !s.Found || s.NodeID == winning.NodeID {
			continue
		}
		if s.Version >= winning.Version && s.ETag == winning.ETag && s.Deleted == winning.Deleted {
			continue
		}
		metrics.RecordStaleReplica()
		metrics.RecordReadRepair()
		if c.logger != nil {
			c.logger.Info("read repair scheduled",
				"bucket", bucket,
				"key", key,
				"stale_replica", s.NodeID,
				"stale_version", s.Version,
				"winning_version", winning.Version,
				"repair_target", s.NodeID,
			)
		}
		kind := metadata.HintKindRepair
		if winning.Deleted {
			kind = metadata.HintKindTombstone
		}
		_ = c.enqueueHint(ctx, kind, s.NodeID, bucket, key, winning.Version, winning.ETag, winning.ContentType, winning.CustomMetadata, winning.NodeID)
	}
}

func (c *Coordinator) enqueueHint(ctx context.Context, kind metadata.HintKind, targetNode, bucket, key string, version int64, etag, contentType string, custom map[string]string, sourceNode string) error {
	if c.meta == nil {
		return nil
	}
	hint := metadata.ReplicationHint{
		TargetNode:     targetNode,
		BucketName:     bucket,
		Key:            key,
		Version:        version,
		ETag:           etag,
		Kind:           kind,
		ContentType:    contentType,
		CustomMetadata: custom,
		SourceNode:     sourceNode,
		NextAttemptAt:  time.Now().UTC(),
		Status:         "pending",
	}
	if (kind == metadata.HintKindPut || kind == metadata.HintKindRepair) && sourceNode == c.localNodeID {
		path, _, err := c.svc.StageHintPayload(ctx, bucket, key, version, targetNode)
		if err != nil {
			if c.logger != nil {
				c.logger.Warn("hint payload staging failed", "bucket", bucket, "key", key, "version", version, "error", err)
			}
		} else {
			hint.SourcePath = path
		}
	}
	if err := c.meta.CreateHint(ctx, hint); err != nil {
		return err
	}
	metrics.RecordHintCreated(string(kind))
	return nil
}

// DeliverHint attempts to deliver a persisted replication hint.
func (c *Coordinator) DeliverHint(ctx context.Context, hint metadata.ReplicationHint) error {
	target, ok := c.nodeByID(hint.TargetNode)
	if !ok {
		return fmt.Errorf("unknown target node %s", hint.TargetNode)
	}

	switch hint.Kind {
	case metadata.HintKindTombstone:
		return c.replicator.deleteOnNode(ctx, target, hint.BucketName, hint.Key, hint.Version)
	case metadata.HintKindPut, metadata.HintKindRepair:
		source, ok := c.nodeByID(hint.SourceNode)
		if !ok {
			return fmt.Errorf("unknown source node %s", hint.SourceNode)
		}
		var openBody func() (io.ReadCloser, error)
		var size int64
		if hint.SourcePath != "" {
			stagedPath := hint.SourcePath
			rc, sz, err := c.svc.OpenStoragePath(stagedPath)
			if err != nil {
				return fmt.Errorf("open staged hint payload: %w", err)
			}
			rc.Close()
			size = sz
			openBody = func() (io.ReadCloser, error) {
				rc, _, err := c.svc.OpenStoragePath(stagedPath)
				return rc, err
			}
		} else if hint.SourceNode == c.localNodeID {
			openBody = func() (io.ReadCloser, error) {
				rc, sz, err := c.svc.OpenStoredObject(ctx, hint.BucketName, hint.Key)
				size = sz
				return rc, err
			}
		} else {
			openBody = func() (io.ReadCloser, error) {
				rc, sz, _, err := c.replicator.StreamFromNode(ctx, source, hint.BucketName, hint.Key)
				size = sz
				return rc, err
			}
		}
		return c.replicator.putToNode(ctx, target, PutReplicaInput{
			Bucket:         hint.BucketName,
			Key:            hint.Key,
			Version:        hint.Version,
			ExpectedETag:   hint.ETag,
			ContentType:    hint.ContentType,
			CustomMetadata: hint.CustomMetadata,
			Size:           size,
			OpenBody:       openBody,
		})
	default:
		return fmt.Errorf("unknown hint kind %q", hint.Kind)
	}
}

func (c *Coordinator) nodeByID(id string) (cluster.Node, bool) {
	if c.membership == nil {
		return cluster.Node{}, false
	}
	for _, n := range c.membership.ListNodes() {
		if n.ID == id {
			return n, true
		}
	}
	return cluster.Node{}, false
}

// GetObjectMetadata is a thin wrapper for tests.
func (c *Coordinator) GetObjectMetadata(ctx context.Context, bucket, key string) (*metadata.Object, error) {
	return c.svc.GetObjectMetadata(ctx, bucket, key)
}

// OpenStoredObject exposes blob read for tests.
func (c *Coordinator) OpenStoredObject(ctx context.Context, bucket, key string) (io.ReadCloser, int64, error) {
	return c.svc.OpenStoredObject(ctx, bucket, key)
}

// CleanupHintPayload removes a staged hint blob after successful delivery.
func (c *Coordinator) CleanupHintPayload(path string) {
	if c == nil || c.svc == nil {
		return
	}
	_ = c.svc.RemoveStoragePath(path)
}

// MetaStore exposes metadata for the hint worker.
func (c *Coordinator) MetaStore() metadata.Store {
	return c.meta
}

// HealPartialObjects scans objects with replication_status=partial on this primary and schedules repair hints.
func (c *Coordinator) HealPartialObjects(ctx context.Context, limit int) (int, error) {
	if c == nil || !c.Enabled() || c.meta == nil {
		return 0, nil
	}
	objects, err := c.meta.ListObjectsByReplicationStatus(ctx, "partial", limit)
	if err != nil {
		return 0, err
	}
	healed := 0
	for _, obj := range objects {
		if err := c.healObject(ctx, obj.BucketName, obj.Key); err != nil {
			if c.logger != nil {
				c.logger.Warn("heal object failed", "bucket", obj.BucketName, "key", obj.Key, "error", err)
			}
			continue
		}
		healed++
	}
	metrics.RecordHealingScan()
	return healed, nil
}

func (c *Coordinator) healObject(ctx context.Context, bucket, key string) error {
	placement, err := c.placer.Locate(bucket, key)
	if err != nil {
		return err
	}
	if placement.Primary.ID != c.localNodeID {
		return nil
	}

	obj, err := c.svc.GetObjectMetadata(ctx, bucket, key)
	if err != nil || obj == nil || obj.Deleted {
		return nil
	}

	winning := c.localReplicaState(ctx, bucket, key)
	if !winning.Found {
		return nil
	}

	states := c.collectReplicaStates(ctx, placement.ReplicaSet(), bucket, key)
	c.scheduleReadRepair(ctx, bucket, key, winning, states)

	if replicasInSync(winning, states) {
		_ = c.svc.SetReplicationStatus(ctx, bucket, key, "replicated")
	}
	return nil
}

func replicasInSync(winning ReplicaState, states []ReplicaState) bool {
	for _, s := range states {
		if !s.Found {
			return false
		}
		if s.Version < winning.Version || (s.Version == winning.Version && s.ETag != winning.ETag) {
			return false
		}
		if s.Deleted != winning.Deleted {
			return false
		}
	}
	return len(states) > 0
}

// OnPeerRecovered resets hint backoff and runs a healing pass for a recovered peer.
func (c *Coordinator) OnPeerRecovered(ctx context.Context, nodeID string, hints *HintWorker, healing *HealingWorker) {
	if c == nil || c.meta == nil {
		return
	}
	if err := c.meta.ResetHintsForTarget(ctx, nodeID); err != nil && c.logger != nil {
		c.logger.Warn("reset hints for recovered peer failed", "peer", nodeID, "error", err)
	}
	if hints != nil {
		hints.ProcessDueHintsOnce(ctx)
	}
	if healing != nil {
		healing.ScanOnce(ctx)
	}
}
