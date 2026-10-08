# ScanDrix Wake-on-Request Serverless Proxy Gateway

This solution implements **True Scale-to-Zero** for the ScanDrix AWS EC2 backend (`i-07a8af27e8bc5380c` in `ap-southeast-2`), saving ~95% of your AWS monthly credit burn.

## How It Works

```
                       ┌─────────────────────────┐
                       │  Client / GitHub Hook   │
                       └────────────┬────────────┘
                                    │ HTTP Request
                                    ▼
                       ┌─────────────────────────┐
                       │   AWS Lambda Gateway    │  (Costs $0.00 when idle)
                       │   (Function URL)        │
                       └────────────┬────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │ Is EC2 running?               │
                    ├───────────────┬───────────────┤
                 YES│               │ NO
                    │               ▼
                    │    Calls ec2:StartInstances
                    │    Waits ~20s until healthy
                    │               │
                    └───────► ◄─────┘
                              │ Proxies HTTP Request
                              ▼
        ┌──────────────────────────────────────────────────┐
        │  ScanDrix EC2 Backend (ap-southeast-2)          │
        │  - scandrix-api (port 8080)                      │
        │  - scandrix-webhooks (port 8081)                 │
        │                                                  │
        │  * Idle Watchdog Daemon (systemd)                │
        │    Shuts down EC2 after 15 minutes of inactivity │
        └──────────────────────────────────────────────────┘
```

## Deployed Components

1. **EC2 Auto-Idle Shutdown Watchdog**:
   - Installed on EC2 at `/usr/local/bin/scandrix-idle-watchdog.sh`.
   - Enabled as a systemd service `scandrix-idle-watchdog.service`.
   - Automatically executes `/sbin/shutdown -h now` when:
     - No SSH users are connected
     - No active TCP connections on ports 8080 or 8081
     - Server load is idle for 15 consecutive minutes (900s).

2. **Serverless Wake Gateway (AWS Lambda)**:
   - Handler in `handler.py`
   - CloudFormation template in `template.yaml`
   - Deploy script: `./deploy.sh`
   - Zero cost when idle (covered 100% under AWS Lambda Free Tier).

## Quick Deployment

Run once from any terminal configured with AWS CLI:
```bash
./infra/wake-gateway/deploy.sh
```
Use the output Function URL as the target for your GitHub webhook and dashboard API proxy.
