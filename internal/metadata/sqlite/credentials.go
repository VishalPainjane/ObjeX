package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/VishalPainjane/objex/internal/auth"
)

// CredentialStore implements auth.CredentialStore.
type CredentialStore struct {
	store *Store
}

// Credentials returns a credential store backed by this SQLite store.
func (s *Store) Credentials() *CredentialStore {
	return &CredentialStore{store: s}
}

func (c *CredentialStore) GetCredential(ctx context.Context, accessKeyID string) (*auth.Credential, error) {
	row := c.store.db.QueryRowContext(ctx,
		`SELECT access_key_id, secret_access_key FROM s3_credentials WHERE access_key_id = ?`,
		accessKeyID,
	)
	var cred auth.Credential
	if err := row.Scan(&cred.AccessKeyID, &cred.SecretAccessKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &cred, nil
}

// UpsertCredential inserts or updates a credential.
func (s *Store) UpsertCredential(ctx context.Context, name, accessKeyID, secretAccessKey string) error {
	now := time.Now().UTC()
	id := newID()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO s3_credentials (id, name, access_key_id, secret_access_key, created_at, updated_at) VALUES (?,?,?,?,?,?)
		 ON CONFLICT(access_key_id) DO UPDATE SET secret_access_key=excluded.secret_access_key, updated_at=excluded.updated_at`,
		id, name, accessKeyID, secretAccessKey, formatTime(now), formatTime(now),
	)
	return err
}

// FirstCredential returns the first credential if any exists.
func (s *Store) FirstCredential(ctx context.Context) (*auth.Credential, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT access_key_id, secret_access_key FROM s3_credentials ORDER BY created_at LIMIT 1`,
	)
	var cred auth.Credential
	if err := row.Scan(&cred.AccessKeyID, &cred.SecretAccessKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &cred, nil
}
