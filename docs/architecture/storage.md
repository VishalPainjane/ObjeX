# Storage Layout

Physical blobs use SHA256-derived paths, not raw object keys:

```
{blobRoot}/{bucket}/{L1}/{L2}/{hash}.blob

L1 = hash[0:2], L2 = hash[2:4]
hash = SHA256("{bucket}/{key}")
```

## Writes

1. Stream to `{hash}.blob.{random}.tmp`
2. `fsync` then atomic rename to `{hash}.blob`
3. Stale `.tmp` files older than 1 hour are removed on startup

## Multipart parts

Parts are stored outside the content-addressed layout:

```
{blobRoot}/_multipart/{uploadId}/{partNumber}.part
```

On complete, parts are concatenated into the final hashed blob path. Part directories are removed after complete or abort.

## Orphan blobs

If metadata commit fails after a successful blob write, the object service deletes the blob. A background job also scans `*.blob` files and removes any not referenced in metadata.
