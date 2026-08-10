package api_test

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VishalPainjane/objex/internal/auth"
	"github.com/VishalPainjane/objex/internal/cluster"
)

func threeNodeCluster() []cluster.Node {
	return []cluster.Node{
		{ID: "node-1", Address: "", Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: "", Status: cluster.NodeStatusActive},
		{ID: "node-3", Address: "", Status: cluster.NodeStatusActive},
	}
}

type replCluster struct {
	servers map[string]*httptest.Server
	nodes   []cluster.Node
}

func startReplicationCluster(t *testing.T) *replCluster {
	t.Helper()
	nodes := []cluster.Node{
		{ID: "node-1", Status: cluster.NodeStatusActive},
		{ID: "node-2", Status: cluster.NodeStatusActive},
		{ID: "node-3", Status: cluster.NodeStatusActive},
	}
	servers := make(map[string]*httptest.Server, 3)

	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()
	url3, ln3 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = url2
	nodes[2].Address = url3

	servers["node-1"] = startClusterServerOnListenerRF(t, ln1, "node-1", nodes, 3)
	servers["node-2"] = startClusterServerOnListenerRF(t, ln2, "node-2", nodes, 3)
	servers["node-3"] = startClusterServerOnListenerRF(t, ln3, "node-3", nodes, 3)

	return &replCluster{servers: servers, nodes: nodes}
}

func (c *replCluster) URL(id string) string {
	return c.servers[id].URL
}

func (c *replCluster) Client(id string) *http.Client {
	return c.servers[id].Client()
}

func ensureBucketOn(client *http.Client, baseURL, bucket string) {
	req, _ := http.NewRequest(http.MethodPut, baseURL+"/"+bucket, nil)
	resp, err := signAndDo(client, req, nil)
	if err != nil {
		panic(err)
	}
	resp.Body.Close()
}

func objectExists(t *testing.T, client *http.Client, baseURL, bucket, key string) bool {
	t.Helper()
	resp, err := signedHead(client, baseURL+"/"+bucket+"/"+key)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func signedHead(client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return nil, err
	}
	return signAndDo(client, req, nil)
}

func internalReplicaGet(client *http.Client, baseURL, bucket, key string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/"+bucket+"/"+key, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(auth.InternalTokenHeader, testClusterInternalToken)
	req.Header.Set(auth.InternalOperationHeader, auth.OpReplicateGet)
	return client.Do(req)
}

func TestReplicationBasicPut(t *testing.T) {
	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "repl-bucket"
	key := "hello.txt"
	content := []byte("replicated-object")

	ensureBucketOn(client, c.URL("node-1"), bucket)

	resp, err := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, content)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("put status %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	for _, id := range []string{"node-1", "node-2", "node-3"} {
		if !objectExists(t, c.Client(id), c.URL(id), bucket, key) {
			t.Fatalf("object missing on %s", id)
		}
		got, err := signedGet(c.Client(id), c.URL(id)+"/"+bucket+"/"+key)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(got.Body)
		got.Body.Close()
		if !bytes.Equal(body, content) {
			t.Fatalf("node %s content mismatch", id)
		}
	}
}

func TestReplicationNonPrimaryPut(t *testing.T) {
	c := startReplicationCluster(t)
	mem, _ := cluster.NewStaticMembership("node-1", c.nodes)
	p := cluster.NewRendezvousPlacer(mem, 3, nil)
	key := findKeyPlacedOn(t, c.nodes, "fwd-bucket", "node-1")

	ensureBucketOn(c.Client("node-2"), c.URL("node-2"), "fwd-bucket")
	content := []byte("forwarded-and-replicated")
	resp, err := signedPut(c.Client("node-2"), c.URL("node-2")+"/fwd-bucket/"+key, content)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("put status %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	placement, _ := p.Locate("fwd-bucket", key)
	if placement.Primary.ID != "node-1" {
		t.Fatalf("expected node-1 primary, got %s", placement.Primary.ID)
	}
	for _, id := range []string{"node-1", "node-2", "node-3"} {
		if !objectExists(t, c.Client(id), c.URL(id), "fwd-bucket", key) {
			t.Fatalf("missing on %s", id)
		}
	}
}

func TestReplicationLargeObject(t *testing.T) {
	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "large-bucket"
	key := "big.bin"
	size := 5 * 1024 * 1024
	data := bytes.Repeat([]byte{0xAB}, size)
	sum := md5.Sum(data)
	expectedETag := hex.EncodeToString(sum[:])

	ensureBucketOn(client, c.URL("node-1"), bucket)
	resp, err := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, data)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	for _, id := range []string{"node-1", "node-2", "node-3"} {
		got, err := signedGet(c.Client(id), c.URL(id)+"/"+bucket+"/"+key)
		if err != nil {
			t.Fatal(err)
		}
		hasher := md5.New()
		n, err := io.Copy(hasher, got.Body)
		got.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if n != int64(size) {
			t.Fatalf("node %s size %d", id, n)
		}
		if hex.EncodeToString(hasher.Sum(nil)) != expectedETag {
			t.Fatalf("node %s checksum mismatch", id)
		}
	}
}

func TestReplicationOverwrite(t *testing.T) {
	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "ow-bucket"
	key := "file.txt"

	ensureBucketOn(client, c.URL("node-1"), bucket)
	for i, content := range [][]byte{[]byte("version-1"), []byte("version-2")} {
		resp, err := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, content)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("put %d status %d", i+1, resp.StatusCode)
		}
	}

	want := []byte("version-2")
	for _, id := range []string{"node-1", "node-2", "node-3"} {
		got, _ := signedGet(c.Client(id), c.URL(id)+"/"+bucket+"/"+key)
		body, _ := io.ReadAll(got.Body)
		got.Body.Close()
		if !bytes.Equal(body, want) {
			t.Fatalf("node %s has %q", id, body)
		}
	}
}

func TestReplicationStaleWriteRejected(t *testing.T) {
	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "stale-bucket"
	key := "obj"

	ensureBucketOn(client, c.URL("node-1"), bucket)
	resp, err := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, []byte("v2"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Simulate delayed v1 replica write to node-2 (version 1 < stored version 2).
	req, _ := http.NewRequest(http.MethodPut, c.URL("node-2")+"/"+bucket+"/"+key, bytes.NewReader([]byte("v1")))
	req.Header.Set(auth.InternalTokenHeader, testClusterInternalToken)
	req.Header.Set(auth.InternalOperationHeader, auth.OpReplicatePut)
	req.Header.Set(auth.InternalObjectVersion, "1")
	req.Header.Set(auth.InternalExpectedETag, md5hex([]byte("v1")))
	staleResp, err := c.Client("node-2").Do(req)
	if err != nil {
		t.Fatal(err)
	}
	staleResp.Body.Close()
	if staleResp.StatusCode != http.StatusConflict {
		t.Fatalf("stale write status = %d", staleResp.StatusCode)
	}

	got, _ := signedGet(c.Client("node-2"), c.URL("node-2")+"/"+bucket+"/"+key)
	body, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if string(body) != "v2" {
		t.Fatalf("stale write overwrote object: %q", body)
	}
}

func TestReplicationIdempotentReplicaPut(t *testing.T) {
	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "idem-bucket"
	key := "obj"
	content := []byte("same")

	ensureBucketOn(client, c.URL("node-1"), bucket)
	resp, err := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, content)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	etag := md5hex(content)
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodPut, c.URL("node-3")+"/"+bucket+"/"+key, bytes.NewReader(content))
		req.Header.Set(auth.InternalTokenHeader, testClusterInternalToken)
		req.Header.Set(auth.InternalOperationHeader, auth.OpReplicatePut)
		req.Header.Set(auth.InternalObjectVersion, "1")
		req.Header.Set(auth.InternalExpectedETag, etag)
		r, err := c.Client("node-3").Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("attempt %d status %d", i+1, r.StatusCode)
		}
	}
}

func TestReplicationDelete(t *testing.T) {
	c := startReplicationCluster(t)
	client := c.Client("node-1")
	bucket := "del-bucket"
	key := "gone.txt"

	ensureBucketOn(client, c.URL("node-1"), bucket)
	resp, _ := signedPut(client, c.URL("node-1")+"/"+bucket+"/"+key, []byte("bye"))
	resp.Body.Close()

	del, err := signedDelete(client, c.URL("node-2")+"/"+bucket+"/"+key)
	if err != nil {
		t.Fatal(err)
	}
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d", del.StatusCode)
	}

	for _, id := range []string{"node-1", "node-2", "node-3"} {
		if objectExists(t, c.Client(id), c.URL(id), bucket, key) {
			t.Fatalf("object still exists on %s", id)
		}
	}
}

func TestReplicationPartialFailure(t *testing.T) {
	nodes := threeNodeCluster()
	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()
	nodes[0].Address = url1
	nodes[1].Address = url2
	nodes[2].Address = "http://127.0.0.1:1" // unreachable

	_, root1 := buildClusterHandlerRF(t, "node-1", nodes, 3)
	srv1 := httptest.NewUnstartedServer(root1)
	srv1.Listener = ln1
	srv1.Start()
	t.Cleanup(srv1.Close)

	_, root2 := buildClusterHandlerRF(t, "node-2", nodes, 3)
	srv2 := httptest.NewUnstartedServer(root2)
	srv2.Listener = ln2
	srv2.Start()
	t.Cleanup(srv2.Close)

	client := srv1.Client()
	bucket := "partial-bucket"
	key := findKeyPlacedOn(t, nodes, bucket, "node-1")
	ensureBucketOn(client, srv1.URL, bucket)

	resp, err := signedPut(client, srv1.URL+"/"+bucket+"/"+key, []byte("partial"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected quorum write success with W=2, got status %d", resp.StatusCode)
	}

	if !objectExists(t, srv1.Client(), srv1.URL, bucket, key) {
		t.Fatal("primary should have object after partial replication")
	}
	if !objectExists(t, srv2.Client(), srv2.URL, bucket, key) {
		t.Fatal("replica-1 should have object")
	}
}

func md5hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}

func findKeyForAllNodes(t *testing.T, nodes []cluster.Node, bucket string) string {
	mem, _ := cluster.NewStaticMembership("node-1", nodes)
	p := cluster.NewRendezvousPlacer(mem, 3, nil)
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("k-%d", i)
		r, err := p.Locate(bucket, key)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.ReplicaSet()) == 3 {
			return key
		}
	}
	t.Fatal("no suitable key")
	return ""
}
