package replication_test

import (
	"testing"

	"github.com/VishalPainjane/objex/internal/replication"
)

func TestPickNewestState(t *testing.T) {
	states := []replication.ReplicaState{
		{NodeID: "n1", Found: true, Version: 8, ETag: "a"},
		{NodeID: "n2", Found: true, Version: 7, ETag: "b"},
		{NodeID: "n3", Found: true, Version: 8, ETag: "a"},
	}
	best, ok := replication.PickNewestState(states)
	if !ok || best.Version != 8 {
		t.Fatalf("expected version 8, got %+v", best)
	}
}

func TestPickNewestStateTombstoneWins(t *testing.T) {
	states := []replication.ReplicaState{
		{NodeID: "n1", Found: true, Version: 10, Deleted: true},
		{NodeID: "n2", Found: true, Version: 9, ETag: "x"},
	}
	best, ok := replication.PickNewestState(states)
	if !ok || best.Version != 10 || !best.Deleted {
		t.Fatalf("tombstone should win: %+v", best)
	}
}
