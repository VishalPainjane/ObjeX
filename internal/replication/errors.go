package replication

import (
	"errors"
	"fmt"
)

var (
	// ErrReplicationFailed is returned when one or more replicas fail.
	ErrReplicationFailed = errors.New("replication failed")
	// ErrNotPrimary is returned when a replicated operation runs on a non-primary node.
	ErrNotPrimary = errors.New("local node is not primary for object")
)

// ReplicaError describes a single replica operation failure.
type ReplicaError struct {
	NodeID string
	Err    error
}

func (e *ReplicaError) Error() string {
	return fmt.Sprintf("replica %s: %v", e.NodeID, e.Err)
}

// MultiReplicaError aggregates replica failures.
type MultiReplicaError struct {
	Failures []ReplicaError
}

func (e *MultiReplicaError) Error() string {
	return fmt.Sprintf("replication failed on %d replica(s)", len(e.Failures))
}

func (e *MultiReplicaError) Unwrap() []error {
	out := make([]error, len(e.Failures))
	for i, f := range e.Failures {
		out[i] = &f
	}
	return out
}
