package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const defaultNodeID = "node-1"

// ClusterNodeConfig describes a node in static cluster membership.
type ClusterNodeConfig struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type clusterFile struct {
	Cluster *struct {
		Nodes []ClusterNodeConfig `json:"nodes"`
	} `json:"cluster"`
	Nodes []ClusterNodeConfig `json:"nodes"`
}

func loadClusterNodes(nodeID, publicURL, httpAddr string) (string, []ClusterNodeConfig, error) {
	if path := os.Getenv("OBJEX_CLUSTER_CONFIG"); path != "" {
		nodes, err := parseClusterFile(path)
		if err != nil {
			return "", nil, err
		}
		id := envOr("OBJEX_NODE_ID", nodeID)
		if id == "" {
			id = defaultNodeID
		}
		return id, nodes, nil
	}

	if raw := os.Getenv("OBJEX_CLUSTER_NODES"); raw != "" {
		var nodes []ClusterNodeConfig
		if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
			return "", nil, fmt.Errorf("invalid OBJEX_CLUSTER_NODES JSON: %w", err)
		}
		id := envOr("OBJEX_NODE_ID", nodeID)
		if id == "" {
			id = defaultNodeID
		}
		return id, nodes, nil
	}

	id := envOr("OBJEX_NODE_ID", nodeID)
	if id == "" {
		id = defaultNodeID
	}
	addr := localNodeAddress(publicURL, httpAddr)
	return id, []ClusterNodeConfig{{ID: id, Address: addr}}, nil
}

func parseClusterFile(path string) ([]ClusterNodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cluster config %q: %w", path, err)
	}
	var file clusterFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse cluster config %q: %w", path, err)
	}
	if file.Cluster != nil && len(file.Cluster.Nodes) > 0 {
		return file.Cluster.Nodes, nil
	}
	if len(file.Nodes) > 0 {
		return file.Nodes, nil
	}
	return nil, fmt.Errorf("cluster config %q has no nodes", path)
}

func localNodeAddress(publicURL, httpAddr string) string {
	if publicURL != "" {
		u, err := url.Parse(publicURL)
		if err == nil && u.Host != "" {
			return u.Host
		}
	}
	httpAddr = strings.TrimSpace(httpAddr)
	if httpAddr == "" {
		return "localhost:9000"
	}
	if strings.HasPrefix(httpAddr, ":") {
		return "localhost" + httpAddr
	}
	if idx := strings.LastIndex(httpAddr, ":"); idx > 0 {
		host := httpAddr[:idx]
		if host == "" || host == "0.0.0.0" || host == "[::]" {
			port := httpAddr[idx+1:]
			return "localhost:" + port
		}
	}
	return httpAddr
}
