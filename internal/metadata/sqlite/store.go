package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/VishalPainjane/objex/internal/metadata"
	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS buckets (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    object_count INTEGER NOT NULL DEFAULT 0,
    total_size INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS objects (
    id TEXT NOT NULL PRIMARY KEY,
    bucket_name TEXT NOT NULL REFERENCES buckets(name),
    key TEXT NOT NULL,
    size INTEGER NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    etag TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    custom_metadata TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    replication_status TEXT NOT NULL DEFAULT 'replicated',
    deleted INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(bucket_name, key)
);

CREATE INDEX IF NOT EXISTS idx_objects_bucket_key ON objects(bucket_name, key);

CREATE TABLE IF NOT EXISTS multipart_uploads (
    id TEXT NOT NULL PRIMARY KEY,
    bucket_name TEXT NOT NULL REFERENCES buckets(name),
    key TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS multipart_upload_parts (
    id TEXT NOT NULL PRIMARY KEY,
    upload_id TEXT NOT NULL REFERENCES multipart_uploads(id) ON DELETE CASCADE,
    part_number INTEGER NOT NULL,
    size INTEGER NOT NULL,
    etag TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(upload_id, part_number)
);

CREATE INDEX IF NOT EXISTS idx_multipart_uploads_bucket ON multipart_uploads(bucket_name);

CREATE TABLE IF NOT EXISTS s3_credentials (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL DEFAULT 'default',
    access_key_id TEXT NOT NULL UNIQUE,
    secret_access_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

// Store implements metadata.Store using SQLite.
type Store struct {
	db *sql.DB
}

// Open opens or creates a SQLite metadata database at dbPath.
func Open(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	if err := migrateObjectsTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate objects table: %w", err)
	}
	if err := migrateHintsTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate hints table: %w", err)
	}
	if err := migrateHintsColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate hints columns: %w", err)
	}
	return &Store{db: db}, nil
}

func migrateHintsTable(db *sql.DB) error {
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='replication_hints'`).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS replication_hints (
    id TEXT NOT NULL PRIMARY KEY,
    target_node TEXT NOT NULL,
    bucket_name TEXT NOT NULL,
    key TEXT NOT NULL,
    version INTEGER NOT NULL,
    etag TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT '',
    custom_metadata TEXT,
    source_node TEXT NOT NULL,
    source_path TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    last_error TEXT,
    created_at TEXT NOT NULL,
    UNIQUE(target_node, bucket_name, key, version, kind)
);
CREATE INDEX IF NOT EXISTS idx_hints_due ON replication_hints(status, next_attempt_at);
`)
		return err
	}
	return err
}

func migrateHintsColumns(db *sql.DB) error {
	cols, err := tableColumns(db, "replication_hints")
	if err != nil {
		return err
	}
	if cols != nil && !cols["source_path"] {
		_, err = db.Exec(`ALTER TABLE replication_hints ADD COLUMN source_path TEXT NOT NULL DEFAULT ''`)
		return err
	}
	return nil
}

func migrateObjectsTable(db *sql.DB) error {
	cols, err := tableColumns(db, "objects")
	if err != nil {
		return err
	}
	if !cols["version"] {
		if _, err := db.Exec(`ALTER TABLE objects ADD COLUMN version INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	if !cols["replication_status"] {
		if _, err := db.Exec(`ALTER TABLE objects ADD COLUMN replication_status TEXT NOT NULL DEFAULT 'replicated'`); err != nil {
			return err
		}
	}
	if !cols["deleted"] {
		if _, err := db.Exec(`ALTER TABLE objects ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB exposes the underlying database for health checks.
func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) CreateBucket(ctx context.Context, name string) error {
	now := time.Now().UTC()
	id := newID()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO buckets (id, name, object_count, total_size, created_at, updated_at) VALUES (?, ?, 0, 0, ?, ?)`,
		id, name, formatTime(now), formatTime(now),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return metadata.ErrBucketExists
		}
		return err
	}
	return nil
}

func (s *Store) DeleteBucket(ctx context.Context, name string) error {
	count, err := s.objectCount(ctx, name)
	if err != nil {
		return err
	}
	if count > 0 {
		return metadata.ErrBucketNotEmpty
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM buckets WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return metadata.ErrBucketNotFound
	}
	return nil
}

func (s *Store) BucketExists(ctx context.Context, name string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM buckets WHERE name = ? LIMIT 1`, name).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) GetBucket(ctx context.Context, name string) (*metadata.Bucket, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, object_count, total_size, created_at, updated_at FROM buckets WHERE name = ?`, name,
	)
	b, err := scanBucket(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, metadata.ErrBucketNotFound
	}
	return b, err
}

func (s *Store) ListBuckets(ctx context.Context) ([]metadata.Bucket, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, object_count, total_size, created_at, updated_at FROM buckets ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []metadata.Bucket
	for rows.Next() {
		b, err := scanBucket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func (s *Store) PutObject(ctx context.Context, obj metadata.Object) error {
	if obj.BucketName == "" || obj.Key == "" {
		return metadata.ErrInvalidArgument
	}
	exists, err := s.BucketExists(ctx, obj.BucketName)
	if err != nil {
		return err
	}
	if !exists {
		return metadata.ErrBucketNotFound
	}

	metaJSON, err := encodeMetadata(obj.CustomMetadata)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if obj.ID == "" {
		obj.ID = newID()
	}
	if obj.ContentType == "" {
		obj.ContentType = "application/octet-stream"
	}
	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = now
	}
	obj.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldSize int64
	err = tx.QueryRowContext(ctx,
		`SELECT size FROM objects WHERE bucket_name = ? AND key = ?`, obj.BucketName, obj.Key,
	).Scan(&oldSize)
	isUpdate := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if isUpdate {
		_, err = tx.ExecContext(ctx,
			`UPDATE objects SET size=?, content_type=?, etag=?, storage_path=?, custom_metadata=?, version=?, replication_status=?, deleted=?, updated_at=? WHERE bucket_name=? AND key=?`,
			obj.Size, obj.ContentType, obj.ETag, obj.StoragePath, metaJSON, obj.Version, obj.ReplicationStatus, boolToInt(obj.Deleted), formatTime(obj.UpdatedAt), obj.BucketName, obj.Key,
		)
	} else {
		if obj.Version == 0 {
			obj.Version = 1
		}
		if obj.ReplicationStatus == "" {
			obj.ReplicationStatus = "replicated"
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO objects (id, bucket_name, key, size, content_type, etag, storage_path, custom_metadata, version, replication_status, deleted, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			obj.ID, obj.BucketName, obj.Key, obj.Size, obj.ContentType, obj.ETag, obj.StoragePath, metaJSON, obj.Version, obj.ReplicationStatus, boolToInt(obj.Deleted),
			formatTime(obj.CreatedAt), formatTime(obj.UpdatedAt),
		)
	}
	if err != nil {
		if isUniqueViolation(err) {
			// Concurrent insert — fetch existing id and retry as update once.
			var existingID string
			if qerr := tx.QueryRowContext(ctx,
				`SELECT id FROM objects WHERE bucket_name = ? AND key = ?`, obj.BucketName, obj.Key,
			).Scan(&existingID); qerr == nil {
				obj.ID = existingID
				isUpdate = true
				err = tx.QueryRowContext(ctx,
					`SELECT size FROM objects WHERE bucket_name = ? AND key = ?`, obj.BucketName, obj.Key,
				).Scan(&oldSize)
				if err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx,
					`UPDATE objects SET size=?, content_type=?, etag=?, storage_path=?, custom_metadata=?, version=?, replication_status=?, deleted=?, updated_at=? WHERE bucket_name=? AND key=?`,
					obj.Size, obj.ContentType, obj.ETag, obj.StoragePath, metaJSON, obj.Version, obj.ReplicationStatus, boolToInt(obj.Deleted), formatTime(obj.UpdatedAt), obj.BucketName, obj.Key,
				)
			}
		}
		if err != nil {
			return err
		}
	}

	sizeDelta := obj.Size - oldSize
	countDelta := 0
	if !isUpdate && !obj.Deleted {
		countDelta = 1
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE buckets SET object_count = object_count + ?, total_size = total_size + ?, updated_at = ? WHERE name = ?`,
		countDelta, sizeDelta, formatTime(now), obj.BucketName,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetObject(ctx context.Context, bucket, key string) (*metadata.Object, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, bucket_name, key, size, content_type, etag, storage_path, custom_metadata, version, replication_status, deleted, created_at, updated_at FROM objects WHERE bucket_name = ? AND key = ?`,
		bucket, key,
	)
	o, err := scanObject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, metadata.ErrObjectNotFound
	}
	return o, err
}

// PutTombstone records a versioned delete marker for bucket/key.
func (s *Store) PutTombstone(ctx context.Context, bucket, key string, version int64) error {
	exists, err := s.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		return metadata.ErrBucketNotFound
	}

	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldSize int64
	var oldDeleted int
	err = tx.QueryRowContext(ctx,
		`SELECT size, deleted FROM objects WHERE bucket_name = ? AND key = ?`, bucket, key,
	).Scan(&oldSize, &oldDeleted)
	isUpdate := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if isUpdate {
		_, err = tx.ExecContext(ctx,
			`UPDATE objects SET size=0, content_type='application/octet-stream', etag='', storage_path='', custom_metadata=NULL, version=?, replication_status='replicated', deleted=1, updated_at=? WHERE bucket_name=? AND key=?`,
			version, formatTime(now), bucket, key,
		)
	} else {
		id := newID()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO objects (id, bucket_name, key, size, content_type, etag, storage_path, custom_metadata, version, replication_status, deleted, created_at, updated_at) VALUES (?,?,?,0,'application/octet-stream','','',NULL,?,'replicated',1,?,?)`,
			id, bucket, key, version, formatTime(now), formatTime(now),
		)
	}
	if err != nil {
		return err
	}

	if isUpdate && oldDeleted == 0 {
		_, err = tx.ExecContext(ctx,
			`UPDATE buckets SET object_count = object_count - 1, total_size = total_size - ?, updated_at = ? WHERE name = ?`,
			oldSize, formatTime(now), bucket,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UpdateReplicationStatus sets replication_status on the primary's object metadata.
func (s *Store) UpdateReplicationStatus(ctx context.Context, bucket, key, status string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE objects SET replication_status = ?, updated_at = ? WHERE bucket_name = ? AND key = ?`,
		status, formatTime(time.Now().UTC()), bucket, key,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return metadata.ErrObjectNotFound
	}
	return nil
}

// ListObjectsByReplicationStatus returns objects with the given replication_status (primary metadata).
func (s *Store) ListObjectsByReplicationStatus(ctx context.Context, status string, limit int) ([]metadata.Object, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, bucket_name, key, size, content_type, etag, storage_path, custom_metadata, version, replication_status, deleted, created_at, updated_at
FROM objects
WHERE replication_status = ? AND deleted = 0
ORDER BY updated_at DESC
LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []metadata.Object
	for rows.Next() {
		obj, err := scanObject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *obj)
	}
	return out, rows.Err()
}

func (s *Store) DeleteObject(ctx context.Context, bucket, key string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var size int64
	err = tx.QueryRowContext(ctx,
		`SELECT size FROM objects WHERE bucket_name = ? AND key = ?`, bucket, key,
	).Scan(&size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // idempotent
	}
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM objects WHERE bucket_name = ? AND key = ?`, bucket, key)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx,
		`UPDATE buckets SET object_count = object_count - 1, total_size = total_size - ?, updated_at = ? WHERE name = ?`,
		size, formatTime(now), bucket,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListObjects(ctx context.Context, bucket string, opts metadata.ListOptions) (metadata.ListResult, error) {
	if opts.MaxKeys <= 0 {
		opts.MaxKeys = 1000
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bucket_name, key, size, content_type, etag, storage_path, custom_metadata, version, replication_status, deleted, created_at, updated_at FROM objects WHERE bucket_name = ? AND key >= ? ORDER BY key`,
		bucket, effectiveStart(opts),
	)
	if err != nil {
		return metadata.ListResult{}, err
	}
	defer rows.Close()

	var all []metadata.Object
	for rows.Next() {
		o, err := scanObject(rows)
		if err != nil {
			return metadata.ListResult{}, err
		}
		if opts.Prefix != "" && !strings.HasPrefix(o.Key, opts.Prefix) {
			continue
		}
		if o.Deleted {
			continue
		}
		all = append(all, *o)
	}
	if err := rows.Err(); err != nil {
		return metadata.ListResult{}, err
	}

	if opts.Delimiter == "" {
		return paginateFlat(all, opts)
	}
	return listWithDelimiter(all, opts)
}

func (s *Store) objectCount(ctx context.Context, name string) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM objects WHERE bucket_name = ?`, name).Scan(&n)
	return n, err
}

func paginateFlat(all []metadata.Object, opts metadata.ListOptions) (metadata.ListResult, error) {
	start := 0
	if opts.ContinuationToken != "" {
		for i, o := range all {
			if o.Key > opts.ContinuationToken {
				start = i
				break
			}
			if i == len(all)-1 {
				start = len(all)
			}
		}
	}

	end := start + opts.MaxKeys
	truncated := end < len(all)
	if end > len(all) {
		end = len(all)
	}
	page := all[start:end]

	var next string
	if truncated && len(page) > 0 {
		next = page[len(page)-1].Key
	}
	return metadata.ListResult{
		Objects:               page,
		NextContinuationToken: next,
		IsTruncated:           truncated,
	}, nil
}

func listWithDelimiter(all []metadata.Object, opts metadata.ListOptions) (metadata.ListResult, error) {
	prefixSet := make(map[string]struct{})
	var objects []metadata.Object

	for _, o := range all {
		rest := strings.TrimPrefix(o.Key, opts.Prefix)
		idx := strings.Index(rest, opts.Delimiter)
		if idx >= 0 {
			common := opts.Prefix + rest[:idx+len(opts.Delimiter)]
			prefixSet[common] = struct{}{}
			continue
		}
		objects = append(objects, o)
	}

	prefixes := make([]string, 0, len(prefixSet))
	for p := range prefixSet {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)

	// Paginate objects only; common prefixes included when in window
	result, err := paginateFlat(objects, opts)
	if err != nil {
		return metadata.ListResult{}, err
	}
	result.CommonPrefixes = prefixes
	return result, nil
}

func scanBucket(row scanner) (*metadata.Bucket, error) {
	var b metadata.Bucket
	var created, updated string
	if err := row.Scan(&b.ID, &b.Name, &b.ObjectCount, &b.TotalSize, &created, &updated); err != nil {
		return nil, err
	}
	b.CreatedAt, _ = parseTime(created)
	b.UpdatedAt, _ = parseTime(updated)
	return &b, nil
}

func scanObject(row scanner) (*metadata.Object, error) {
	var o metadata.Object
	var metaJSON sql.NullString
	var created, updated string
	var deleted int
	if err := row.Scan(&o.ID, &o.BucketName, &o.Key, &o.Size, &o.ContentType, &o.ETag, &o.StoragePath, &metaJSON, &o.Version, &o.ReplicationStatus, &deleted, &created, &updated); err != nil {
		return nil, err
	}
	o.Deleted = deleted != 0
	if metaJSON.Valid && metaJSON.String != "" {
		o.CustomMetadata, _ = decodeMetadata(metaJSON.String)
	}
	o.CreatedAt, _ = parseTime(created)
	o.UpdatedAt, _ = parseTime(updated)
	return &o, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func encodeMetadata(m map[string]string) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeMetadata(s string) (map[string]string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func newID() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
func effectiveStart(opts metadata.ListOptions) string {
	if opts.StartAfter != "" {
		return opts.StartAfter
	}
	if opts.Prefix != "" {
		return opts.Prefix
	}
	return ""
}
