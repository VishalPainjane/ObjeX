package cluster_test

import (
	"testing"

	"github.com/VishalPainjane/objex/internal/cluster"
)

func BenchmarkRendezvousLocate(b *testing.B) {
	mem, err := cluster.NewStaticMembership("node-1", testNodes("node-1", "node-2", "node-3"))
	if err != nil {
		b.Fatal(err)
	}
	p := cluster.NewRendezvousPlacer(mem, 1, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Locate("photos", "cat.jpg")
		if err != nil {
			b.Fatal(err)
		}
	}
}
