package quorum_test

import (
	"testing"

	"github.com/VishalPainjane/objex/internal/quorum"
)

func TestValidateOverlap(t *testing.T) {
	cfg := quorum.Config{N: 3, W: 2, R: 2}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsNoOverlap(t *testing.T) {
	cfg := quorum.Config{N: 3, W: 1, R: 1}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for W+R <= N")
	}
}

func TestValidateRejectsWTooLarge(t *testing.T) {
	cfg := quorum.Config{N: 3, W: 4, R: 2}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteReadSatisfied(t *testing.T) {
	cfg := quorum.Config{N: 3, W: 2, R: 2}
	if !cfg.WriteSatisfied(2) || cfg.WriteSatisfied(1) {
		t.Fatal("write quorum")
	}
	if !cfg.ReadSatisfied(2) || cfg.ReadSatisfied(1) {
		t.Fatal("read quorum")
	}
}
