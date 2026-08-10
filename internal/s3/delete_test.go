package s3_test

import (
	"testing"

	"github.com/VishalPainjane/objex/internal/s3"
)

func TestParseDeleteObjectKeys(t *testing.T) {
	body := []byte(`<Delete>
  <Object><Key>a.txt</Key></Object>
  <Object><Key>b.txt</Key></Object>
</Delete>`)
	keys, err := s3.ParseDeleteObjectKeys(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "a.txt" || keys[1] != "b.txt" {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestParseDeleteObjectKeysBareKeyElements(t *testing.T) {
	body := []byte(`<Delete><Key>only.txt</Key></Delete>`)
	keys, err := s3.ParseDeleteObjectKeys(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "only.txt" {
		t.Fatalf("keys = %#v", keys)
	}
}
