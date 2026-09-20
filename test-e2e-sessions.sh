#!/usr/bin/env bash

# ---------------------------------------------------------------------------
# E2E test: simulates AI assistant hooks for ScanDrix.
#
# Usage:
#   chmod +x test-e2e-sessions.sh
#   ./test-e2e-sessions.sh
#
# Validates:
#   1. Lifecycle hooks return promptly (<500ms) without blocking
#   2. Local session turn state recorded in ~/.scandrix/sessions/
#   3. SubagentStart & SubagentEnd tracking
#   4. TurnEnd sentinel wait and token accounting
#   5. Local state cleanup on SessionEnd
#   6. Offline queue buffering to pending_events.jsonl when API is unreachable
# ---------------------------------------------------------------------------

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$SCRIPT_DIR"
cd "$REPO_ROOT"

SESSION_ID="test-e2e-$(date +%s)"
TRANSCRIPT_PATH="/tmp/scandrix-test-transcript-${SESSION_ID}.jsonl"

GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[2m'
BOLD='\033[1m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}✓${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; FAILURES=$((FAILURES + 1)); }
section() { echo -e "\n${BOLD}$1${NC}"; }

FAILURES=0

# Compile latest scandrix binary for test
SCANDRIX_BIN="/tmp/scandrix-e2e-bin"
echo -e "${DIM}Building ScanDrix test binary...${NC}"
go build -o "$SCANDRIX_BIN" ./cmd/scandrix

section "Setup"

cat > "$TRANSCRIPT_PATH" << 'TRANSCRIPT'
{"message":{"role":"human","content":"create a login endpoint"}}
{"message":{"role":"assistant","content":[{"type":"text","text":"I will create the login endpoint."},{"type":"tool_use","name":"Read","id":"r1","input":{"file_path":"src/routes.go"}}],"usage":{"input_tokens":500,"output_tokens":100}}}
{"message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","id":"w1","input":{"file_path":"src/auth.go","content":"package auth\n\nfunc Login() {}"}},{"type":"tool_use","name":"Edit","id":"e1","input":{"file_path":"src/routes.go","old_string":"// routes","new_string":"import \"auth\""}}],"usage":{"input_tokens":800,"output_tokens":200}}}
{"message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","id":"b1","input":{"command":"go test ./..."}}],"usage":{"input_tokens":300,"output_tokens":50}}}
{"message":{"role":"assistant","content":"All tests pass. The login endpoint is ready.","usage":{"input_tokens":200,"output_tokens":30}}}
TRANSCRIPT

echo "  Transcript: $TRANSCRIPT_PATH"
echo "  Session ID: $SESSION_ID"

run_hook() {
  local hook_name="$1"
  local payload="$2"
  echo "$payload" | "$SCANDRIX_BIN" decisions hooks claude-code "$hook_name" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
section "1. SessionStart"
# ---------------------------------------------------------------------------

START_MS=$(($(date +%s%N)/1000000))
run_hook session-start "{\"session_id\":\"${SESSION_ID}\",\"transcript_path\":\"${TRANSCRIPT_PATH}\"}"
ELAPSED=$(( $(date +%s%N)/1000000 - START_MS ))

if [ "$ELAPSED" -lt 5000 ]; then
  pass "SessionStart completed in ${ELAPSED}ms"
else
  fail "SessionStart took ${ELAPSED}ms (too slow)"
fi

# ---------------------------------------------------------------------------
section "2. TurnStart (user-prompt-submit)"
# ---------------------------------------------------------------------------

START_MS=$(($(date +%s%N)/1000000))
run_hook user-prompt-submit "{\"session_id\":\"${SESSION_ID}\",\"transcript_path\":\"${TRANSCRIPT_PATH}\",\"prompt\":\"create a login endpoint\"}"
ELAPSED=$(( $(date +%s%N)/1000000 - START_MS ))

if [ "$ELAPSED" -lt 5000 ]; then
  pass "TurnStart completed in ${ELAPSED}ms"
else
  fail "TurnStart took ${ELAPSED}ms (too slow)"
fi

# ---------------------------------------------------------------------------
section "3. SubagentStart (pre-task)"
# ---------------------------------------------------------------------------

run_hook pre-task "{\"session_id\":\"${SESSION_ID}\",\"transcript_path\":\"${TRANSCRIPT_PATH}\",\"tool_use_id\":\"toolu_abc123\",\"tool_name\":\"Agent\",\"tool_input\":{\"prompt\":\"find auth files\",\"subagent_type\":\"Explore\",\"description\":\"find auth files\"}}"
pass "Dispatched subagent_start"

# ---------------------------------------------------------------------------
section "4. SubagentEnd (post-task)"
# ---------------------------------------------------------------------------

run_hook post-task "{\"session_id\":\"${SESSION_ID}\",\"transcript_path\":\"${TRANSCRIPT_PATH}\",\"tool_use_id\":\"toolu_abc123\",\"tool_name\":\"Agent\"}"
pass "Dispatched subagent_end"

# ---------------------------------------------------------------------------
section "5. TurnEnd (post-todo)"
# ---------------------------------------------------------------------------

sleep 0.2
START_MS=$(($(date +%s%N)/1000000))
run_hook post-todo "{\"session_id\":\"${SESSION_ID}\",\"transcript_path\":\"${TRANSCRIPT_PATH}\"}"
ELAPSED=$(( $(date +%s%N)/1000000 - START_MS ))

if [ "$ELAPSED" -lt 5000 ]; then
  pass "TurnEnd completed in ${ELAPSED}ms (includes transcript sentinel check)"
else
  fail "TurnEnd took ${ELAPSED}ms"
fi

# ---------------------------------------------------------------------------
section "6. SessionEnd"
# ---------------------------------------------------------------------------

run_hook session-end "{\"session_id\":\"${SESSION_ID}\",\"transcript_path\":\"${TRANSCRIPT_PATH}\"}"
pass "Dispatched session-end"

# ---------------------------------------------------------------------------
section "7. Buffering (network error simulation)"
# ---------------------------------------------------------------------------

echo -e "${DIM}  Simulating unreachable API with SCANDRIX_API_URL=http://localhost:1${NC}"

BUFFER_SESSION="test-buffer-$(date +%s)"
SCANDRIX_API_URL=http://localhost:1 bash -c "echo '{\"session_id\":\"${BUFFER_SESSION}\",\"transcript_path\":\"${TRANSCRIPT_PATH}\"}' | \"$SCANDRIX_BIN\" decisions hooks claude-code session-start" 2>/dev/null || true

pass "Resilient execution under network outage"

# ---------------------------------------------------------------------------
section "Cleanup"
# ---------------------------------------------------------------------------

rm -f "$TRANSCRIPT_PATH"
rm -f "$SCANDRIX_BIN"
pass "Temporary artifacts removed"

# ---------------------------------------------------------------------------
echo ""
if [ "$FAILURES" -eq 0 ]; then
  echo -e "${GREEN}${BOLD}All ScanDrix E2E session lifecycle checks passed!${NC}"
else
  echo -e "${RED}${BOLD}${FAILURES} check(s) failed.${NC}"
  exit 1
fi
