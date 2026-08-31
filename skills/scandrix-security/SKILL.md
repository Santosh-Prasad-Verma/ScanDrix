---
name: scandrix-security
description: Enterprise security analyzer, secret scanner, and vulnerability prevention
---

# ScanDrix Security Guard

- **No Hardcoded Secrets**: Never store API keys, tokens, or private credentials in tracked files.
- **Runtime Configuration**: Always read sensitive values at runtime using environment variables (`os.Getenv` / `process.env`).
- **Parameterized Queries**: Prohibit string concatenation in database queries. Always use parameterized statements (`$1`, `?`) or ORM bindings.
- **Fail Closed**: All authorization checks must deny by default if an error occurs.
