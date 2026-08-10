# Changelog

## Unreleased

### Added
- Go rewrite: S3-compatible API with AWS SigV4, multipart, presigned URLs, batch delete
- Distributed cluster mode: quorum N/R/W, hinted handoff, read repair, peer health, healing
- Background jobs: orphan blob cleanup, abandoned multipart cleanup, integrity verification
- Docker Compose (single-node + 3-node cluster), GitHub Actions CI, showcase docs

### Removed
- Legacy .NET / Blazor implementation (superseded by Go codebase)
