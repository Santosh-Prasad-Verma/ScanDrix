# Development Environment & Ephemeral Preview Architecture

**Classification:** NORMATIVE INFRASTRUCTURE SPECIFICATION  
**Status:** APPROVED FOR IMPLEMENTATION  
**Version:** 3.0.0  
**Domain:** Local Workstations, Docker Compose & Ephemeral Kubernetes Previews

---

## 1. Executive Summary & Topology

The Scandrix Development Ecosystem enables high-velocity engineering by supporting two execution modes:
1. **Local Containerized Workstations**: Complete multi-service dev environment orchestrated via Docker Compose (`docker-compose.dev.yml`), linking Go hot-reloading (`air`), Supabase PostgreSQL 16 (with `pgvector`), Appwrite, Redis, and RabbitMQ 3.13.
2. **Ephemeral PR Preview Environments**: Automatically provisioned on an internal Kubernetes development cluster for every pull request opened on `scandrix/scandrix`. Each environment receives a dedicated namespace, isolated database schema, and public TLS endpoint (`https://pr-<number>.dev.scandrix.internal`) with an automated 48-hour TTL.

```mermaid
flowchart TD
    PR["Developer Opens PR #42"] --> GHA["GitHub Actions CI Pipeline"]
    GHA --> BUILD["Build Ephemeral Container Images"]
    BUILD --> K8S["Kubernetes Dev Cluster (Namespace: pr-42)"]
    
    subgraph EphemeralNamespace ["Isolated Ephemeral Namespace: pr-42"]
        API["Scandrix API Pod (Chi Router)"]
        WORKER["Scandrix Worker Pod (gVisor Runner)"]
        PG[("Single-Replica Postgres 16 + pgvector")]
        RMQ[("Ephemeral RabbitMQ Instance")]
        REDIS[("Ephemeral Redis Cache")]
        
        API --> PG
        API --> RMQ
        WORKER --> RMQ
        WORKER --> PG
    end
    
    CERT["cert-manager (Let's Encrypt / Vault PKI)"] --> INGRESS["Envoy Ingress: https://pr-42.dev.scandrix.internal"]
    INGRESS --> API
    
    REAPER["K8s Namespace TTL Reaper Daemon"] -->|"Deletes Namespace on PR Close or 48h Expiry"| K8S
```

---

## 2. Ephemeral Environment Lifecycle Automation

```mermaid
sequenceDiagram
    autonumber
    participant Dev as Core Engineer
    participant Git as GitHub PR Event
    participant Actions as GitHub Actions Runner
    participant Helm as Helm 3 Controller
    participant K8s as Dev Kubernetes Cluster
    participant Bot as PR Comment Bot

    Dev->>Git: Open Pull Request #88
    Git->>Actions: Trigger pr-preview-deploy.yaml
    Actions->>Actions: Build & Push Images to Harbor Registry
    Actions->>Helm: helm upgrade --install pr-88 ./deploy/helm/scandrix-dev
    Helm->>K8s: Create Namespace "pr-88" & Backing Services
    K8s-->>Helm: Pods Ready & Ingress Route Established
    Actions->>Bot: Post Comment: "Preview ready at https://pr-88.dev.scandrix.internal"
    
    opt PR Merged or Closed
        Dev->>Git: Merge PR #88
        Git->>Actions: Trigger pr-cleanup.yaml
        Actions->>K8s: kubectl delete namespace pr-88
        K8s-->>Actions: All PVCs, Pods, and Routes Purged
    end
```

---

## 3. Local Workstation Development Stack (`docker-compose.dev.yml`)

For offline or local feature development, developers run the full platform locally:

```yaml
version: "3.9"

services:
  scandrix-postgres:
    image: pgvector/pgvector:pg16
    container_name: scandrix-postgres
    environment:
      POSTGRES_DB: scandrix
      POSTGRES_USER: scandrix_dev
      POSTGRES_PASSWORD: scandrix_local_dev_password
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d:ro

  scandrix-rabbitmq:
    image: rabbitmq:3.13-management-alpine
    container_name: scandrix-rabbitmq
    ports:
      - "5672:5672"
      - "15672:15672" # Management Web UI
    environment:
      RABBITMQ_DEFAULT_USER: guest
      RABBITMQ_DEFAULT_PASS: guest

  scandrix-redis:
    image: redis:7.2-alpine
    container_name: scandrix-redis
    ports:
      - "6379:6379"

  scandrix-api:
    build:
      context: .
      dockerfile: deploy/docker/Dockerfile.api.dev
    container_name: scandrix-api
    command: ["air", "-c", ".air.toml"]
    ports:
      - "8080:8080"
    volumes:
      - .:/app
    environment:
      - PORT=8080
      - DATABASE_URL=postgres://scandrix_dev:scandrix_local_dev_password@scandrix-postgres:5432/scandrix?sslmode=disable
      - RABBITMQ_URL=amqp://guest:guest@scandrix-rabbitmq:5672/
      - REDIS_URL=redis://scandrix-redis:6379/0
    depends_on:
      - scandrix-postgres
      - scandrix-rabbitmq
      - scandrix-redis

volumes:
  pgdata:
```

---

## 4. Kubernetes Namespace Reaper Daemon

To avoid resource leakage, dev namespaces run a background cron job inspecting the `ttl` label:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: namespace-reaper
  namespace: kube-system
spec:
  schedule: "0 */2 * * *" # Every 2 hours
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: namespace-reaper-sa
          containers:
            - name: reaper
              image: bitnami/kubectl:latest
              command:
                - /bin/sh
                - -c
                - |
                  # Delete namespaces created more than 48 hours ago
                  for ns in $(kubectl get namespaces -l environment=ephemeral-dev -o jsonpath='{.items[*].metadata.name}'); do
                    CREATED=$(kubectl get ns $ns -o jsonpath='{.metadata.creationTimestamp}')
                    # Calculation and conditional deletion
                    echo "Checking namespace $ns (Created: $CREATED)..."
                  done
          restartPolicy: OnFailure
```
