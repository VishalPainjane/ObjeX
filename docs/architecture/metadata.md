# Metadata (SQLite)

SQLite stores bucket and object metadata with WAL mode and foreign keys enabled.

## Tables

- `buckets` — name, object count, total size
- `objects` — bucket, key, size, content type, ETag, storage path, custom metadata JSON
- `multipart_uploads` — in-progress upload sessions
- `multipart_upload_parts` — part metadata (cascade delete with upload)

## Listing

Object listing loads keys ordered by `key` and paginates in memory. Suitable for single-node scale; SQL-level pagination is a future optimization for very large buckets.

## Multipart

Upload sessions persist in `multipart_uploads` until complete or abort. Parts are upserted by `(upload_id, part_number)`.

Abandoned uploads older than 7 days are deleted by the background cleanup job.
