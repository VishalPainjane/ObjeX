package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/VishalPainjane/objex/internal/cluster"
)

func TestClusterEndpointHTTP(t *testing.T) {
	srv := newTestClusterHTTP(t, "node-1", []cluster.Node{
		{ID: "node-1", Address: "localhost:9001", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "localhost:9002", Status: cluster.NodeStatusActive},
	})
	client := srv.Client()

	resp, err := client.Get(srv.URL + "/cluster")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		NodeID string `json:"node_id"`
		Nodes  []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.NodeID != "node-1" {
		t.Fatalf("node_id = %q", out.NodeID)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("nodes = %d", len(out.Nodes))
	}
}

func TestPlacementDebugEndpointHTTP(t *testing.T) {
	srv := newTestClusterHTTP(t, "node-1", []cluster.Node{
		{ID: "node-1", Address: "localhost:9001", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "localhost:9002", Status: cluster.NodeStatusActive},
		{ID: "node-3", Address: "localhost:9003", Status: cluster.NodeStatusActive},
	})
	client := srv.Client()

	resp, err := client.Get(srv.URL + "/debug/placement?bucket=photos&key=cat.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Bucket  string `json:"bucket"`
		Key     string `json:"key"`
		Primary string `json:"primary"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Bucket != "photos" || out.Key != "cat.jpg" || out.Primary == "" {
		t.Fatalf("unexpected body: %s", body)
	}
}
