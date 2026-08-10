package cluster

// NodeStatus describes configured membership state (not runtime health).
type NodeStatus string

const (
	// NodeStatusActive marks a node as part of the configured cluster.
	NodeStatusActive NodeStatus = "active"
)

// Node is a storage node in the cluster membership view.
type Node struct {
	ID      string
	Address string
	Status  NodeStatus
}
