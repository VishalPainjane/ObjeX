package cluster

import (
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"sort"
)

// PlacementResult names the primary node and replica peers for an object.
type PlacementResult struct {
	Primary  Node
	Replicas []Node
}

// ReplicaSet returns all nodes that should store the object (primary first).
func (p PlacementResult) ReplicaSet() []Node {
	out := make([]Node, 0, 1+len(p.Replicas))
	out = append(out, p.Primary)
	out = append(out, p.Replicas...)
	return out
}

// Placer deterministically maps objects to nodes.
type Placer interface {
	Locate(bucket, key string) (PlacementResult, error)
	ReplicationFactor() int
}

// RendezvousPlacer uses highest-random-weight (rendezvous) hashing over sorted node IDs.
type RendezvousPlacer struct {
	membership        Membership
	replicationFactor int
	logger            *slog.Logger
}

// NewRendezvousPlacer creates a placement engine backed by static membership.
func NewRendezvousPlacer(membership Membership, replicationFactor int, logger *slog.Logger) *RendezvousPlacer {
	if replicationFactor <= 0 {
		replicationFactor = 1
	}
	return &RendezvousPlacer{
		membership:        membership,
		replicationFactor: replicationFactor,
		logger:            logger,
	}
}

// ReplicationFactor returns the configured replication factor.
func (p *RendezvousPlacer) ReplicationFactor() int {
	return p.replicationFactor
}

// Locate returns the primary node and replica peers for bucket/key.
func (p *RendezvousPlacer) Locate(bucket, key string) (PlacementResult, error) {
	nodes := p.membership.ListNodes()
	if len(nodes) == 0 {
		return PlacementResult{}, ErrEmptyCluster
	}
	if p.replicationFactor > len(nodes) {
		return PlacementResult{}, ErrReplicationFactorTooLarge
	}

	objectKey := placementObjectKey(bucket, key)
	type scored struct {
		idx   int
		score uint64
		id    string
	}
	ranked := make([]scored, len(nodes))
	for i, n := range nodes {
		ranked[i] = scored{
			idx:   i,
			score: rendezvousScore(objectKey, n.ID),
			id:    n.ID,
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].id < ranked[j].id
	})

	primary := nodes[ranked[0].idx]
	replicas := make([]Node, 0, p.replicationFactor-1)
	for i := 1; i < p.replicationFactor; i++ {
		replicas = append(replicas, nodes[ranked[i].idx])
	}

	if p.logger != nil {
		replicaIDs := make([]string, len(replicas))
		for i, r := range replicas {
			replicaIDs[i] = r.ID
		}
		p.logger.Debug("placement decision",
			"bucket", bucket,
			"key", key,
			"primary_node", primary.ID,
			"replicas", replicaIDs,
			"replication_factor", p.replicationFactor,
		)
	}
	return PlacementResult{Primary: primary, Replicas: replicas}, nil
}

func placementObjectKey(bucket, key string) string {
	return bucket + "/" + key
}

func rendezvousScore(objectKey, nodeID string) uint64 {
	sum := sha256.Sum256([]byte(objectKey + ":" + nodeID))
	return binary.BigEndian.Uint64(sum[:8])
}
