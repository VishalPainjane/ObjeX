package api_test

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestMultipartUploadHTTP(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	key := "multipart-test.bin"
	initResp, err := signedPost(client, srv.URL+"/test-bucket/"+key+"?uploads", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if initResp.StatusCode != http.StatusOK {
		t.Fatalf("initiate: %d", initResp.StatusCode)
	}
	body, _ := io.ReadAll(initResp.Body)
	initResp.Body.Close()
	uploadID := extractXMLTag(body, "UploadId")
	if uploadID == "" {
		t.Fatalf("no UploadId in %s", string(body))
	}

	part1 := bytes.Repeat([]byte("a"), 100)
	putPart1, _ := http.NewRequest(http.MethodPut, srv.URL+"/test-bucket/"+key+"?partNumber=1&uploadId="+uploadID, bytes.NewReader(part1))
	putResp1, err := signAndDo(client, putPart1, part1)
	if err != nil {
		t.Fatal(err)
	}
	if putResp1.StatusCode != http.StatusOK {
		t.Fatalf("upload part 1: %d", putResp1.StatusCode)
	}
	etag1 := strings.Trim(putResp1.Header.Get("ETag"), "\"")

	completeXML := `<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUpload>
  <Part><PartNumber>1</PartNumber><ETag>"` + etag1 + `"</ETag></Part>
</CompleteMultipartUpload>`
	completeResp, err := signedPost(client, srv.URL+"/test-bucket/"+key+"?uploadId="+uploadID, strings.NewReader(completeXML), []byte(completeXML))
	if err != nil {
		t.Fatal(err)
	}
	if completeResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(completeResp.Body)
		t.Fatalf("complete: %d body=%s", completeResp.StatusCode, b)
	}
	completeResp.Body.Close()

	getResp, err := signedGet(client, srv.URL+"/test-bucket/"+key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if !bytes.Equal(got, part1) {
		t.Fatalf("body len=%d want %d", len(got), len(part1))
	}
}

func TestCopyObjectHTTP(t *testing.T) {
	srv := newTestHTTP(t)
	defer srv.Close()
	client := srv.Client()
	ensureBucket(client, srv.URL)

	src := []byte("copy-me")
	_, _ = signedPut(client, srv.URL+"/test-bucket/source.txt", src)

	copyReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/test-bucket/dest.txt", nil)
	copyReq.Header.Set("x-amz-copy-source", "/test-bucket/source.txt")
	copyResp, err := signAndDo(client, copyReq, nil)
	if err != nil {
		t.Fatal(err)
	}
	if copyResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(copyResp.Body)
		t.Fatalf("copy: %d %s", copyResp.StatusCode, b)
	}
	copyResp.Body.Close()

	getResp, err := signedGet(client, srv.URL+"/test-bucket/dest.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if !bytes.Equal(got, src) {
		t.Fatalf("copy dest = %q", got)
	}
}

func extractXMLTag(xml []byte, tag string) string {
	re := regexp.MustCompile(`<` + tag + `>([^<]+)</` + tag + `>`)
	m := re.FindSubmatch(xml)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}
