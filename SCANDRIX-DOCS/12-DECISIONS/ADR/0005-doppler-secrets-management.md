# ADR 0005: Doppler for Centralized Secrets Management

**Classification:** ARCHITECTURE DECISION RECORD  
**Status:** APPROVED  
**Date:** 2026-08-28  
**Deciders:** Scandrix Core Architecture Team  

---

## 1. Context & Problem Statement

Scandrix operates across developer workstations, ephemeral CI/CD runners, staging clusters, and production multi-AZ Kubernetes. Storing environment variables in local unencrypted `.env` files or hardcoded Kubernetes ConfigMaps creates critical risk of secret sprawl and accidental git leaks.

---

## 2. Considered Options

1. **Doppler Enterprise**: Centralized secret platform with instant CLI injection, Kubernetes Secrets Operator integration, audit trails, and automatic secret rotation.
2. **Raw Kubernetes Secrets / Sealed Secrets**: Cumbersome developer experience, lack of centralized audit logs, difficult cross-environment drift tracking.
3. **HashiCorp Vault**: Powerful, but high operational maintenance overhead for initial team velocity.

---

## 3. Decision Outcome

We select **Doppler** as the authoritative source of configuration and application secrets across all non-production and production environments.

### Key Justifications:
- **Developer Simplicity**: Developers run `doppler run -- go run cmd/scandrix-api/main.go`, injecting credentials without writing secrets to disk.
- **Kubernetes Operator**: The Doppler Operator automatically synchronizes encrypted secrets to native Kubernetes Secret objects.
- **Auditability**: Every secret access and modification is recorded in Doppler's audit logs.

---

## 4. Consequences

- **Positive**: Zero plaintext `.env` files in git or developer laptops; automated secret rotation support; unified cross-environment visibility.
- **Trade-off**: External SaaS dependency during cloud deployment (mitigated via local caching and offline fallback).
