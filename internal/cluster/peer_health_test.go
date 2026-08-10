package cluster_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VishalPainjane/objex/internal/cluster"
)

func TestPeerHealthTrackerMarksDownAndRecovered(t *testing.T) {
	nodes := []cluster.Node{
		{ID: "node-1", Address: "http://localhost:1", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "http://localhost:2", Status: cluster.NodeStatusActive},
	}
	mem, err := cluster.NewStaticMembership("node-1", nodes)
	if err != nil {
		t.Fatal(err)
	}
	tracker := cluster.NewPeerHealthTracker(mem)

	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	downSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer downSrv.Close()

	nodes[1].Address = downSrv.URL
	mem, _ = cluster.NewStaticMembership("node-1", nodes)

	recovered := make(chan string, 1)
	monitor := cluster.NewPeerHealthMonitor(mem, tracker, cluster.PeerHealthConfig{
		Interval:         time.Hour,
		Timeout:          time.Second,
		FailThreshold:    2,
		RecoverThreshold: 2,
		OnRecovered: func(nodeID string) {
			recovered <- nodeID
		},
	}, nil)

	ctx := context.Background()
	monitor.ProbeOnce(ctx)
	monitor.ProbeOnce(ctx)
	if tracker.IsReachable("node-2") {
		t.Fatal("expected node-2 unreachable after failed probes")
	}

	nodes[1].Address = okSrv.URL
	mem, _ = cluster.NewStaticMembership("node-1", nodes)
	monitor = cluster.NewPeerHealthMonitor(mem, tracker, cluster.PeerHealthConfig{
		Interval:         time.Hour,
		Timeout:          time.Second,
		FailThreshold:    2,
		RecoverThreshold: 2,
		OnRecovered: func(nodeID string) {
			recovered <- nodeID
		},
	}, nil)
	monitor.ProbeOnce(ctx)
	monitor.ProbeOnce(ctx)
	if !tracker.IsReachable("node-2") {
		t.Fatal("expected node-2 reachable after recovery probes")
	}

	select {
	case id := <-recovered:
		if id != "node-2" {
			t.Fatalf("recovery callback for %q", id)
		}
	default:
		t.Fatal("expected recovery callback")
	}
}
