# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| `main`  | Yes       |

## Reporting a Vulnerability

Please report security issues **privately** — do not open a public GitHub issue.

**Email:** painjanevishal2204@gmail.com

Include steps to reproduce, impact, and any suggested fix. I aim to respond within 7 days.

## Production Checklist

- Change default S3 credentials (`OBJEX_ACCESS_KEY_ID` / `OBJEX_SECRET_ACCESS_KEY`)
- Set a strong `OBJEX_CLUSTER_INTERNAL_TOKEN` in cluster deployments
- Run behind TLS (reverse proxy)
- Restrict network access to the S3 port
