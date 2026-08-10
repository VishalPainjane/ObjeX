# Contributing to ObjeX

Thanks for your interest! This project is maintained by **Vishal Painjane**.

## Development setup

```bash
git clone https://github.com/VishalPainjane/ObjeX.git
cd ObjeX
go test ./...
go run ./cmd/objex
```

## Pull requests

1. Open an issue for large changes
2. Run `go test ./...` and `go vet ./...` before submitting
3. Keep PRs focused — one feature or fix per PR

## Code style

- Match existing patterns in `internal/`
- No framework in domain logic (`internal/object`, `internal/metadata`)
- S3 XML errors via `internal/s3`

## Questions

Email: painjanevishal2204@gmail.com
