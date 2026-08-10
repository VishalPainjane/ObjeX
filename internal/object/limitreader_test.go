package object

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestLimitReaderAllowsExactLimit(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 100)
	lim := newLimitReader(bytes.NewReader(data), 100)
	out, err := io.ReadAll(lim)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(out) != 100 {
		t.Fatalf("len = %d, want 100", len(out))
	}
}

func TestLimitReaderRejectsOverflow(t *testing.T) {
	data := bytes.Repeat([]byte("b"), 101)
	lim := newLimitReader(bytes.NewReader(data), 100)
	_, err := io.ReadAll(lim)
	if !errors.Is(err, errUploadTooLarge) {
		t.Fatalf("expected upload too large, got %v", err)
	}
}
