---
name: scandrix-sec-input-web
description: Input sanitization, schema enforcement, XSS, CSRF, and file upload protection
---

# ScanDrix Web Security & Input Validation

### Input Boundaries & Schemas
- **Strict Allowlisting**: Validate all incoming payloads against schemas (Zod, Pydantic, Joi, JSON Schema). Reject unexpected fields.
- **Type Coercion Defense**: Validate type, string length, character set, and range limits before processing.

### Web Defense Layers
- **XSS Prevention**: Context-aware output encoding. Prohibit `dangerouslySetInnerHTML`, `v-html`, and unescaped template expressions.
- **File Upload Security**:
  - Verify file headers via magic bytes (`libmagic`), never rely solely on file extensions or `Content-Type` headers.
  - Store uploaded files outside the web root (e.g., dedicated S3 bucket with private ACLs).
  - Force random UUIDs for uploaded filenames to prevent path traversal (`../../`).
- **Cookie Security**:
  - Required flags: `HttpOnly; Secure; SameSite=Strict` (use `SameSite=Lax` specifically on top-level OAuth/SSO callback endpoints to permit IdP cross-origin redirects without dropping session tokens).
