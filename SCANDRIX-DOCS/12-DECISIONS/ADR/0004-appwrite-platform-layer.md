# ADR 0004: Appwrite as Platform Infrastructure Layer

**Classification:** ARCHITECTURE DECISION RECORD  
**Status:** APPROVED  
**Date:** 2026-08-28  
**Deciders:** Scandrix Core Architecture Team  

---

## 1. Context & Problem Statement

In addition to relational records, Scandrix requires high-performance object storage for large scan diffs, real-time WebSocket state distribution for dashboard clients, transactional notifications, and avatar/presence management. Building these ancillary subsystems from scratch in custom Go services introduces significant operational surface.

---

## 2. Considered Options

1. **Appwrite Platform Integration**: Mature open-source platform offering unified Storage, Realtime WebSockets, Messaging, and Teams management with self-hostable Docker/K8s distribution.
2. **AWS S3 + Custom WebSocket Server + SendGrid**: Fragmented custom architecture requiring multiple proprietary SaaS contracts and custom Go WebSocket hub code.
3. **Firebase**: Proprietary Google cloud lock-in incompatible with air-gapped and self-hosted enterprise deployments.

---

## 3. Decision Outcome

We adopt **Appwrite** as the unified platform foundation layer for:
- **Object Storage**: Storing raw git patches, AST snapshots, and SARIF artifacts with bucket-level encryption.
- **Realtime Service**: Pushing scan progress and attack graph updates to Next.js clients over WebSockets.
- **Messaging**: Dispatching transactional email notifications to engineers.

---

## 4. Consequences

- **Positive**: Eliminates thousands of lines of boilerplate file-upload and WebSocket hub code; fully self-hostable in private clouds.
- **Trade-off**: Requires maintaining Appwrite container infrastructure alongside core Go services.
