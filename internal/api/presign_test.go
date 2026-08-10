package api_test

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/VishalPainjane/objex/internal/auth"
)

func TestPresignedPUTHTTP(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	presignResp, err := signedGet(client, srv.URL+"/api/presign/test-bucket/upload-via-presign.txt?expires=3600&method=PUT")
	if err != nil {
		t.Fatal(err)
	}
	presignBody, _ := io.ReadAll(presignResp.Body)
	presignResp.Body.Close()
	url := extractJSONField(presignBody, "url")
	if url == "" {
		t.Fatal("empty presign url")
	}

	content := []byte("uploaded via presigned put")
	putReq, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	putResp, err := client.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	if putResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(putResp.Body)
		t.Fatalf("put status %d %s", putResp.StatusCode, b)
	}

	getResp, err := signedGet(client, srv.URL+"/test-bucket/upload-via-presign.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q", got)
	}
}

func TestPresignedURLExpired(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)
	_, _ = signedPut(client, srv.URL+"/test-bucket/expired.txt", []byte("x"))

	url, err := auth.GeneratePresignedURL(auth.PresignOptions{
		BaseURL:         srv.URL,
		Bucket:          "test-bucket",
		Key:             "expired.txt",
		AccessKeyID:     testAccessKeyID,
		SecretAccessKey: testSecretKey,
		ExpiresSeconds:  1,
		Region:          "us-east-1",
		Method:          "GET",
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestPresignedURLTamperedSignature(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)
	_, _ = signedPut(client, srv.URL+"/test-bucket/tampered.txt", []byte("secret"))

	url, err := auth.GeneratePresignedURL(auth.PresignOptions{
		BaseURL:         srv.URL,
		Bucket:          "test-bucket",
		Key:             "tampered.txt",
		AccessKeyID:     testAccessKeyID,
		SecretAccessKey: testSecretKey,
		ExpiresSeconds:  3600,
		Region:          "us-east-1",
		Method:          "GET",
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(url, "X-Amz-Signature=", "X-Amz-Signature=00000000", 1)
	resp, err := client.Get(tampered)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered presign status = %d, want 403", resp.StatusCode)
	}
}
