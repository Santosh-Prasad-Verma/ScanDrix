---
name: scandrix-owasp
description: Detection and auto-remediation for SQLi, XSS, SSRF, IDOR, and Broken Auth
---

# ScanDrix OWASP Guard

- **Injection Prevention**: Use prepared statements and bind variables for all data access.
- **XSS Sanitization**: Escape and sanitize user input before rendering in DOM/HTML templates.
- **SSRF Hardening**: Validate URL protocols (allowlist `https://` only) and prohibit private IP addresses.
- **Access Control**: Verify user authorization on every endpoint query based on session credentials.
