"""
ScanDrix Wake-on-Request Proxy Gateway (AWS Lambda)

Acts as a zero-cost serverless ingress (via Lambda Function URL).
When an incoming HTTP request (API call or webhook) arrives:
  1. Checks if EC2 instance (i-07a8af27e8bc5380c) is running.
  2. If stopped, starts the instance via ec2:StartInstances and polls until port is healthy (~20-25s).
  3. Seamlessly proxies the HTTP request, query parameters, headers, and body to the backend.
  4. Returns the backend response to the client.

Once the backend is idle for 15 minutes, the on-instance watchdog daemon powers down the EC2 automatically.
"""

import base64
import json
import logging
import os
import socket
import time
import urllib.error
import urllib.parse
import urllib.request

logger = logging.getLogger()
logger.setLevel(logging.INFO)

INSTANCE_ID = os.environ.get("EC2_INSTANCE_ID", "i-07a8af27e8bc5380c")
AWS_REGION = os.environ.get("AWS_REGION", "ap-southeast-2")
FALLBACK_HOST = os.environ.get("EC2_HOST", "3.107.153.4")
API_PORT = int(os.environ.get("API_PORT", "8080"))
WEBHOOKS_PORT = int(os.environ.get("WEBHOOKS_PORT", "8081"))
BOOT_TIMEOUT_SECS = int(os.environ.get("BOOT_TIMEOUT_SECS", "50"))

import boto3

ec2_client = boto3.client("ec2", region_name=AWS_REGION)


def get_instance_info():
    resp = ec2_client.describe_instances(InstanceIds=[INSTANCE_ID])
    inst = resp["Reservations"][0]["Instances"][0]
    state = inst["State"]["Name"]
    public_ip = inst.get("PublicIpAddress") or FALLBACK_HOST
    return state, public_ip


def wait_for_tcp(host, port, timeout=BOOT_TIMEOUT_SECS):
    start = time.time()
    while time.time() - start < timeout:
        try:
            with socket.create_connection((host, port), timeout=2):
                return True
        except (socket.timeout, OSError):
            time.sleep(2)
    return False


def ensure_ec2_running():
    state, host = get_instance_info()
    logger.info(f"EC2 {INSTANCE_ID} current state: {state}, host: {host}")

    if state == "running":
        return host

    if state in ("stopped", "stopping"):
        # If transitioning from stopping, wait until fully stopped before start
        while state == "stopping":
            time.sleep(3)
            state, host = get_instance_info()

        logger.info(f"Triggering start for EC2 {INSTANCE_ID}...")
        ec2_client.start_instances(InstanceIds=[INSTANCE_ID])

    # Wait for instance state to reach running
    start_time = time.time()
    while time.time() - start_time < BOOT_TIMEOUT_SECS:
        state, host = get_instance_info()
        if state == "running":
            break
        time.sleep(2)

    logger.info(f"EC2 running with host {host}. Waiting for port {API_PORT} to accept traffic...")
    healthy = wait_for_tcp(host, API_PORT, timeout=BOOT_TIMEOUT_SECS)
    if not healthy:
        logger.warning(f"Port {API_PORT} not healthy within timeout; attempting request anyway.")

    return host


def lambda_handler(event, context):
    logger.info("Incoming request to Wake-on-Request Gateway")

    # 1. Parse request parameters
    raw_path = event.get("rawPath") or event.get("path") or "/"
    raw_query = event.get("rawQueryString", "")
    method = (event.get("requestContext", {}).get("http", {}).get("method") or 
              event.get("httpMethod") or "GET").upper()
    headers = event.get("headers", {})

    # Route webhooks paths to port 8081, all other API paths to port 8080
    target_port = WEBHOOKS_PORT if raw_path.startswith("/webhooks") or "/webhook" in raw_path else API_PORT

    # Decode body if base64 encoded
    body = event.get("body")
    body_bytes = None
    if body:
        if event.get("isBase64Encoded", False):
            body_bytes = base64.b64decode(body)
        else:
            body_bytes = body.encode("utf-8")

    # 2. Wake EC2 if stopped
    try:
        backend_host = ensure_ec2_running()
    except Exception as e:
        logger.error(f"Failed to start/verify EC2 {INSTANCE_ID}: {e}")
        return {
            "statusCode": 503,
            "headers": {"Content-Type": "application/json"},
            "body": json.dumps({"error": "Failed to wake backend instance", "detail": str(e)})
        }

    # 3. Construct target URL
    query_str = f"?{raw_query}" if raw_query else ""
    target_url = f"http://{backend_host}:{target_port}{raw_path}{query_str}"
    logger.info(f"Forwarding {method} request to: {target_url}")

    # Clean headers before forwarding
    forward_headers = {k: v for k, v in headers.items() if k.lower() not in ("host", "x-forwarded-for", "x-forwarded-proto")}
    forward_headers["Host"] = f"{backend_host}:{target_port}"

    req = urllib.request.Request(
        url=target_url,
        data=body_bytes,
        headers=forward_headers,
        method=method
    )

    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            resp_body = resp.read()
            resp_headers = {k: v for k, v in resp.headers.items() if k.lower() not in ("transfer-encoding", "content-encoding")}
            is_text = resp.headers.get_content_type().startswith(("text/", "application/json", "application/xml"))

            return {
                "statusCode": resp.status,
                "headers": resp_headers,
                "isBase64Encoded": not is_text,
                "body": resp_body.decode("utf-8", errors="replace") if is_text else base64.b64encode(resp_body).decode("ascii")
            }
    except urllib.error.HTTPError as http_err:
        err_body = http_err.read()
        return {
            "statusCode": http_err.code,
            "headers": {k: v for k, v in http_err.headers.items() if k.lower() not in ("transfer-encoding", "content-encoding")},
            "body": err_body.decode("utf-8", errors="replace")
        }
    except Exception as net_err:
        logger.error(f"Proxy network error connecting to backend: {net_err}")
        return {
            "statusCode": 502,
            "headers": {"Content-Type": "application/json"},
            "body": json.dumps({"error": "Bad Gateway", "detail": str(net_err)})
        }
