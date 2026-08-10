package cluster_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VishalPainjane/objex/internal/cluster"
)

func threeNodeConfigs() []cluster.Node {
	return []cluster.Node{
		{ID: "node-1", Address: "localhost:9001", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "localhost:9002", Status: cluster.NodeStatusActive},
		{ID: "node-3", Address: "localhost:9003", Status: cluster.NodeStatusActive},
	}
}

func startSimulatedNode(t *testing.T, localID string, nodes []cluster.Node) (*httptest.Server, cluster.Placer) {
	mem, err := cluster.NewStaticMembership(localID, nodes)
	if err != nil {
		t.Fatal(err)
	}
	placer := cluster.NewRendezvousPlacer(mem, 1, nil)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cluster":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"node_id":"` + localID + `"}`))
		case "/debug/placement":
			bucket := r.URL.Query().Get("bucket")
			key := r.URL.Query().Get("key")
			result, err := placer.Locate(bucket, key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"primary":"` + result.Primary.ID + `"}`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, placer
}

func TestThreeNodePlacementAgreement(t *testing.T) {
	nodes := threeNodeConfigs()
	placers := make([]cluster.Placer, 3)
	for i, id := range []string{"node-1", "node-2", "node-3"} {
		mem, err := cluster.NewStaticMembership(id, nodes)
		if err != nil {
			t.Fatal(err)
		}
		placers[i] = cluster.NewRendezvousPlacer(mem, 1, nil)
	}

	const n = 5000
	for i := 0; i < n; i++ {
		key := "object-" + itoa(i)
		ref, err := placers[0].Locate("bucket", key)
		if err != nil {
			t.Fatal(err)
		}
		for j := 1; j < len(placers); j++ {
			got, err := placers[j].Locate("bucket", key)
			if err != nil {
				t.Fatal(err)
			}
			if got.Primary.ID != ref.Primary.ID {
				t.Fatalf("key %s: node-%d=%q ref=%q", key, j+1, got.Primary.ID, ref.Primary.ID)
			}
		}
	}
}

func TestSimulatedClusterHTTPPlacement(t *testing.T) {
	nodes := threeNodeConfigs()
	srv1, _ := startSimulatedNode(t, "node-1", nodes)
	srv2, _ := startSimulatedNode(t, "node-2", nodes)

	resp1, err := http.Get(srv1.URL + "/debug/placement?bucket=photos&key=cat.jpg")
	if err != nil {
		t.Fatal(err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("status1 = %d", resp1.StatusCode)
	}

	resp2, err := http.Get(srv2.URL + "/debug/placement?bucket=photos&key=cat.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status2 = %d", resp2.StatusCode)
	}
}
