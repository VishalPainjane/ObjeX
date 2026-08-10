package validation

import "testing"

func TestBucketNameError(t *testing.T) {
	valid := []string{"abc", "my-bucket", "a1b2c3", "photos"}
	for _, name := range valid {
		if err := BucketNameError(name); err != "" {
			t.Errorf("BucketNameError(%q) = %q, want valid", name, err)
		}
	}

	invalid := map[string]string{
		"":           "empty",
		"ab":         "short",
		"MyBucket":   "uppercase",
		"-bucket":    "leading hyphen",
		"bucket-":    "trailing hyphen",
		"bu..cket":   "double dot",
	}
	for name := range invalid {
		if err := BucketNameError(name); err == "" {
			t.Errorf("BucketNameError(%q) expected error", name)
		}
	}
}

func TestKeyError(t *testing.T) {
	if KeyError("photos/2024/trip.jpg") != "" {
		t.Error("expected valid key")
	}
	if KeyError("") == "" {
		t.Error("empty key should fail")
	}
	if KeyError("/leading") == "" {
		t.Error("leading slash should fail")
	}
	if KeyError("..") == "" {
		t.Error(".. key should fail after normalization")
	}
	if KeyError("key\x00bad") == "" {
		t.Error("control char should fail")
	}
}

func TestSanitizeKey(t *testing.T) {
	if got := SanitizeKey("path\\to\\file"); got != "path/to/file" {
		t.Errorf("SanitizeKey = %q", got)
	}
	if got := SanitizeKey(".."); got != "" {
		t.Errorf("SanitizeKey(..) = %q, want empty", got)
	}
}
