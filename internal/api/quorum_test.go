package api_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VishalPainjane/objex/internal/cluster"
	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/replication"
)

func startThreeNodeClusterPartial(t *testing.T, deadNodeID string) (srv1, srv2, srv3 *httptest.Server, nodes []cluster.Node) {
	t.Helper()
	nodes = threeNodeCluster()
	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()
	url3, ln3 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = url2
	nodes[2].Address = url3
	if deadNodeID == "node-3" {
		nodes[2].Address = "http://127.0.0.1:1"
	}

	srv1 = httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-1", nodes, 3, "").Root)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	srv2 = httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-2", nodes, 3, "").Root)
	srv2.Listener = ln2
	srv2.Start()
	t.Cleanup(srv2.Close)

	if deadNodeID != "node-3" {
		srv3 = httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-3", nodes, 3, "").Root)
		srv3.Listener = ln3
		srv3.Start()
		t.Cleanup(srv3.Close)
	}
	return srv1, srv2, srv3, nodes
}

func TestStaleReadReturnsNewestAndReadRepair(t *testing.T) {
	replication.SetFaultInjector(nil)
	t.Cleanup(func() { replication.SetFaultInjector(nil) })

	srv1, srv2, srv3, nodes := startThreeNodeClusterPartial(t, "")
	client := srv1.Client()
	bucket := "stale-read"
	key := findKeyForAllNodes(t, nodes, bucket)

	ensureBucketOn(client, srv1.URL, bucket)

	v1 := []byte("version-one")
	resp, err := signedPut(client, srv1.URL+"/"+bucket+"/"+key, v1)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var blockNode3 atomic.Bool
	replication.SetFaultInjector(func(nodeID, op string) error {
		if nodeID == "node-3" && op == "put" && blockNode3.Load() {
			return fmt.Errorf("injected replica failure")
		}
		return nil
	})

	blockNode3.Store(true)
	v2 := []byte("version-two")
	resp, err = signedPut(client, srv1.URL+"/"+bucket+"/"+key, v2)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put v2 status %d", resp.StatusCode)
	}
	blockNode3.Store(false)

	got, err := signedGet(client, srv2.URL+"/"+bucket+"/"+key)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if !bytes.Equal(body, v2) {
		t.Fatalf("read returned %q, want v2", body)
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		r, err := signedGet(srv3.Client(), srv3.URL+"/"+bucket+"/"+key)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if bytes.Equal(b, v2) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("node-3 was not read-repaired to v2")
}

func TestHintPersistsAndDeliversAfterNodeRecovery(t *testing.T) {
	replication.SetFaultInjector(nil)
	t.Cleanup(func() { replication.SetFaultInjector(nil) })

	nodes := threeNodeCluster()
	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()
	url3, ln3 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = url2
	nodes[2].Address = "http://127.0.0.1:1" // node-3 down

	dataDir := t.TempDir()
	env1 := buildClusterTestEnv(t, "node-1", nodes, 3, dataDir)

	srv1 := httptest.NewUnstartedServer(env1.Root)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	srv2 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-2", nodes, 3, "").Root)
	srv2.Listener = ln2
	srv2.Start()
	t.Cleanup(srv2.Close)

	client := srv1.Client()
	bucket := "hint-persist"
	key := findKeyPlacedOn(t, nodes, bucket, "node-1")
	ensureBucketOn(client, srv1.URL, bucket)

	content := []byte("hint-payload")
	resp, err := signedPut(client, srv1.URL+"/"+bucket+"/"+key, content)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put status %d", resp.StatusCode)
	}

	pending, err := env1.Meta.CountPendingHints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending < 1 {
		t.Fatalf("expected pending hint, got %d", pending)
	}

	// Simulate process restart: new handler on same primary data dir.
	env1Restart := buildClusterTestEnv(t, "node-1", nodes, 3, dataDir)

	nodes[2].Address = url3
	srv3 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-3", nodes, 3, "").Root)
	srv3.Listener = ln3
	srv3.Start()
	t.Cleanup(srv3.Close)

	// Update node-2 env addresses for node-3 recovery (membership list must match).
	for i := 0; i < 20; i++ {
		env1Restart.HintWorker.ProcessDueHintsOnce(context.Background())
		if objectExists(t, srv3.Client(), srv3.URL, bucket, key) {
			got, _ := signedGet(srv3.Client(), srv3.URL+"/"+bucket+"/"+key)
			body, _ := io.ReadAll(got.Body)
			got.Body.Close()
			if bytes.Equal(body, content) {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("hint was not delivered after node-3 recovery")
}

func TestHintStagingSurvivesOverwrite(t *testing.T) {
	replication.SetFaultInjector(nil)
	t.Cleanup(func() { replication.SetFaultInjector(nil) })

	nodes := threeNodeCluster()
	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()
	url3, ln3 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = url2
	nodes[2].Address = "http://127.0.0.1:1" // node-3 down

	dataDir := t.TempDir()
	env1 := buildClusterTestEnv(t, "node-1", nodes, 3, dataDir)

	srv1 := httptest.NewUnstartedServer(env1.Root)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	srv2 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-2", nodes, 3, "").Root)
	srv2.Listener = ln2
	srv2.Start()
	t.Cleanup(srv2.Close)

	client := srv1.Client()
	bucket := "hint-overwrite"
	key := findKeyPlacedOn(t, nodes, bucket, "node-1")
	ensureBucketOn(client, srv1.URL, bucket)

	original := []byte("original-version")
	resp, err := signedPut(client, srv1.URL+"/"+bucket+"/"+key, original)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put status %d", resp.StatusCode)
	}

	hints, err := env1.Meta.ListDueHints(context.Background(), time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hints) < 1 {
		t.Fatal("expected hint for node-3")
	}
	var v1Hint metadata.ReplicationHint
	for _, h := range hints {
		if h.Version == 1 {
			v1Hint = h
			break
		}
	}
	if v1Hint.SourcePath == "" {
		t.Fatal("expected staged source_path on v1 hint")
	}

	overwrite := []byte("newer-overwrite")
	resp, err = signedPut(client, srv1.URL+"/"+bucket+"/"+key, overwrite)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overwrite status %d", resp.StatusCode)
	}

	// Drop v2 hints so we only exercise delivery of the pinned v1 payload.
	ctx := context.Background()
	if _, err := env1.Meta.DB().ExecContext(ctx,
		`DELETE FROM replication_hints WHERE bucket_name = ? AND key = ? AND version > ?`,
		bucket, key, v1Hint.Version,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := env1.Meta.DB().ExecContext(ctx,
		`UPDATE replication_hints SET next_attempt_at = ?, attempts = 0, last_error = NULL WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), v1Hint.ID,
	); err != nil {
		t.Fatal(err)
	}

	nodes[2].Address = url3
	env1Restart := buildClusterTestEnv(t, "node-1", nodes, 3, dataDir)
	dataDir3 := t.TempDir()
	env3 := buildClusterTestEnv(t, "node-3", nodes, 3, dataDir3)
	srv3 := httptest.NewUnstartedServer(env3.Root)
	srv3.Listener = ln3
	srv3.Start()
	t.Cleanup(srv3.Close)
	ensureBucketOn(srv3.Client(), srv3.URL, bucket)

	due, err := env1Restart.Meta.ListDueHints(ctx, time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	var deliverHint metadata.ReplicationHint
	for _, h := range due {
		if h.ID == v1Hint.ID {
			deliverHint = h
			break
		}
	}
	if deliverHint.ID == "" {
		t.Fatal("v1 hint missing before delivery")
	}
	if err := env1Restart.Coordinator.DeliverHint(ctx, deliverHint); err != nil {
		t.Fatalf("deliver hint: %v", err)
	}

	got, err := internalReplicaGet(srv3.Client(), srv3.URL, bucket, key)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("local replica status %d", got.StatusCode)
	}
	body, _ := io.ReadAll(got.Body)
	if !bytes.Equal(body, original) {
		t.Fatalf("local replica body = %q, want %q", body, original)
	}
}

func TestHintRetryBackoffRecordsFailure(t *testing.T) {
	replication.SetFaultInjector(nil)
	t.Cleanup(func() { replication.SetFaultInjector(nil) })

	nodes := threeNodeCluster()
	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = url2
	nodes[2].Address = "http://127.0.0.1:1"

	dataDir := t.TempDir()
	env := buildClusterTestEnv(t, "node-1", nodes, 3, dataDir)
	srv1 := httptest.NewUnstartedServer(env.Root)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	srv2 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-2", nodes, 3, "").Root)
	srv2.Listener = ln2
	srv2.Start()
	t.Cleanup(srv2.Close)

	client := srv1.Client()
	bucket := "hint-retry"
	key := findKeyPlacedOn(t, nodes, bucket, "node-1")
	ensureBucketOn(client, srv1.URL, bucket)

	resp, err := signedPut(client, srv1.URL+"/"+bucket+"/"+key, []byte("retry-me"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	pending, err := env.Meta.CountPendingHints(context.Background())
	if err != nil || pending < 1 {
		t.Fatalf("expected pending hint, got %d err=%v", pending, err)
	}

	var failCount atomic.Int32
	replication.SetFaultInjector(func(nodeID, op string) error {
		if nodeID == "node-3" && op == "put" {
			failCount.Add(1)
			return fmt.Errorf("injected failure")
		}
		return nil
	})

	for i := 0; i < 3; i++ {
		env.HintWorker.ProcessDueHintsOnce(context.Background())
	}

	if failCount.Load() < 1 {
		t.Fatalf("expected hint delivery attempts, got %d", failCount.Load())
	}

	still, _ := env.Meta.CountPendingHints(context.Background())
	if still == 0 {
		t.Fatal("hint should remain pending after failures")
	}
}

func TestQuorumReadFailsWithOneReplica(t *testing.T) {
	nodes := threeNodeCluster()
	url1, ln1 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = "http://127.0.0.1:1"
	nodes[2].Address = "http://127.0.0.1:2"

	srv1 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-1", nodes, 3, "").Root)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	client := srv1.Client()
	bucket := "read-fail"
	key := findKeyForAllNodes(t, nodes, bucket)
	ensureBucketOn(client, srv1.URL, bucket)

	put, _ := signedPut(client, srv1.URL+"/"+bucket+"/"+key, []byte("only-primary"))
	put.Body.Close()

	head, err := signedHead(client, srv1.URL+"/"+bucket+"/"+key)
	if err != nil {
		t.Fatal(err)
	}
	head.Body.Close()
	if head.StatusCode == http.StatusOK {
		t.Fatal("expected read quorum failure with only one replica")
	}
}

func TestConcurrentReadWriteNoMixedBytes(t *testing.T) {
	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "concurrent-q"
	key := findKeyForAllNodes(t, c.nodes, bucket)
	ensureBucketOn(client, c.URL("node-1"), bucket)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	errCh := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			payload := []byte(fmt.Sprintf("payload-%d-%d", i, time.Now().UnixNano()))
			resp, err := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, payload)
			if err != nil {
				errCh <- err
				return
			}
			resp.Body.Close()
			i++
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			resp, err := signedGet(client, c.URL("node-2")+"/"+bucket+"/"+key)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if len(body) == 0 {
				continue
			}
			got := string(body)
			var prefix string
			if _, err := fmt.Sscanf(got, "payload-%d-", &prefix); err == nil {
				// valid format
			}
			if len(got) < 8 || got[:8] != "payload-" {
				errCh <- fmt.Errorf("corrupt body: %q", got)
				return
			}
		}
	}()

	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
}

func TestFaultInjectorBlocksReplicaPut(t *testing.T) {
	replication.SetFaultInjector(func(nodeID, op string) error {
		if nodeID == "node-2" && op == "put" {
			return fmt.Errorf("blocked")
		}
		return nil
	})
	t.Cleanup(func() { replication.SetFaultInjector(nil) })

	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "fault-inject"
	key := findKeyForAllNodes(t, c.nodes, bucket)
	ensureBucketOn(client, c.URL("node-1"), bucket)

	resp, err := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// node-1 + node-3 ack, node-2 blocked → W=2 still satisfied
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected success with W=2, got %d", resp.StatusCode)
	}
}

func TestQuorumWritePartialSuccessWithHint(t *testing.T) {
	nodes := threeNodeCluster()
	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = url2
	nodes[2].Address = "http://127.0.0.1:1"

	srv1 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-1", nodes, 3, "").Root)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	srv2 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-2", nodes, 3, "").Root)
	srv2.Listener = ln2
	srv2.Start()
	t.Cleanup(srv2.Close)

	client := srv1.Client()
	bucket := "hint-bucket"
	key := findKeyPlacedOn(t, nodes, bucket, "node-1")
	ensureBucketOn(client, srv1.URL, bucket)

	resp, err := signedPut(client, srv1.URL+"/"+bucket+"/"+key, []byte("hinted"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put status %d", resp.StatusCode)
	}

	time.Sleep(3 * time.Second)
	if !objectExists(t, srv1.Client(), srv1.URL, bucket, key) {
		t.Fatal("missing on primary")
	}
	if !objectExists(t, srv2.Client(), srv2.URL, bucket, key) {
		t.Fatal("missing on replica-1")
	}
}

func TestQuorumReadReturnsNewestVersion(t *testing.T) {
	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "read-quorum"
	key := "obj"
	content := []byte("version-eight")

	ensureBucketOn(client, c.URL("node-1"), bucket)
	resp, _ := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, content)
	resp.Body.Close()

	got, err := signedGet(client, c.URL("node-2")+"/"+bucket+"/"+key)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if !bytes.Equal(body, content) {
		t.Fatalf("content mismatch: %q", body)
	}
}

func TestTombstonePreventsResurrection(t *testing.T) {
	nodes := threeNodeCluster()
	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = url2
	nodes[2].Address = "http://127.0.0.1:1"

	srv1 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-1", nodes, 3, "").Root)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	srv2 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-2", nodes, 3, "").Root)
	srv2.Listener = ln2
	srv2.Start()
	t.Cleanup(srv2.Close)

	client := srv1.Client()
	bucket := "tomb-bucket"
	key := findKeyPlacedOn(t, nodes, bucket, "node-1")
	ensureBucketOn(client, srv1.URL, bucket)

	put, _ := signedPut(client, srv1.URL+"/"+bucket+"/"+key, []byte("alive"))
	put.Body.Close()

	del, err := signedDelete(client, srv2.URL+"/"+bucket+"/"+key)
	if err != nil {
		t.Fatal(err)
	}
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d", del.StatusCode)
	}

	if objectExists(t, srv1.Client(), srv1.URL, bucket, key) {
		t.Fatal("object should be deleted on primary")
	}
	if objectExists(t, srv2.Client(), srv2.URL, bucket, key) {
		t.Fatal("object should be deleted on reachable replica")
	}
}

func TestQuorumWriteFailsWithOneReplica(t *testing.T) {
	nodes := threeNodeCluster()
	url1, ln1 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = "http://127.0.0.1:1"
	nodes[2].Address = "http://127.0.0.1:2"

	srv1 := httptest.NewUnstartedServer(buildClusterTestEnv(t, "node-1", nodes, 3, "").Root)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	client := srv1.Client()
	bucket := "fail-bucket"
	key := findKeyPlacedOn(t, nodes, bucket, "node-1")
	ensureBucketOn(client, srv1.URL, bucket)

	resp, err := signedPut(client, srv1.URL+"/"+bucket+"/"+key, []byte("fail"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected write quorum failure")
	}
}
