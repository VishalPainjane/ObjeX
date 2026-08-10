package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClusterFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cluster.json")
	content := `{
  "cluster": {
    "nodes": [
      {"id": "node-1", "address": "localhost:9001"},
      {"id": "node-2", "address": "localhost:9002"}
    ]
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OBJEX_CLUSTER_CONFIG", path)
	t.Setenv("OBJEX_NODE_ID", "node-1")
	t.Setenv("OBJEX_CLUSTER_NODES", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ClusterNodes) != 2 {
		t.Fatalf("nodes = %d", len(cfg.ClusterNodes))
	}
}

func TestDefaultSingleNodeCluster(t *testing.T) {
	t.Setenv("OBJEX_CLUSTER_CONFIG", "")
	t.Setenv("OBJEX_CLUSTER_NODES", "")
	t.Setenv("OBJEX_NODE_ID", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NodeID != "node-1" {
		t.Fatalf("NodeID = %q", cfg.NodeID)
	}
	if len(cfg.ClusterNodes) != 1 {
		t.Fatalf("nodes = %d", len(cfg.ClusterNodes))
	}
}

func TestApplyRuntimeOverrides(t *testing.T) {
	cfg := Config{
		NodeID:       "node-1",
		HTTPAddress:  ":9000",
		ClusterNodes: []ClusterNodeConfig{{ID: "node-1", Address: "localhost:9000"}},
		PublicURL:    "http://localhost:9000",
	}
	cfg.ApplyRuntimeOverrides("node-2", ":9002")
	cfg.RebuildLocalClusterAddress()
	if cfg.NodeID != "node-2" {
		t.Fatalf("NodeID = %q", cfg.NodeID)
	}
	if cfg.HTTPAddress != ":9002" {
		t.Fatalf("HTTPAddress = %q", cfg.HTTPAddress)
	}
	if cfg.ClusterNodes[0].ID != "node-2" {
		t.Fatalf("cluster node id = %q", cfg.ClusterNodes[0].ID)
	}
}
