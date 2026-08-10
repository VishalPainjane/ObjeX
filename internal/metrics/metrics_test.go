package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VishalPainjane/objex/internal/metrics"
)

func TestMetricsEndpoint(t *testing.T) {
	handler := metrics.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("status = %d", metricsRec.Code)
	}
	body := metricsRec.Body.String()
	if !strings.Contains(body, "objex_http_requests_total") {
		t.Fatalf("missing objex_http_requests_total in metrics output")
	}
}

func TestClassifyOperationBounded(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/my-bucket/object.txt", nil)
	op := metrics.ClassifyOperation(req)
	if op != "PUT_OBJECT" {
		t.Fatalf("op = %q", op)
	}
}

func TestClassifyOperationNoRawKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/bucket/very/long/key/path", nil)
	op := metrics.ClassifyOperation(req)
	if strings.Contains(op, "very") {
		t.Fatalf("operation label should not contain key: %q", op)
	}
}

func TestRecordUploadDownload(t *testing.T) {
	metrics.SetStorageStats(10, 1024)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "objex_objects_total") {
		t.Fatalf("missing objects gauge")
	}
}
