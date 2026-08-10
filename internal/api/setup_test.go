package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VishalPainjane/objex/internal/api"
	"github.com/VishalPainjane/objex/internal/cluster"
)

const testClusterInternalToken = "test-cluster-internal-token"

func newTestHTTP(t *testing.T) *httptest.Server {
	return newTestClusterHTTP(t, "node-1", []cluster.Node{
		{ID: "node-1", Address: "localhost:9000", Status: cluster.NodeStatusActive},
	})
}

func newTestClusterHTTP(t *testing.T, localID string, nodes []cluster.Node) *httptest.Server {
	h, root := buildClusterHandler(t, localID, nodes)
	srv := httptest.NewUnstartedServer(root)
	srv.Start()
	t.Cleanup(srv.Close)
	h.SetPublicURL(srv.URL)
	return srv
}

func buildClusterHandler(t *testing.T, localID string, nodes []cluster.Node) (*api.Handler, http.Handler) {
	return buildClusterHandlerRF(t, localID, nodes, 1)
}

func srvClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}
