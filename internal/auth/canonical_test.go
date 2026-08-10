package auth

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCanonicalURI(t *testing.T) {
	got := CanonicalURI("/my-bucket/hello.txt")
	want := "/my-bucket/hello.txt"
	if got != want {
		t.Fatalf("CanonicalURI = %q want %q", got, want)
	}
	got2 := CanonicalURI("/bucket/spaced%20key")
	if !strings.Contains(got2, "%20") {
		t.Fatalf("expected encoded space in %q", got2)
	}
}

func TestCanonicalQueryStringOrder(t *testing.T) {
	ctx := RequestContext{
		RawQuery: "b=2&a=1",
		Query:    parseQuery("b=2&a=1"),
	}
	parsed := ParsedSig{SignedHeaders: []string{"host"}}
	got := CanonicalQueryString(ctx, parsed)
	if got != "a=1&b=2" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalQueryPresignedExcludesSignature(t *testing.T) {
	ctx := RequestContext{
		IsPresigned: true,
		Query: url.Values{
			"X-Amz-Algorithm":    []string{"AWS4-HMAC-SHA256"},
			"X-Amz-Credential":     []string{"AKIA/20240101/us-east-1/s3/aws4_request"},
			"X-Amz-Date":           []string{"20240101T000000Z"},
			"X-Amz-Expires":        []string{"3600"},
			"X-Amz-SignedHeaders":  []string{"host"},
			"X-Amz-Signature":      []string{"abc123"},
		},
	}
	parsed := ParsedSig{SignedHeaders: []string{"host"}}
	got := CanonicalQueryString(ctx, parsed)
	if strings.Contains(got, "X-Amz-Signature") {
		t.Fatalf("signature should be excluded: %q", got)
	}
}

func TestBuildStringToSignDeterministic(t *testing.T) {
	ctx := RequestContext{
		Method: "GET",
		Path:   "/",
		Host:   "localhost:9000",
		Headers: http.Header{
			"Host":                []string{"localhost:9000"},
			"X-Amz-Date":          []string{"20240101T120000Z"},
			"X-Amz-Content-Sha256": []string{"UNSIGNED-PAYLOAD"},
		},
	}
	parsed := ParsedSig{
		Date:          "20240101",
		Region:        "us-east-1",
		Service:       "s3",
		SignedHeaders: []string{"host", "x-amz-content-sha256", "x-amz-date"},
	}
	canonical := BuildCanonicalRequest(ctx, parsed)
	sts := BuildStringToSign(ctx, parsed, canonical)
	if !strings.HasPrefix(sts, "AWS4-HMAC-SHA256\n") {
		t.Fatalf("sts = %q", sts)
	}
}

func TestVerifySignatureKnownSecret(t *testing.T) {
	secret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	req, _ := http.NewRequest(http.MethodGet, "http://localhost:9000/", nil)
	req.Host = "localhost:9000"
	req.Header.Set("Host", "localhost:9000")
	req.Header.Set("x-amz-date", "20240101T120000Z")
	req.Header.Set("x-amz-content-sha256", "UNSIGNED-PAYLOAD")
	if err := SignRequest(req, "AKIAIOSFODNN7EXAMPLE", secret, nil); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSigV4(req)
	if err != nil || parsed == nil {
		t.Fatal("parse failed")
	}
	ctx := RequestContextFromHTTP(req)
	valid, _, _ := VerifySignature(secret, *parsed, ctx)
	if !valid {
		t.Fatal("expected valid signature")
	}
}

func TestVerifySignatureWrongSecret(t *testing.T) {
	secret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	req, _ := http.NewRequest(http.MethodGet, "http://localhost:9000/", nil)
	req.Host = "localhost:9000"
	_ = SignRequest(req, "AKIAIOSFODNN7EXAMPLE", secret, nil)
	parsed, _ := ParseSigV4(req)
	ctx := RequestContextFromHTTP(req)
	valid, _, _ := VerifySignature("wrong-secret-key", *parsed, ctx)
	if valid {
		t.Fatal("expected invalid signature")
	}
}

func parseQuery(q string) url.Values {
	v, _ := url.ParseQuery(q)
	return v
}
