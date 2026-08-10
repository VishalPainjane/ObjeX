package auth_test

import (
	"net/http"
	"testing"

	"github.com/VishalPainjane/objex/internal/auth"
)

func BenchmarkCanonicalRequest(b *testing.B) {
	ctx := auth.RequestContext{
		Method: "GET",
		Path:   "/bucket/key",
		Host:   "localhost:9000",
		Headers: http.Header{
			"Host":                 []string{"localhost:9000"},
			"X-Amz-Date":           []string{"20240101T120000Z"},
			"X-Amz-Content-Sha256": []string{"UNSIGNED-PAYLOAD"},
		},
	}
	parsed := auth.ParsedSig{
		Date:          "20240101",
		Region:        "us-east-1",
		Service:       "s3",
		SignedHeaders: []string{"host", "x-amz-content-sha256", "x-amz-date"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.BuildCanonicalRequest(ctx, parsed)
	}
}

func BenchmarkVerifySignature(b *testing.B) {
	secret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	req, _ := http.NewRequest(http.MethodGet, "http://localhost:9000/", nil)
	req.Host = "localhost:9000"
	_ = auth.SignRequest(req, "AKIAIOSFODNN7EXAMPLE", secret, nil)
	parsed, _ := auth.ParseSigV4(req)
	ctx := auth.RequestContextFromHTTP(req)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.VerifySignature(secret, *parsed, ctx)
	}
}
