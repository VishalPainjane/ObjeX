package cluster

import (
	"fmt"
	"sort"
)

// Membership provides static cluster membership.
type Membership interface {
	LocalNodeID() string
	GetNode(id string) (Node, bool)
	ListNodes() []Node
	Contains(id string) bool
	Len() int
}

// StaticMembership is membership loaded from configuration.
// Node order is always sorted by ID for deterministic placement.
type StaticMembership struct {
	localID string
	nodes   []Node
	byID    map[string]Node
}

// NewStaticMembership builds membership from nodes. localID must exist in nodes.
func NewStaticMembership(localID string, nodes []Node) (*StaticMembership, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("cluster membership cannot be empty")
	}
	byID := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		if n.ID == "" {
			return nil, fmt.Errorf("cluster node missing id")
		}
		if n.Address == "" {
			return nil, fmt.Errorf("cluster node %q missing address", n.ID)
		}
		if _, exists := byID[n.ID]; exists {
			return nil, fmt.Errorf("duplicate cluster node id %q", n.ID)
		}
		status := n.Status
		if status == "" {
			status = NodeStatusActive
		}
		byID[n.ID] = Node{ID: n.ID, Address: n.Address, Status: status}
	}
	if _, ok := byID[localID]; !ok {
		return nil, fmt.Errorf("local node id %q not found in cluster membership", localID)
	}

	sorted := make([]Node, 0, len(byID))
	for _, n := range byID {
		sorted = append(sorted, n)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	return &StaticMembership{
		localID: localID,
		nodes:   sorted,
		byID:    byID,
	}, nil
}

func (m *StaticMembership) LocalNodeID() string {
	return m.localID
}

func (m *StaticMembership) GetNode(id string) (Node, bool) {
	n, ok := m.byID[id]
	return n, ok
}

func (m *StaticMembership) ListNodes() []Node {
	out := make([]Node, len(m.nodes))
	copy(out, m.nodes)
	return out
}

func (m *StaticMembership) Contains(id string) bool {
	_, ok := m.byID[id]
	return ok
}

func (m *StaticMembership) Len() int {
	return len(m.nodes)
}
