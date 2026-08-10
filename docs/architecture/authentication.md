# Authentication

ObjeX uses **AWS Signature Version 4** for all S3 API requests except health and metrics endpoints.

## Flow

```
HTTP Request → Parse Authorization / query params
            → Lookup credential by Access Key ID
            → Validate timestamp (±15 min or presigned expiry window)
            → Build canonical request
            → Build string to sign
            → Derive signing key (kDate → kRegion → kService → kSigning)
            → HMAC-SHA256 → constant-time signature compare
            → Optional payload hash verification
```

## Credentials

Stored in SQLite `s3_credentials` table. Configure via environment:

| Variable | Description |
|----------|-------------|
| `OBJEX_ACCESS_KEY_ID` | Access key (seeded on startup) |
| `OBJEX_SECRET_ACCESS_KEY` | Secret key (never logged) |

## Presigned URLs

Generate via authenticated `GET /api/presign/{bucket}/{key}?expires=N&method=GET|PUT`.

Uses query-parameter SigV4 (`X-Amz-Algorithm`, `X-Amz-Credential`, `X-Amz-Date`, `X-Amz-Expires`, `X-Amz-SignedHeaders`, `X-Amz-Signature`).

Presigned requests use `UNSIGNED-PAYLOAD` and validate expiry via `X-Amz-Expires` relative to `X-Amz-Date`.

## Errors

Matches S3-compatible codes: `AccessDenied`, `InvalidAccessKeyId`, `SignatureDoesNotMatch`, `RequestExpired`, `InvalidArgument`.
