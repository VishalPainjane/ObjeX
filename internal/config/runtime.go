package config

// ApplyRuntimeOverrides updates config from CLI flags (non-empty values win).
func (c *Config) ApplyRuntimeOverrides(nodeID, httpAddr string) {
	if nodeID != "" {
		c.NodeID = nodeID
	}
	if httpAddr != "" {
		c.HTTPAddress = httpAddr
	}
}

// RebuildLocalClusterAddress updates the local node's address (and single-node ID) after runtime overrides.
func (c *Config) RebuildLocalClusterAddress() {
	addr := localNodeAddress(c.PublicURL, c.HTTPAddress)
	if len(c.ClusterNodes) == 1 {
		c.ClusterNodes[0].ID = c.NodeID
		c.ClusterNodes[0].Address = addr
		return
	}
	for i, n := range c.ClusterNodes {
		if n.ID == c.NodeID {
			c.ClusterNodes[i].Address = addr
			break
		}
	}
}
