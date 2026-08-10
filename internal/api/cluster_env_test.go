package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/VishalPainjane/objex/internal/api"
	"github.com/VishalPainjane/objex/internal/auth"
	"github.com/VishalPainjane/objex/internal/cluster"
	"github.com/VishalPainjane/objex/internal/metadata/sqlite"
	"github.com/VishalPainjane/objex/internal/metrics"
	"github.com/VishalPainjane/objex/internal/object"
	"github.com/VishalPainjane/objex/internal/quorum"
	"github.com/VishalPainjane/objex/internal/replication"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

// clusterTestEnv holds wired cluster test dependencies.
type clusterTestEnv struct {
	Handler     *api.Handler
	Root        http.Handler
	Meta        *sqlite.Store
	Coordinator *replication.Coordinator
	HintWorker  *replication.HintWorker
	DataDir     string
}

func buildClusterHandlerRF(t *testing.T, localID string, nodes []cluster.Node, replicationFactor int) (*api.Handler, http.Handler) {
	env := buildClusterTestEnv(t, localID, nodes, replicationFactor, "")
	return env.Handler, env.Root
}

func buildClusterTestEnv(t *testing.T, localID string, nodes []cluster.Node, replicationFactor int, dataDir string) clusterTestEnv {
	t.Helper()
	if dataDir == "" {
		dataDir = t.TempDir()
	}
	meta, err := sqlite.Open(dataDir + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { meta.Close() })

	if err := meta.UpsertCredential(context.Background(), "test", testAccessKeyID, testSecretKey); err != nil {
		t.Fatal(err)
	}

	blob, err := filesystem.New(dataDir+"/blobs", nil)
	if err != nil {
		t.Fatal(err)
	}

	mem, err := cluster.NewStaticMembership(localID, nodes)
	if err != nil {
		t.Fatal(err)
	}
	placer := cluster.NewRendezvousPlacer(mem, replicationFactor, nil)
	svc := object.NewService(meta, blob, 0, 0)
	svc.SetPlacement(placer, nil)
	proxy := cluster.NewProxy(testClusterInternalToken, srvClient())
	peerSync := cluster.NewPeerSync(mem, localID, testClusterInternalToken, srvClient())
	replicator := replication.NewReplicator(testClusterInternalToken, srvClient(), nil)
	w, r := quorum.DefaultsForN(replicationFactor)
	qcfg := quorum.Config{N: replicationFactor, W: w, R: r}
	replCoord := replication.NewCoordinator(localID, mem, placer, svc, meta, replicator, qcfg, nil)
	hintWorker := replication.NewHintWorker(replCoord, nil)
	go hintWorker.Run(context.Background())

	h := api.NewHandlerWithConfig(svc, api.HandlerConfig{
		LocalNodeID:   localID,
		Membership:    mem,
		Placement:     placer,
		Proxy:         proxy,
		PeerSync:      peerSync,
		Replication:   replCoord,
		InternalToken: testClusterInternalToken,
	}, nil)
	h.SetReadyCheck(func(ctx context.Context) error {
		return meta.DB().PingContext(ctx)
	})

	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	h.Register(mux)

	verifier := &auth.Verifier{Region: "us-east-1", Store: meta.Credentials()}
	authMW := &auth.Middleware{
		Verifier:      verifier,
		Skip:          auth.DefaultSkip,
		InternalToken: testClusterInternalToken,
	}
	return clusterTestEnv{
		Handler:     h,
		Root:        metrics.Middleware(authMW.Handler(mux)),
		Meta:        meta,
		Coordinator: replCoord,
		HintWorker:  hintWorker,
		DataDir:     dataDir,
	}
}
