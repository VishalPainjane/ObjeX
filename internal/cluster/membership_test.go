package cluster_test

import (
	"testing"

	"github.com/VishalPainjane/objex/internal/cluster"
)

func TestStaticMembershipSorted(t *testing.T) {
	nodes := []cluster.Node{
		{ID: "node-3", Address: "a:3"},
		{ID: "node-1", Address: "a:1"},
		{ID: "node-2", Address: "a:2"},
	}
	mem, err := cluster.NewStaticMembership("node-2", nodes)
	if err != nil {
		t.Fatal(err)
	}
	list := mem.ListNodes()
	if len(list) != 3 {
		t.Fatalf("len = %d", len(list))
	}
	want := []string{"node-1", "node-2", "node-3"}
	for i, id := range want {
		if list[i].ID != id {
			t.Fatalf("index %d: got %q want %q", i, list[i].ID, id)
		}
	}
}

func TestStaticMembershipDuplicateID(t *testing.T) {
	nodes := []cluster.Node{
		{ID: "node-1", Address: "a:1"},
		{ID: "node-1", Address: "a:2"},
	}
	_, err := cluster.NewStaticMembership("node-1", nodes)
	if err == nil {
		t.Fatal("expected error for duplicate id")
	}
}

func TestStaticMembershipLocalNotFound(t *testing.T) {
	nodes := []cluster.Node{{ID: "node-1", Address: "a:1"}}
	_, err := cluster.NewStaticMembership("node-2", nodes)
	if err == nil {
		t.Fatal("expected error when local id missing")
	}
}
