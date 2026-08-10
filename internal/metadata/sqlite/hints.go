package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/VishalPainjane/objex/internal/metadata"
)

func (s *Store) CreateHint(ctx context.Context, hint metadata.ReplicationHint) error {
	metaJSON, err := encodeMetadata(hint.CustomMetadata)
	if err != nil {
		return err
	}
	if hint.ID == "" {
		hint.ID = newID()
	}
	now := time.Now().UTC()
	if hint.CreatedAt.IsZero() {
		hint.CreatedAt = now
	}
	if hint.Status == "" {
		hint.Status = "pending"
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO replication_hints (id, target_node, bucket_name, key, version, etag, kind, content_type, custom_metadata, source_node, source_path, attempts, next_attempt_at, status, last_error, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(target_node, bucket_name, key, version, kind) DO UPDATE SET
  etag=excluded.etag,
  content_type=excluded.content_type,
  custom_metadata=excluded.custom_metadata,
  source_path=excluded.source_path,
  status='pending',
  next_attempt_at=excluded.next_attempt_at,
  last_error=NULL`,
		hint.ID, hint.TargetNode, hint.BucketName, hint.Key, hint.Version, hint.ETag, string(hint.Kind),
		hint.ContentType, metaJSON, hint.SourceNode, hint.SourcePath, hint.Attempts, formatTime(hint.NextAttemptAt), hint.Status, hint.LastError, formatTime(hint.CreatedAt),
	)
	return err
}

func (s *Store) ListDueHints(ctx context.Context, now time.Time, limit int) ([]metadata.ReplicationHint, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, target_node, bucket_name, key, version, etag, kind, content_type, custom_metadata, source_node, source_path, attempts, next_attempt_at, status, last_error, created_at
FROM replication_hints
WHERE status = 'pending' AND next_attempt_at <= ?
ORDER BY next_attempt_at
LIMIT ?`, formatTime(now), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []metadata.ReplicationHint
	for rows.Next() {
		h, err := scanHint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) MarkHintDelivered(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM replication_hints WHERE id = ?`, id)
	return err
}

func (s *Store) RecordHintFailure(ctx context.Context, id string, nextAttempt time.Time, lastError string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE replication_hints SET attempts = attempts + 1, next_attempt_at = ?, last_error = ?, status = 'pending' WHERE id = ?`,
		formatTime(nextAttempt), lastError, id,
	)
	return err
}

func (s *Store) CountPendingHints(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM replication_hints WHERE status = 'pending'`).Scan(&n)
	return n, err
}

func (s *Store) ResetHintsForTarget(ctx context.Context, targetNode string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE replication_hints
SET next_attempt_at = ?, attempts = 0, last_error = NULL, status = 'pending'
WHERE target_node = ? AND status = 'pending'`,
		formatTime(time.Now().UTC()), targetNode,
	)
	return err
}

func (s *Store) ListHintProtectedStoragePaths(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT source_path FROM replication_hints
WHERE status = 'pending' AND source_path != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

func scanHint(row scanner) (metadata.ReplicationHint, error) {
	var h metadata.ReplicationHint
	var kind, metaJSON, sourcePath sql.NullString
	var next, created, lastErr sql.NullString
	if err := row.Scan(&h.ID, &h.TargetNode, &h.BucketName, &h.Key, &h.Version, &h.ETag, &kind, &h.ContentType, &metaJSON, &h.SourceNode, &sourcePath, &h.Attempts, &next, &h.Status, &lastErr, &created); err != nil {
		return h, err
	}
	h.Kind = metadata.HintKind(kind.String)
	h.SourcePath = sourcePath.String
	if metaJSON.Valid && metaJSON.String != "" {
		h.CustomMetadata, _ = decodeMetadata(metaJSON.String)
	}
	if next.Valid {
		h.NextAttemptAt, _ = parseTime(next.String)
	}
	if created.Valid {
		h.CreatedAt, _ = parseTime(created.String)
	}
	if lastErr.Valid {
		h.LastError = lastErr.String
	}
	return h, nil
}
