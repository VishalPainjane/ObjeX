package cluster_test

import (
	"testing"

	"github.com/VishalPainjane/objex/internal/cluster"
)

func testNodes(ids ...string) []cluster.Node {
	out := make([]cluster.Node, len(ids))
	for i, id := range ids {
		out[i] = cluster.Node{ID: id, Address: "localhost:" + id, Status: cluster.NodeStatusActive}
	}
	return out
}

func TestPlacementDeterminism(t *testing.T) {
	mem, err := cluster.NewStaticMembership("node-1", testNodes("node-1", "node-2", "node-3"))
	if err != nil {
		t.Fatal(err)
	}
	p := cluster.NewRendezvousPlacer(mem, 1, nil)

	first, err := p.Locate("photos", "cat.jpg")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		got, err := p.Locate("photos", "cat.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if got.Primary.ID != first.Primary.ID {
			t.Fatalf("iteration %d: got %q want %q", i, got.Primary.ID, first.Primary.ID)
		}
	}
}

func TestPlacementNodeOrderIndependent(t *testing.T) {
	nodesA := []cluster.Node{
		{ID: "node-3", Address: "a:3", Status: cluster.NodeStatusActive},
		{ID: "node-1", Address: "a:1", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "a:2", Status: cluster.NodeStatusActive},
	}
	nodesB := []cluster.Node{
		{ID: "node-1", Address: "b:1", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "b:2", Status: cluster.NodeStatusActive},
		{ID: "node-3", Address: "b:3", Status: cluster.NodeStatusActive},
	}
	memA, _ := cluster.NewStaticMembership("node-1", nodesA)
	memB, _ := cluster.NewStaticMembership("node-1", nodesB)
	pA := cluster.NewRendezvousPlacer(memA, 1, nil)
	pB := cluster.NewRendezvousPlacer(memB, 1, nil)

	keys := []string{"a", "photos/cat.jpg", "deep/path/file.bin", "x"}
	for _, key := range keys {
		rA, _ := pA.Locate("bucket", key)
		rB, _ := pB.Locate("bucket", key)
		if rA.Primary.ID != rB.Primary.ID {
			t.Fatalf("key %q: A=%q B=%q", key, rA.Primary.ID, rB.Primary.ID)
		}
	}
}

func TestPlacementEmptyCluster(t *testing.T) {
	p := cluster.NewRendezvousPlacer(emptyMembership{}, 1, nil)
	_, err := p.Locate("b", "k")
	if err != cluster.ErrEmptyCluster {
		t.Fatalf("err = %v", err)
	}
}

type emptyMembership struct{}

func (emptyMembership) LocalNodeID() string       { return "node-1" }
func (emptyMembership) GetNode(string) (cluster.Node, bool) { return cluster.Node{}, false }
func (emptyMembership) ListNodes() []cluster.Node { return nil }
func (emptyMembership) Contains(string) bool      { return false }
func (emptyMembership) Len() int                  { return 0 }

func TestPlacementOneNode(t *testing.T) {
	mem, _ := cluster.NewStaticMembership("node-1", testNodes("node-1"))
	p := cluster.NewRendezvousPlacer(mem, 1, nil)
	r, err := p.Locate("b", "any-key")
	if err != nil {
		t.Fatal(err)
	}
	if r.Primary.ID != "node-1" {
		t.Fatalf("got %q", r.Primary.ID)
	}
}

func TestPlacementDistributesAcrossNodes(t *testing.T) {
	mem, _ := cluster.NewStaticMembership("node-1", testNodes("node-1", "node-2", "node-3"))
	p := cluster.NewRendezvousPlacer(mem, 1, nil)
	counts := map[string]int{}
	const n = 10000
	for i := 0; i < n; i++ {
		r, err := p.Locate("bucket", "object-"+itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		counts[r.Primary.ID]++
	}
	for id, c := range counts {
		if c == 0 {
			t.Fatalf("node %q received zero objects", id)
		}
		t.Logf("distribution: %s = %d (%.1f%%)", id, c, float64(c)*100/float64(n))
	}
}

func TestMembershipChangeRemapRatio(t *testing.T) {
	three := testNodes("node-1", "node-2", "node-3")
	mem3, _ := cluster.NewStaticMembership("node-1", three)
	p3 := cluster.NewRendezvousPlacer(mem3, 1, nil)

	const n = 10000
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = "object-" + itoa(i)
	}

	before := make(map[string]string, n)
	for _, key := range keys {
		r, _ := p3.Locate("bucket", key)
		before[key] = r.Primary.ID
	}

	four := testNodes("node-1", "node-2", "node-3", "node-4")
	mem4, _ := cluster.NewStaticMembership("node-1", four)
	p4 := cluster.NewRendezvousPlacer(mem4, 1, nil)

	changed := 0
	for _, key := range keys {
		r, _ := p4.Locate("bucket", key)
		if r.Primary.ID != before[key] {
			changed++
		}
	}
	ratio := float64(changed) / float64(n)
	t.Logf("placement changed for %d/%d objects (%.1f%%) after adding node-4", changed, n, ratio*100)
	if ratio > 0.85 {
		t.Fatalf("too many keys moved (%.1f%%); rendezvous should move ~25%% on add", ratio*100)
	}
}

func TestPlacementReplicationFactor(t *testing.T) {
	nodes := testNodes("node-1", "node-2", "node-3")
	mem, _ := cluster.NewStaticMembership("node-1", nodes)

	for _, rf := range []int{1, 2, 3} {
		p := cluster.NewRendezvousPlacer(mem, rf, nil)
		r, err := p.Locate("bucket", "photos/cat.jpg")
		if err != nil {
			t.Fatalf("RF=%d: %v", rf, err)
		}
		set := r.ReplicaSet()
		if len(set) != rf {
			t.Fatalf("RF=%d: got %d nodes", rf, len(set))
		}
		seen := map[string]bool{}
		for _, n := range set {
			if seen[n.ID] {
				t.Fatalf("RF=%d: duplicate node %s", rf, n.ID)
			}
			seen[n.ID] = true
		}
		if r.Primary.ID != set[0].ID {
			t.Fatalf("RF=%d: primary not first in set", rf)
		}
	}
}

func TestPlacementRFExceedsCluster(t *testing.T) {
	mem, _ := cluster.NewStaticMembership("node-1", testNodes("node-1", "node-2"))
	p := cluster.NewRendezvousPlacer(mem, 3, nil)
	_, err := p.Locate("b", "k")
	if err != cluster.ErrReplicationFactorTooLarge {
		t.Fatalf("err = %v", err)
	}
}

func TestPlacementRFDeterministicReplicas(t *testing.T) {
	nodesA := []cluster.Node{
		{ID: "node-3", Address: "a:3", Status: cluster.NodeStatusActive},
		{ID: "node-1", Address: "a:1", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "a:2", Status: cluster.NodeStatusActive},
	}
	nodesB := []cluster.Node{
		{ID: "node-1", Address: "b:1", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "b:2", Status: cluster.NodeStatusActive},
		{ID: "node-3", Address: "b:3", Status: cluster.NodeStatusActive},
	}
	memA, _ := cluster.NewStaticMembership("node-1", nodesA)
	memB, _ := cluster.NewStaticMembership("node-1", nodesB)
	pA := cluster.NewRendezvousPlacer(memA, 3, nil)
	pB := cluster.NewRendezvousPlacer(memB, 3, nil)

	rA, _ := pA.Locate("bucket", "key")
	rB, _ := pB.Locate("bucket", "key")
	if rA.Primary.ID != rB.Primary.ID {
		t.Fatalf("primary mismatch: %q vs %q", rA.Primary.ID, rB.Primary.ID)
	}
	for i := range rA.Replicas {
		if rA.Replicas[i].ID != rB.Replicas[i].ID {
			t.Fatalf("replica %d mismatch", i)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
