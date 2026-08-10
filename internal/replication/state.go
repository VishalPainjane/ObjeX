package replication

// ReplicaState is metadata returned by replicate-head / local replica inspection.
type ReplicaState struct {
	NodeID         string
	Found          bool
	Version        int64
	ETag           string
	Size           int64
	ContentType    string
	Deleted        bool
	CustomMetadata map[string]string
}

// PickNewestState selects the highest-version replica state from responses.
func PickNewestState(states []ReplicaState) (ReplicaState, bool) {
	var best ReplicaState
	var ok bool
	for _, s := range states {
		if !s.Found {
			continue
		}
		if !ok || s.Version > best.Version {
			best = s
			ok = true
		}
	}
	return best, ok
}
