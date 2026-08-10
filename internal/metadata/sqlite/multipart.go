package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/VishalPainjane/objex/internal/metadata"
)

func (s *Store) CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error) {
	exists, err := s.BucketExists(ctx, bucket)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", metadata.ErrBucketNotFound
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	now := time.Now().UTC()
	id := newUploadID()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO multipart_uploads (id, bucket_name, key, content_type, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		id, bucket, key, contentType, formatTime(now), formatTime(now),
	)
	return id, err
}

func (s *Store) GetMultipartUpload(ctx context.Context, uploadID, bucket, key string) (*metadata.MultipartUpload, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, bucket_name, key, content_type, created_at, updated_at FROM multipart_uploads WHERE id = ? AND bucket_name = ? AND key = ?`,
		uploadID, bucket, key,
	)
	var u metadata.MultipartUpload
	var created, updated string
	if err := row.Scan(&u.ID, &u.BucketName, &u.Key, &u.ContentType, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, metadata.ErrNoSuchUpload
		}
		return nil, err
	}
	u.CreatedAt, _ = parseTime(created)
	u.UpdatedAt, _ = parseTime(updated)
	return &u, nil
}

func (s *Store) ListMultipartUploads(ctx context.Context, bucket string) ([]metadata.MultipartUpload, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bucket_name, key, content_type, created_at, updated_at FROM multipart_uploads WHERE bucket_name = ? ORDER BY created_at`,
		bucket,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []metadata.MultipartUpload
	for rows.Next() {
		var u metadata.MultipartUpload
		var created, updated string
		if err := rows.Scan(&u.ID, &u.BucketName, &u.Key, &u.ContentType, &created, &updated); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = parseTime(created)
		u.UpdatedAt, _ = parseTime(updated)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) DeleteMultipartUpload(ctx context.Context, uploadID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM multipart_uploads WHERE id = ?`, uploadID)
	return err
}

func (s *Store) UpsertMultipartPart(ctx context.Context, part metadata.MultipartPart) error {
	now := time.Now().UTC()
	id := newID()

	res, err := s.db.ExecContext(ctx,
		`UPDATE multipart_upload_parts SET size=?, etag=?, storage_path=?, updated_at=? WHERE upload_id=? AND part_number=?`,
		part.Size, part.ETag, part.StoragePath, formatTime(now), part.UploadID, part.PartNumber,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO multipart_upload_parts (id, upload_id, part_number, size, etag, storage_path, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		id, part.UploadID, part.PartNumber, part.Size, part.ETag, part.StoragePath, formatTime(now), formatTime(now),
	)
	return err
}

func (s *Store) ListMultipartParts(ctx context.Context, uploadID string) ([]metadata.MultipartPart, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT upload_id, part_number, size, etag, storage_path, created_at, updated_at FROM multipart_upload_parts WHERE upload_id = ? ORDER BY part_number`,
		uploadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []metadata.MultipartPart
	for rows.Next() {
		var p metadata.MultipartPart
		var created, updated string
		if err := rows.Scan(&p.UploadID, &p.PartNumber, &p.Size, &p.ETag, &p.StoragePath, &created, &updated); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = parseTime(created)
		p.UpdatedAt, _ = parseTime(updated)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAbandonedMultipartUploads(ctx context.Context, olderThan time.Time) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM multipart_uploads WHERE created_at < ?`, formatTime(olderThan),
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM multipart_uploads WHERE id = ?`, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (s *Store) AllObjectStoragePaths(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT storage_path FROM objects`)
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

func (s *Store) ListAllStoredObjects(ctx context.Context) ([]metadata.Object, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bucket_name, key, size, content_type, etag, storage_path, custom_metadata, version, replication_status, deleted, created_at, updated_at
FROM objects WHERE deleted = 0 AND storage_path != '' AND etag != ''`)
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

func newUploadID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16],
	)
}
