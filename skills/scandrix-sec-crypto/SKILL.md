---
name: scandrix-sec-crypto
description: Cryptographic standards, secure key management, and data encryption policies
---

# ScanDrix Crypto Standards

### Approved Cryptographic Primitives
- **Symmetric Encryption**: AES-256-GCM, ChaCha20-Poly1305. (Disallow: AES-ECB, AES-CBC without HMAC, DES, 3DES, RC4).
- **Asymmetric Encryption / Signatures**: RSA (≥ 2048-bit, prefer 4096-bit), Ed25519, ECDSA (P-256 or higher).
- **Hashing**: SHA-256, SHA-384, SHA-512, BLAKE2b. (Disallow: MD5, SHA-1).
- **Password Storage**: Argon2id (preferred), bcrypt (work factor ≥ 12), scrypt. Never use standard SHA-256 for passwords.

### Operational Rules
1. **Never roll custom crypto**: Use standard library cryptographic modules (`crypto/rand`, `crypto.randomBytes`, `secrets`).
2. **CSPRNG**: Never use `Math.random()`, `rand()`, or seedable PRNGs for tokens, IVs, salts, nonces, or session IDs.
3. **Nonces & IVs**: Generate nonces via CSPRNG and never reuse an IV/nonce with the same key.
4. **Key Rotation & Lifecycle**: Implement distinct separation of Key Encrypting Keys (KEK) and Data Encrypting Keys (DEK).
