package api_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/VishalPainjane/objex/internal/auth"
)

func TestObjectLifecycleHTTP(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()

	resp, err := signedPut(client, srv.URL+"/test-bucket", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create bucket: %d", resp.StatusCode)
	}

	content := []byte("Hello, ObjeX round-trip test!")
	putResp, err := signedPut(client, srv.URL+"/test-bucket/lifecycle-test.txt", content)
	if err != nil {
		t.Fatal(err)
	}
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put: %d", putResp.StatusCode)
	}

	getResp, err := signedGet(client, srv.URL+"/test-bucket/lifecycle-test.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if !bytes.Equal(body, content) {
		t.Fatal("get body mismatch")
	}

	delResp, err := signedDelete(client, srv.URL+"/test-bucket/lifecycle-test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", delResp.StatusCode)
	}

	notFound, err := signedGet(client, srv.URL+"/test-bucket/lifecycle-test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if notFound.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", notFound.StatusCode)
	}
}

func ensureBucket(client *http.Client, url string) {
	resp, err := signedPut(client, url+"/test-bucket", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		panic("create test bucket failed")
	}
}

func TestRangeRequestHTTP(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	_, _ = signedPut(client, srv.URL+"/test-bucket/range.txt", []byte("Hello, Range Request!"))

	getReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/test-bucket/range.txt", nil)
	getReq.Header.Set("Range", "bytes=0-4")
	getResp, err := signAndDo(client, getReq, nil)
	if err != nil {
		t.Fatal(err)
	}
	if getResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d", getResp.StatusCode)
	}
	body, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if string(body) != "Hello" {
		t.Errorf("range body = %q", string(body))
	}
}

func TestCustomMetadataHTTP(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	putReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/test-bucket/meta.txt", bytes.NewReader([]byte("x")))
	putReq.Header.Set("x-amz-meta-custom-tag", "my-value")
	_, _ = signAndDo(client, putReq, []byte("x"))

	headReq, _ := http.NewRequest(http.MethodHead, srv.URL+"/test-bucket/meta.txt", nil)
	headResp, err := signAndDo(client, headReq, nil)
	if err != nil {
		t.Fatal(err)
	}
	if headResp.Header.Get("x-amz-meta-custom-tag") != "my-value" {
		t.Errorf("meta header = %q", headResp.Header.Get("x-amz-meta-custom-tag"))
	}
}

func TestDeleteNonexistentHTTP(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	ensureBucket(srv.Client(), srv.URL)
	resp, err := signedDelete(srv.Client(), srv.URL+"/test-bucket/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHealthEndpoints(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	for _, path := range []string{"/health", "/health/live", "/health/ready"} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", path, resp.StatusCode)
		}
	}
}

func TestSigV4NoCredentials(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(body, []byte("AccessDenied")) {
		t.Fatalf("body = %s", body)
	}
}

func TestSigV4InvalidAccessKey(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	if err := auth.SignRequest(req, "OBXBADKEY9999999999", testSecretKey, nil); err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestSigV4WrongSignature(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	if err := auth.SignRequest(req, testAccessKeyID, "WrongSecretKeyThatDoesNotMatchAtAll12345678", nil); err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestSigV4ExpiredTimestamp(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	if err := auth.SignRequestWithTime(req, testAccessKeyID, testSecretKey, time.Now().UTC().Add(-20*time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestSigV4ValidListBuckets(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	resp, err := signedGet(srv.Client(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(body, []byte("ListAllMyBucketsResult")) {
		t.Fatalf("body = %s", body)
	}
}

func TestPresignedURLHTTP(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	content := []byte("presigned content")
	_, _ = signedPut(client, srv.URL+"/test-bucket/presigned-test.txt", content)

	presignResp, err := signedGet(client, srv.URL+"/api/presign/test-bucket/presigned-test.txt?expires=3600")
	if err != nil {
		t.Fatal(err)
	}
	if presignResp.StatusCode != http.StatusOK {
		t.Fatalf("presign status = %d", presignResp.StatusCode)
	}
	presignBody, _ := io.ReadAll(presignResp.Body)
	presignResp.Body.Close()

	url := extractJSONField(presignBody, "url")
	if url == "" {
		t.Fatalf("no url in %s", presignBody)
	}

	getResp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("presigned get status = %d", getResp.StatusCode)
	}
	got, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q want %q", got, content)
	}
}

func extractJSONField(json []byte, field string) string {
	// minimal: {"url":"..."}
	prefix := `"` + field + `":"`
	start := bytes.Index(json, []byte(prefix))
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := bytes.IndexByte(json[start:], '"')
	if end < 0 {
		return ""
	}
	return string(json[start : start+end])
}
