package cluster

import "errors"

var (
	// ErrEmptyCluster is returned when placement runs against zero nodes.
	ErrEmptyCluster = errors.New("cluster has no nodes")
	// ErrNodeNotFound is returned when a node ID is not in membership.
	ErrNodeNotFound = errors.New("node not found in cluster")
	// ErrReplicationFactorTooLarge is returned when RF exceeds cluster size.
	ErrReplicationFactorTooLarge = errors.New("replication factor exceeds cluster size")
)
