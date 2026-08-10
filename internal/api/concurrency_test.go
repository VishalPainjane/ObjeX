package api_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
)

// Concurrent PUTs to different keys must all succeed without corruption.
func TestConcurrentPutDifferentKeys(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			body := []byte(fmt.Sprintf("payload-%d", i))
			resp, err := signedPut(client, srv.URL+"/test-bucket/concurrent-"+fmt.Sprint(i)+".txt", body)
			if err != nil || resp.StatusCode != http.StatusOK {
				t.Errorf("put %d failed: %v status=%d", i, err, resp.StatusCode)
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		want := []byte(fmt.Sprintf("payload-%d", i))
		resp, err := signedGet(client, srv.URL+"/test-bucket/concurrent-"+fmt.Sprint(i)+".txt")
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !bytes.Equal(got, want) {
			t.Fatalf("key %d: got %q want %q", i, got, want)
		}
	}
}

// Concurrent PUTs to the same key: final object must be a complete write (no partial reads).
func TestConcurrentPutSameKey(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			body := bytes.Repeat([]byte{byte('a' + (i % 26))}, 64)
			_, _ = signedPut(client, srv.URL+"/test-bucket/same-key.bin", body)
		}(i)
	}
	wg.Wait()

	resp, err := signedGet(client, srv.URL+"/test-bucket/same-key.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(got) != 64 {
		t.Fatalf("expected complete 64-byte object, got len=%d", len(got))
	}
}
