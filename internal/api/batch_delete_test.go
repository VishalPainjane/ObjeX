package api_test

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBatchDeleteMultipleKeys(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()

	if resp, err := signedPut(client, srv.URL+"/test-bucket", nil); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create bucket: %v status %d", err, resp.StatusCode)
	}

	id := strings.ReplaceAll(t.Name(), "/", "-")
	keys := []string{
		"batch-" + id + "-a.txt",
		"batch-" + id + "-b.txt",
		"batch-" + id + "-c.txt",
	}
	for _, key := range keys {
		content := []byte("content-" + key)
		resp, err := signedPut(client, srv.URL+"/test-bucket/"+key, content)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("put %s: %d", key, resp.StatusCode)
		}
	}

	var xmlBody strings.Builder
	xmlBody.WriteString("<Delete>")
	for _, key := range keys {
		xmlBody.WriteString("<Object><Key>")
		xmlBody.WriteString(key)
		xmlBody.WriteString("</Key></Object>")
	}
	xmlBody.WriteString("</Delete>")
	payload := []byte(xmlBody.String())

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/test-bucket?delete", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/xml")
	resp, err := signAndDo(client, req, payload)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("batch delete: %d %s", resp.StatusCode, body)
	}
	responseXML := string(body)
	if !strings.Contains(responseXML, "DeleteResult") {
		t.Fatalf("missing DeleteResult: %s", responseXML)
	}
	for _, key := range keys {
		if !strings.Contains(responseXML, key) {
			t.Fatalf("response missing key %s: %s", key, responseXML)
		}
		head, err := signedHead(client, srv.URL+"/test-bucket/"+key)
		if err != nil {
			t.Fatal(err)
		}
		head.Body.Close()
		if head.StatusCode != http.StatusNotFound {
			t.Fatalf("head %s after delete: %d", key, head.StatusCode)
		}
	}
}

func TestBatchDeleteMixExistingAndMissing(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()

	if resp, err := signedPut(client, srv.URL+"/test-bucket", nil); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create bucket: %v status %d", err, resp.StatusCode)
	}

	existing := "batch-mix-existing.txt"
	ghost := "batch-mix-ghost.txt"
	put, err := signedPut(client, srv.URL+"/test-bucket/"+existing, []byte("exists"))
	if err != nil {
		t.Fatal(err)
	}
	put.Body.Close()

	payload := []byte(`<Delete><Object><Key>` + existing + `</Key></Object><Object><Key>` + ghost + `</Key></Object></Delete>`)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/test-bucket?delete", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/xml")
	resp, err := signAndDo(client, req, payload)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("batch delete: %d", resp.StatusCode)
	}
}
