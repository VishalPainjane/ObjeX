package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/VishalPainjane/objex/internal/quorum"
)

const (
	defaultHTTPAddress      = ":9000"
	defaultDataDir          = "./data"
	defaultMaxUploadBytes   = 5 * 1024 * 1024 * 1024 // 5 GiB
	defaultMinFreeDiskBytes = 500 * 1024 * 1024      // 500 MiB
	defaultPublicURL        = "http://localhost:9000"
	defaultSigV4Region      = "us-east-1"
	defaultPresignExpiry    = 3600
	defaultPresignMaxExpiry = 604800
)

// Config holds runtime configuration for the ObjeX V2 server.
type Config struct {
	HTTPAddress      string
	DataDir          string
	DBPath           string
	BlobPath         string
	MaxUploadBytes   int64
	MinFreeDiskBytes int64
	PublicURL        string
	SigV4Region      string
	AccessKeyID      string
	SecretAccessKey  string
	PresignDefaultExpiry int
	PresignMaxExpiry     int
	NodeID               string
	ClusterNodes         []ClusterNodeConfig
	ClusterInternalToken string
	ReplicationFactor    int
	WriteQuorum          int
	ReadQuorum           int
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (Config, error) {
	dataDir := envOr("OBJEX_DATA_DIR", defaultDataDir)
	dataDir = filepath.Clean(dataDir)

	dbPath := envOr("OBJEX_DB_PATH", filepath.Join(dataDir, "db", "objex.db"))
	blobPath := filepath.Join(dataDir, "blobs")

	cfg := Config{
		HTTPAddress:      envOr("OBJEX_HTTP_ADDRESS", defaultHTTPAddress),
		DataDir:          dataDir,
		DBPath:           dbPath,
		BlobPath:         blobPath,
		MaxUploadBytes:   defaultMaxUploadBytes,
		MinFreeDiskBytes: defaultMinFreeDiskBytes,
		PublicURL:            envOr("OBJEX_PUBLIC_URL", defaultPublicURL),
		SigV4Region:          envOr("OBJEX_SIGV4_REGION", defaultSigV4Region),
		AccessKeyID:          os.Getenv("OBJEX_ACCESS_KEY_ID"),
		SecretAccessKey:      os.Getenv("OBJEX_SECRET_ACCESS_KEY"),
		PresignDefaultExpiry: defaultPresignExpiry,
		PresignMaxExpiry:     defaultPresignMaxExpiry,
		ClusterInternalToken: os.Getenv("OBJEX_CLUSTER_INTERNAL_TOKEN"),
	}

	if v := os.Getenv("OBJEX_MAX_UPLOAD_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("invalid OBJEX_MAX_UPLOAD_BYTES: %q", v)
		}
		cfg.MaxUploadBytes = n
	}

	if v := os.Getenv("OBJEX_MIN_FREE_DISK_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("invalid OBJEX_MIN_FREE_DISK_BYTES: %q", v)
		}
		cfg.MinFreeDiskBytes = n
	}

	if v := os.Getenv("OBJEX_PRESIGN_DEFAULT_EXPIRY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid OBJEX_PRESIGN_DEFAULT_EXPIRY: %q", v)
		}
		cfg.PresignDefaultExpiry = n
	}

	if v := os.Getenv("OBJEX_PRESIGN_MAX_EXPIRY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid OBJEX_PRESIGN_MAX_EXPIRY: %q", v)
		}
		cfg.PresignMaxExpiry = n
	}

	nodeID, clusterNodes, err := loadClusterNodes("", cfg.PublicURL, cfg.HTTPAddress)
	if err != nil {
		return Config{}, err
	}
	cfg.NodeID = nodeID
	cfg.ClusterNodes = clusterNodes

	if v := os.Getenv("OBJEX_REPLICATION_FACTOR"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid OBJEX_REPLICATION_FACTOR: %q", v)
		}
		cfg.ReplicationFactor = n
	} else if len(clusterNodes) >= 3 {
		cfg.ReplicationFactor = 3
	} else {
		cfg.ReplicationFactor = 1
	}

	defW, defR := quorum.DefaultsForN(cfg.ReplicationFactor)
	cfg.WriteQuorum = defW
	cfg.ReadQuorum = defR

	if v := os.Getenv("OBJEX_WRITE_QUORUM"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid OBJEX_WRITE_QUORUM: %q", v)
		}
		cfg.WriteQuorum = n
	}
	if v := os.Getenv("OBJEX_READ_QUORUM"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid OBJEX_READ_QUORUM: %q", v)
		}
		cfg.ReadQuorum = n
	}

	qcfg := quorum.Config{N: cfg.ReplicationFactor, W: cfg.WriteQuorum, R: cfg.ReadQuorum}
	if err := qcfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid quorum config: %w", err)
	}

	return cfg, nil
}

// AbsPaths resolves DataDir, DBPath, and BlobPath to absolute paths.
func (c Config) AbsPaths() (Config, error) {
	absData, err := filepath.Abs(c.DataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data dir: %w", err)
	}
	absDB, err := filepath.Abs(c.DBPath)
	if err != nil {
		return Config{}, fmt.Errorf("resolve db path: %w", err)
	}
	absBlob, err := filepath.Abs(c.BlobPath)
	if err != nil {
		return Config{}, fmt.Errorf("resolve blob path: %w", err)
	}

	c.DataDir = absData
	c.DBPath = absDB
	c.BlobPath = absBlob
	return c, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
