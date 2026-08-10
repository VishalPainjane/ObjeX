package api_test

import (
	"io"
	"net/http"
	"testing"
)

func TestHealthWrongMethod(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/health/live", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /health/live = %d, want 405", resp.StatusCode)
	}
}

func TestHealthPathNotBucket(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()

	// GET /health/ready must not be treated as bucket=health key=ready
	resp, err := srv.Client().Get(srv.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health/ready = %d, want 200", resp.StatusCode)
	}
}

func TestEncodedObjectKey(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	key := "photos/2024/trip.jpg"
	putResp, err := signedPut(client, srv.URL+"/test-bucket/"+key, []byte("snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put status = %d", putResp.StatusCode)
	}

	getResp, err := signedGet(client, srv.URL+"/test-bucket/"+key)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	body, _ := io.ReadAll(getResp.Body)
	if string(body) != "snapshot" {
		t.Fatalf("body = %q", body)
	}
}
