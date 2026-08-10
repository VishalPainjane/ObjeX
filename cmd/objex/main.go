package objex

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/VishalPainjane/objex/internal/api"
	"github.com/VishalPainjane/objex/internal/auth"
	"github.com/VishalPainjane/objex/internal/cluster"
	"github.com/VishalPainjane/objex/internal/config"
	"github.com/VishalPainjane/objex/internal/jobs"
	"github.com/VishalPainjane/objex/internal/metadata/sqlite"
	"github.com/VishalPainjane/objex/internal/metrics"
	"github.com/VishalPainjane/objex/internal/object"
	"github.com/VishalPainjane/objex/internal/quorum"
	"github.com/VishalPainjane/objex/internal/replication"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

func main() {
	nodeIDFlag := flag.String("node-id", "", "Node ID (overrides OBJEX_NODE_ID)")
	httpFlag := flag.String("http", "", "HTTP listen address (overrides OBJEX_HTTP_ADDRESS)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}
	cfg.ApplyRuntimeOverrides(*nodeIDFlag, *httpFlag)
	cfg.RebuildLocalClusterAddress()

	cfg, err = cfg.AbsPaths()
	if err != nil {
		logger.Error("config paths failed", "error", err)
		os.Exit(1)
	}

	clusterNodes := make([]cluster.Node, len(cfg.ClusterNodes))
	for i, n := range cfg.ClusterNodes {
		clusterNodes[i] = cluster.Node{
			ID:      n.ID,
			Address: n.Address,
			Status:  cluster.NodeStatusActive,
		}
	}
	membership, err := cluster.NewStaticMembership(cfg.NodeID, clusterNodes)
	if err != nil {
		logger.Error("cluster membership failed", "error", err)
		os.Exit(1)
	}
	placer := cluster.NewRendezvousPlacer(membership, cfg.ReplicationFactor, logger)

	members := make([]metrics.ClusterMember, len(clusterNodes))
	for i, n := range membership.ListNodes() {
		members[i] = metrics.ClusterMember{ID: n.ID, Address: n.Address}
	}
	metrics.SetClusterMembership(members)

	meta, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		logger.Error("metadata store failed", "error", err)
		os.Exit(1)
	}
	defer meta.Close()

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		if err := meta.UpsertCredential(context.Background(), "seed", cfg.AccessKeyID, cfg.SecretAccessKey); err != nil {
			logger.Error("seed credential failed", "error", err)
			os.Exit(1)
		}
	} else if cred, _ := meta.FirstCredential(context.Background()); cred == nil {
		logger.Warn("no S3 credentials configured; set OBJEX_ACCESS_KEY_ID and OBJEX_SECRET_ACCESS_KEY")
	}

	blob, err := filesystem.New(cfg.BlobPath, logger)
	if err != nil {
		logger.Error("blob storage failed", "error", err)
		os.Exit(1)
	}

	svc := object.NewService(meta, blob, cfg.MaxUploadBytes, cfg.MinFreeDiskBytes)
	svc.SetPlacement(placer, logger)

	proxy := cluster.NewProxy(cfg.ClusterInternalToken, nil)
	peerSync := cluster.NewPeerSync(membership, cfg.NodeID, cfg.ClusterInternalToken, nil)
	replicator := replication.NewReplicator(cfg.ClusterInternalToken, nil, logger)
	w, r := quorum.DefaultsForN(cfg.ReplicationFactor)
	qcfg := quorum.Config{N: cfg.ReplicationFactor, W: cfg.WriteQuorum, R: cfg.ReadQuorum}
	if qcfg.W == 0 {
		qcfg.W = w
	}
	if qcfg.R == 0 {
		qcfg.R = r
	}
	replCoord := replication.NewCoordinator(cfg.NodeID, membership, placer, svc, meta, replicator, qcfg, logger)
	hintWorker := replication.NewHintWorker(replCoord, logger)
	healingWorker := replication.NewHealingWorker(replCoord, logger)

	var peerTracker *cluster.PeerHealthTracker
	if membership.Len() > 1 {
		peerTracker = cluster.NewPeerHealthTracker(membership)
		for id, ok := range peerTracker.Snapshot() {
			metrics.SetPeerReachable(id, ok)
		}
	}

	handler := api.NewHandlerWithConfig(svc, api.HandlerConfig{
		PublicURL:            cfg.PublicURL,
		SigV4Region:          cfg.SigV4Region,
		PresignDefaultExpiry: cfg.PresignDefaultExpiry,
		PresignMaxExpiry:     cfg.PresignMaxExpiry,
		LocalNodeID:          cfg.NodeID,
		Membership:           membership,
		Placement:            placer,
		Proxy:                proxy,
		PeerSync:             peerSync,
		Replication:          replCoord,
		InternalToken:        cfg.ClusterInternalToken,
		PeerHealth:           peerTracker,
	}, logger)
	handler.SetReadyCheck(func(ctx context.Context) error {
		if err := meta.DB().PingContext(ctx); err != nil {
			return err
		}
		probe := filepath.Join(cfg.BlobPath, ".health_probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
			return err
		}
		return os.Remove(probe)
	})

	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	handler.Register(mux)

	verifier := &auth.Verifier{
		Region: cfg.SigV4Region,
		Store:  meta.Credentials(),
	}
	authMW := &auth.Middleware{
		Verifier:      verifier,
		Logger:        logger,
		Skip:          auth.DefaultSkip,
		InternalToken: cfg.ClusterInternalToken,
	}

	root := metrics.Middleware(authMW.Handler(mux))

	server := api.NewServer(cfg.HTTPAddress, root, logger)

	cleanup := jobs.NewCleanup(meta, blob, logger)
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	cleanup.StartPeriodic(cleanupCtx, 24*time.Hour)
	go hintWorker.Run(cleanupCtx)
	if membership.Len() > 1 {
		go healingWorker.Run(cleanupCtx)
		peerMonitor := cluster.NewPeerHealthMonitor(membership, peerTracker, cluster.PeerHealthConfig{
			OnRecovered: func(nodeID string) {
				replCoord.OnPeerRecovered(cleanupCtx, nodeID, hintWorker, healingWorker)
			},
		}, logger)
		go peerMonitor.Run(cleanupCtx)
	}

	go syncStorageMetrics(cleanupCtx, meta, logger)

	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("objex started",
		"node_id", cfg.NodeID,
		"http", cfg.HTTPAddress,
		"cluster_nodes", membership.Len(),
		"db", cfg.DBPath,
		"blobs", cfg.BlobPath,
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	cleanupCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
}

func syncStorageMetrics(ctx context.Context, meta *sqlite.Store, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, size, err := meta.AggregateStats(ctx)
			if err != nil {
				logger.Warn("storage metrics sync failed", "error", err)
				continue
			}
			metrics.SetStorageStats(count, size)
		}
	}
}
