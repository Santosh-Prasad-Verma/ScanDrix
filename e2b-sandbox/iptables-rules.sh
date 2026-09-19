#!/bin/bash
set -euo pipefail

# Resolve the Shadowsocks server hostname from config.json to get current NLB IPs.
# This must run BEFORE iptables redirect is active (otherwise DNS itself gets redirected).
SS_HOST=$(grep -oP '"server"\s*:\s*"\K[^"]+' config.json || true)

if [ -n "$SS_HOST" ]; then
    NLB_IPS=$(getent ahosts "$SS_HOST" 2>/dev/null | awk '{print $1}' | grep -v ':' | sort -u || true)
    if [ -n "$NLB_IPS" ]; then
        # Create the chain if it doesn't exist, then flush it to ensure it's clean.
        iptables -t nat -N SHADOWSOCKS 2>/dev/null || iptables -t nat -F SHADOWSOCKS

        # Exclude NLB IPs from redirect (prevents sslocal -> sslocal loop)
        for ip in $NLB_IPS; do
            echo "iptables RETURN for NLB IP: $ip"
            iptables -t nat -A SHADOWSOCKS -d "$ip" -j RETURN
        done
    fi
fi

# Exclude private/reserved ranges
iptables -t nat -A SHADOWSOCKS -d 0.0.0.0/8 -j RETURN 2>/dev/null || true
iptables -t nat -A SHADOWSOCKS -d 10.0.0.0/8 -j RETURN 2>/dev/null || true
iptables -t nat -A SHADOWSOCKS -d 127.0.0.0/8 -j RETURN 2>/dev/null || true
iptables -t nat -A SHADOWSOCKS -d 169.254.0.0/16 -j RETURN 2>/dev/null || true
iptables -t nat -A SHADOWSOCKS -d 172.16.0.0/12 -j RETURN 2>/dev/null || true
iptables -t nat -A SHADOWSOCKS -d 192.168.0.0/16 -j RETURN 2>/dev/null || true

# Redirect all other outbound TCP to sslocal
iptables -t nat -A SHADOWSOCKS -p tcp -j REDIRECT --to-ports 12345 2>/dev/null || true
iptables -t nat -C OUTPUT -p tcp -j SHADOWSOCKS 2>/dev/null || iptables -t nat -A OUTPUT -p tcp -j SHADOWSOCKS 2>/dev/null || true
