package api_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VishalPainjane/objex/internal/cluster"
)

func TestForwardPutToPrimaryNode(t *testing.T) {
	url1, ln1 := reserveListener()
	url2, ln2 := reserveListener()

	nodes := []cluster.Node{
		{ID: "node-1", Address: url1, Status: cluster.NodeStatusActive},
		{ID: "node-2", Address: url2, Status: cluster.NodeStatusActive},
	}

	srv1 := startClusterServerOnListener(t, ln1, "node-1", nodes)
	srv2 := startClusterServerOnListener(t, ln2, "node-2", nodes)

	key := findKeyPlacedOn(t, nodes, "test-bucket", "node-1")

	client := srv2.Client()
	ensureBucket(client, srv2.URL)

	content := []byte("forwarded-to-primary")
	resp, err := signedPut(client, srv2.URL+"/test-bucket/"+key, content)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("put status %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	getResp, err := signedGet(client, srv1.URL+"/test-bucket/"+key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("primary get: %q", got)
	}
}

func findKeyPlacedOn(t *testing.T, nodes []cluster.Node, bucket, nodeID string) string {
	mem, err := cluster.NewStaticMembership(nodeID, nodes)
	if err != nil {
		t.Fatal(err)
	}
	p := cluster.NewRendezvousPlacer(mem, 1, nil)
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("obj-%d", i)
		r, err := p.Locate(bucket, key)
		if err != nil {
			t.Fatal(err)
		}
		if r.Primary.ID == nodeID {
			return key
		}
	}
	t.Fatalf("no key found for primary %s", nodeID)
	return ""
}

func reserveListener() (string, net.Listener) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	return "http://" + ln.Addr().String(), ln
}

func startClusterServerOnListener(t *testing.T, ln net.Listener, localID string, nodes []cluster.Node) *httptest.Server {
	return startClusterServerOnListenerRF(t, ln, localID, nodes, 1)
}

func startClusterServerOnListenerRF(t *testing.T, ln net.Listener, localID string, nodes []cluster.Node, rf int) *httptest.Server {
	_, root := buildClusterHandlerRF(t, localID, nodes, rf)
	srv := httptest.NewUnstartedServer(root)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}
