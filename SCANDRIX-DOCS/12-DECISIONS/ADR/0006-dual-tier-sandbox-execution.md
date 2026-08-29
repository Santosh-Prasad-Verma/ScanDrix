# ADR 0006: Dual-Tier Sandbox Execution (gVisor & Firecracker)

**Classification:** ARCHITECTURE DECISION RECORD  
**Status:** APPROVED  
**Date:** 2026-08-28  
**Deciders:** Scandrix Core Architecture Team  

---

## 1. Context & Problem Statement

Scandrix evaluates untrusted source code, compiles third-party dependencies, runs developer test suites, and verifies AI-synthesized patches. Executing untrusted code directly on the host machine or in standard rootless Docker containers exposes the platform to kernel vulnerabilities, container escape exploits (e.g., CVE-2024-21626), and network exfiltration.

---

## 2. Considered Options

1. **Dual-Tier Sandboxing (gVisor `runsc` + Firecracker MicroVMs)**: Fast syscall-intercepted gVisor for high-throughput static analysis ($< 1.5\text{s}$) paired with hardware-isolated Firecracker MicroVMs for arbitrary code execution and test runs.
2. **Standard Docker Containers (`runc`)**: Fast, but shares the host Linux kernel directly; vulnerable to kernel privilege escalations.
3. **Full QEMU / VirtualBox Virtual Machines**: Secure hardware virtualization, but slow boot times ($> 15\text{s}$) and massive RAM overhead, making continuous PR reviews impractical.

---

## 3. Decision Outcome

We select a **Dual-Tier Sandboxing Architecture**:
- **Tier 1 (Docker + gVisor `runsc`)**: Deployed for Tree-sitter parsing, Semgrep, and fast static analysis. Drops all Linux capabilities (`CAP_DROP_ALL`), runs read-only with `network: none`.
- **Tier 2 (Firecracker MicroVMs / KVM)**: Deployed for the Closed-Loop Proof-of-Fix engine to compile code, run test suites, and execute dynamic probes inside a dedicated guest Linux kernel with hardware KVM isolation.

---

## 4. Consequences

- **Positive**: Complete defense against host kernel compromise; sub-second boot times for static passes; secure arbitrary code execution.
- **Trade-off**: Requires bare-metal or nested-virtualization-capable cloud compute instances (e.g., AWS `c5.metal` or `c6i.metal`).
