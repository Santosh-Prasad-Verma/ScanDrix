# ADR-0002: License Token Envelope Format

- **Status:** Accepted
- **Context:** Older drafts showed a dot-joined `base64(payload).base64(signature)` string and env `SCANDRIX_LICENSE_TOKEN`; code implements base64-of-JSON `SignedLicenseToken{Payload,Signature}` via `SCANDRIX_LICENSE_KEY`/`_FILE`.
- **Decision:** The code format is normative. All docs, keygen tooling, and customer instructions use the envelope only; dot-format output is forbidden because `LoadLicense` rejects it.
- **Consequences:** Keygen must call `IssueLicense` and print verbatim; a `verify` subcommand acceptance-checks every minted token through the real loader.
