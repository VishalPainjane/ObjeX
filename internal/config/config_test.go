package config

import (
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("OBJEX_DATA_DIR", "")
	t.Setenv("OBJEX_DB_PATH", "")
	t.Setenv("OBJEX_HTTP_ADDRESS", "")
	t.Setenv("OBJEX_MAX_UPLOAD_BYTES", "")
	t.Setenv("OBJEX_MIN_FREE_DISK_BYTES", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.HTTPAddress != ":9000" {
		t.Errorf("HTTPAddress = %q, want :9000", cfg.HTTPAddress)
	}
	if cfg.DataDir != filepath.Clean("./data") {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
	wantDB := filepath.Join(cfg.DataDir, "db", "objex.db")
	if cfg.DBPath != wantDB {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, wantDB)
	}
	if cfg.BlobPath != filepath.Join(cfg.DataDir, "blobs") {
		t.Errorf("BlobPath = %q", cfg.BlobPath)
	}
	if cfg.MaxUploadBytes != 5*1024*1024*1024 {
		t.Errorf("MaxUploadBytes = %d", cfg.MaxUploadBytes)
	}
	if cfg.MinFreeDiskBytes != 500*1024*1024 {
		t.Errorf("MinFreeDiskBytes = %d", cfg.MinFreeDiskBytes)
	}
}

func TestLoadFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OBJEX_HTTP_ADDRESS", ":8080")
	t.Setenv("OBJEX_DATA_DIR", dir)
	t.Setenv("OBJEX_DB_PATH", filepath.Join(dir, "custom.db"))
	t.Setenv("OBJEX_MAX_UPLOAD_BYTES", "1048576")
	t.Setenv("OBJEX_MIN_FREE_DISK_BYTES", "1024")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.HTTPAddress != ":8080" {
		t.Errorf("HTTPAddress = %q", cfg.HTTPAddress)
	}
	if cfg.DBPath != filepath.Join(dir, "custom.db") {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.MaxUploadBytes != 1048576 {
		t.Errorf("MaxUploadBytes = %d", cfg.MaxUploadBytes)
	}
	if cfg.MinFreeDiskBytes != 1024 {
		t.Errorf("MinFreeDiskBytes = %d", cfg.MinFreeDiskBytes)
	}
}

func TestAbsPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OBJEX_DATA_DIR", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg, err = cfg.AbsPaths()
	if err != nil {
		t.Fatalf("AbsPaths: %v", err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != absDir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, absDir)
	}
	if !filepath.IsAbs(cfg.BlobPath) {
		t.Errorf("BlobPath not absolute: %q", cfg.BlobPath)
	}
}

func TestLoadInvalidMaxUpload(t *testing.T) {
	t.Setenv("OBJEX_MAX_UPLOAD_BYTES", "not-a-number")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid OBJEX_MAX_UPLOAD_BYTES")
	}
}

func TestLoadPresignExpiryFromEnv(t *testing.T) {
	t.Setenv("OBJEX_PRESIGN_DEFAULT_EXPIRY", "7200")
	t.Setenv("OBJEX_PRESIGN_MAX_EXPIRY", "86400")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PresignDefaultExpiry != 7200 {
		t.Errorf("PresignDefaultExpiry = %d", cfg.PresignDefaultExpiry)
	}
	if cfg.PresignMaxExpiry != 86400 {
		t.Errorf("PresignMaxExpiry = %d", cfg.PresignMaxExpiry)
	}
}

func TestInvalidWriteQuorumRejected(t *testing.T) {
	t.Setenv("OBJEX_REPLICATION_FACTOR", "3")
	t.Setenv("OBJEX_WRITE_QUORUM", "0")
	t.Setenv("OBJEX_READ_QUORUM", "2")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for W=0")
	}
}

func TestInvalidReadQuorumRejected(t *testing.T) {
	t.Setenv("OBJEX_REPLICATION_FACTOR", "3")
	t.Setenv("OBJEX_WRITE_QUORUM", "2")
	t.Setenv("OBJEX_READ_QUORUM", "4")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for R>N")
	}
}

func TestQuorumOverlapRejected(t *testing.T) {
	t.Setenv("OBJEX_REPLICATION_FACTOR", "3")
	t.Setenv("OBJEX_WRITE_QUORUM", "1")
	t.Setenv("OBJEX_READ_QUORUM", "1")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for W+R<=N")
	}
}

func TestValidQuorumDefaults(t *testing.T) {
	t.Setenv("OBJEX_REPLICATION_FACTOR", "3")
	t.Setenv("OBJEX_WRITE_QUORUM", "")
	t.Setenv("OBJEX_READ_QUORUM", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WriteQuorum != 2 || cfg.ReadQuorum != 2 {
		t.Fatalf("defaults: W=%d R=%d", cfg.WriteQuorum, cfg.ReadQuorum)
	}
}
