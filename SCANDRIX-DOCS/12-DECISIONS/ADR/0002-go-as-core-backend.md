# ADR 0002: Go (Go 1.24+) as Primary Backend Language

**Classification:** ARCHITECTURE DECISION RECORD  
**Status:** APPROVED  
**Date:** 2026-08-28  
**Deciders:** Scandrix Core Architecture Team  

---

## 1. Context & Problem Statement

Scandrix requires a high-performance, low-latency backend capable of handling high-concurrency webhook ingestion, parallel Tree-sitter AST parsing, Dijkstra graph traversals, and single-binary CLI distribution. Furthermore, establishing clean-room intellectual property separation from Node.js/TypeScript-based tools (such as Kodus, which is built on NestJS) requires an independent language and structural foundation.

---

## 2. Considered Options

1. **Go (Go 1.24+)**: Native concurrency (goroutines), sub-millisecond startup, static binaries, low memory footprint ($< 30\text{MB}$ idle), first-class Tree-sitter and gRPC bindings.
2. **TypeScript / Node.js (NestJS)**: High ecosystem familiarity, but high memory overhead ($> 250\text{MB}$ idle per pod), single-threaded event loop bottlenecks during heavy AST computation, and copyright risk if replicating NestJS architecture.
3. **Rust**: Maximum performance and zero-cost abstractions, but higher developer learning curve, longer compile times, and slower feature iteration during initial enterprise launch.

---

## 3. Decision Outcome

We select **Go (Go 1.24+)** for all core APIs (`scandrix-api`), worker daemons (`scandrix-worker`), and the terminal CLI (`scandrix-cli`).

### Key Justifications:
- **Clean-Room Legal Isolation**: Using Go ensures 100% syntactic, structural, and runtime differentiation from TypeScript/NestJS codebases.
- **Performance & Efficiency**: Native goroutines allow Scandrix to process thousands of concurrent webhook events on modest cloud compute.
- **Single Static Binary CLI**: Compiles into a self-contained binary with zero external runtime dependencies for developers.

---

## 4. Consequences

- **Positive**: Exceptional concurrency throughput; predictable low memory usage; zero runtime node/npm dependencies on developer machines.
- **Trade-off**: Requires strict discipline around interface design and explicit error handling (`if err != nil`).
