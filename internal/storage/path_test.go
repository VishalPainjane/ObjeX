package storage

import (
	"path/filepath"
	"testing"
)

func TestObjectAddressHashDeterministic(t *testing.T) {
	h1 := ObjectAddressHash("photos", "2024/trip.jpg")
	h2 := ObjectAddressHash("photos", "2024/trip.jpg")
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("hash length = %d, want 64", len(h1))
	}
}

func TestBlobRelativePathLayout(t *testing.T) {
	hash := ObjectAddressHash("photos", "2024/trip.jpg")
	rel := BlobRelativePath("photos", "2024/trip.jpg")
	want := filepath.Join("photos", hash[0:2], hash[2:4], hash+".blob")
	if rel != want {
		t.Errorf("BlobRelativePath = %q, want %q", rel, want)
	}
}

func TestAssertWithinBase(t *testing.T) {
	base := t.TempDir()
	inside, err := ResolveBlobPath(base, "bucket", "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AssertWithinBase(inside, base); err != nil {
		t.Fatalf("expected inside path valid: %v", err)
	}

	outside := base + "/../outside.blob"
	if _, err := AssertWithinBase(outside, base); err == nil {
		t.Fatal("expected path escape error")
	}
}
