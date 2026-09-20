// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

// RenderTraceUIHTML returns a self-contained, offline-first dashboard HTML page.
// It loads no external CDNs, Google Fonts, or telemetry scripts, ensuring 100% security and privacy.
func RenderTraceUIHTML() string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
<meta name="color-scheme" content="dark" />
<title>ScanDrix Trace — Session Telemetry & Decision Cockpit</title>
<style>
:root {
  color-scheme: dark;
  --bg-dark: #0d1117;
  --bg-card: #161b22;
  --bg-card-hover: #21262d;
  --bg-header: #161b22;
  --border: #30363d;
  --border-strong: #484f58;
  --text: #f0f6fc;
  --text-secondary: #8b949e;
  --text-muted: #6e7681;
  --accent: #58a6ff;
  --accent-soft: rgba(88, 166, 255, 0.15);
  --success: #3fb950;
  --warning: #d29922;
  --danger: #f85149;
  --purple: #bc8cff;
  --font-mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  background: var(--bg-dark);
  color: var(--text);
  font-family: var(--font-sans);
  font-size: 14px;
  line-height: 1.5;
  height: 100vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
header {
  background: var(--bg-header);
  border-bottom: 1px solid var(--border);
  padding: 12px 20px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 700;
  font-size: 16px;
  letter-spacing: -0.02em;
}
.brand-badge {
  background: var(--accent-soft);
  color: var(--accent);
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 11px;
  font-weight: 600;
}
.header-meta {
  color: var(--text-secondary);
  font-size: 12px;
  display: flex;
  gap: 16px;
}
.main-container {
  display: flex;
  flex: 1;
  overflow: hidden;
}
.sidebar {
  width: 320px;
  background: var(--bg-card);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}
.sidebar-header {
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.sidebar-title {
  font-weight: 600;
  font-size: 13px;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.session-list {
  flex: 1;
  overflow-y: auto;
  list-style: none;
}
.session-item {
  padding: 14px 16px;
  border-bottom: 1px solid var(--border);
  cursor: pointer;
  transition: background 0.15s ease;
}
.session-item:hover {
  background: var(--bg-card-hover);
}
.session-item.active {
  background: var(--accent-soft);
  border-left: 3px solid var(--accent);
}
.session-item-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}
.session-agent {
  font-weight: 600;
  color: var(--accent);
  font-size: 12px;
}
.session-time {
  font-size: 11px;
  color: var(--text-muted);
}
.session-branch {
  font-size: 12px;
  color: var(--text-secondary);
  font-family: var(--font-mono);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.session-stats {
  margin-top: 6px;
  font-size: 11px;
  color: var(--text-muted);
  display: flex;
  gap: 12px;
}
.content-area {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
}
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-muted);
  text-align: center;
}
.empty-state h3 {
  margin-bottom: 8px;
  color: var(--text-secondary);
}
.section-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.badge {
  display: inline-block;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
}
.badge-purple { background: rgba(188, 140, 255, 0.2); color: var(--purple); }
.badge-green { background: rgba(63, 185, 80, 0.2); color: var(--success); }
.badge-orange { background: rgba(210, 153, 34, 0.2); color: var(--warning); }
.card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}
.turn-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border);
}
.turn-title {
  font-weight: 600;
  font-size: 13px;
  color: var(--text);
}
.turn-prompt {
  background: rgba(255, 255, 255, 0.03);
  padding: 10px 12px;
  border-radius: 6px;
  margin-bottom: 12px;
  border-left: 3px solid var(--purple);
  font-size: 13px;
  white-space: pre-wrap;
}
.turn-response {
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.6;
  white-space: pre-wrap;
  margin-bottom: 12px;
}
.tool-call {
  background: var(--bg-dark);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 8px 12px;
  margin-top: 8px;
  font-family: var(--font-mono);
  font-size: 12px;
}
.tool-name {
  color: var(--accent);
  font-weight: 600;
}
.decision-card {
  border-left: 4px solid var(--accent);
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-left-width: 4px;
  border-radius: 6px;
  padding: 14px;
  margin-bottom: 12px;
}
.decision-type {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  color: var(--accent);
  margin-bottom: 4px;
}
.decision-text {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 6px;
}
.decision-rationale {
  font-size: 12px;
  color: var(--text-secondary);
  margin-bottom: 8px;
}
.decision-scope {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.scope-tag {
  background: var(--bg-dark);
  border: 1px solid var(--border);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-secondary);
}
</style>
</head>
<body>
<header>
  <div class="brand">
    <span>ScanDrix Trace</span>
    <span class="brand-badge">Local Cockpit</span>
  </div>
  <div class="header-meta">
    <span>Git Ref: scandrix/trace/v1</span>
    <span id="repo-key">Storage: ~/.scandrix/sessions</span>
    <span>Auto-Refresh: Active</span>
  </div>
</header>
<div class="main-container">
  <aside class="sidebar">
    <div class="sidebar-header">
      <span class="sidebar-title">Captured Sessions</span>
      <span id="session-count" class="badge badge-purple">0</span>
    </div>
    <ul class="session-list" id="session-list">
      <li class="session-item" style="color: var(--text-muted); text-align: center; padding: 24px;">Loading sessions...</li>
    </ul>
  </aside>
  <main class="content-area" id="content-area">
    <div class="empty-state">
      <h3>Select a session from the left rail</h3>
      <p>ScanDrix Trace continuously captures coding assistant turns, tool calls, and architectural decisions.</p>
    </div>
  </main>
</div>
<script>
var currentSessionId = null;

async function loadSessions() {
  try {
    var res = await fetch('/api/sessions');
    if (!res.ok) throw new Error('HTTP ' + res.status);
    var data = await res.json();
    var sessions = data.sessions || [];
    document.getElementById('session-count').textContent = sessions.length;
    
    var listEl = document.getElementById('session-list');
    if (sessions.length === 0) {
      listEl.innerHTML = '<li style="color: var(--text-muted); text-align: center; padding: 24px;">No sessions captured yet.<br><small>Run scandrix trace enable to install hooks.</small></li>';
      return;
    }

    listEl.innerHTML = sessions.map(function(s) {
      return '<li class="session-item ' + (s.sessionId === currentSessionId ? 'active' : '') + '" onclick="selectSession(\'' + s.sessionId + '\')">' +
        '<div class="session-item-header">' +
          '<span class="session-agent">' + escapeHtml(s.agentType || 'Agent') + '</span>' +
          '<span class="session-time">' + formatDate(s.startedAt) + '</span>' +
        '</div>' +
        '<div class="session-branch">' + escapeHtml(s.branch || 'main') + '</div>' +
        '<div class="session-stats">' +
          '<span>' + (s.turnCount || 0) + ' turns</span>' +
          '<span>' + ((s.filesTouched || []).length) + ' files</span>' +
        '</div>' +
      '</li>';
    }).join('');

    if (!currentSessionId && sessions.length > 0) {
      selectSession(sessions[0].sessionId);
    }
  } catch (err) {
    console.error('Failed loading sessions:', err);
  }
}

async function selectSession(sessionId) {
  currentSessionId = sessionId;
  document.querySelectorAll('.session-item').forEach(function(el) { el.classList.remove('active'); });
  
  var contentEl = document.getElementById('content-area');
  contentEl.innerHTML = '<div style="color: var(--text-muted); padding: 40px; text-align: center;">Loading session details...</div>';

  try {
    var res = await fetch('/api/sessions/' + encodeURIComponent(sessionId));
    if (!res.ok) throw new Error('HTTP ' + res.status);
    var data = await res.json();
    var session = data.session;
    var decisions = data.decisions || [];

    if (!session) {
      contentEl.innerHTML = '<div class="empty-state"><h3>Session not found</h3></div>';
      return;
    }

    var html = '<div style="margin-bottom: 24px; padding-bottom: 16px; border-bottom: 1px solid var(--border);">' +
      '<div style="display: flex; justify-content: space-between; align-items: flex-start;">' +
        '<div>' +
          '<h2 style="font-size: 20px; font-weight: 700; margin-bottom: 4px;">Session ' + escapeHtml(session.sessionId) + '</h2>' +
          '<div style="color: var(--text-secondary); font-size: 13px;">' +
            'Agent: <strong style="color: var(--accent);">' + escapeHtml(session.agentType || 'assistant') + '</strong> • ' +
            'Branch: <code style="color: var(--purple);">' + escapeHtml(session.branch || 'main') + '</code> • ' +
            'Started: ' + formatDate(session.startedAt) +
          '</div>' +
        '</div>' +
        '<span class="badge badge-green">' + ((session.turns || []).length) + ' Completed Turns</span>' +
      '</div>' +
    '</div>';

    if (decisions.length > 0) {
      html += '<div class="section-title">' +
        '<span>Architectural Decisions & Conventions</span>' +
        '<span class="badge badge-purple">' + decisions.length + '</span>' +
      '</div>' +
      '<div style="margin-bottom: 28px;">' +
        decisions.map(function(d) {
          var scopes = (d.scope || []).map(function(p) {
            return '<span class="scope-tag">' + escapeHtml(p) + '</span>';
          }).join('');
          return '<div class="decision-card">' +
            '<div class="decision-type">' + escapeHtml(d.type) + (d.pinned ? ' 📌 [PINNED]' : '') + '</div>' +
            '<div class="decision-text">' + escapeHtml(d.decision) + '</div>' +
            (d.rationale ? '<div class="decision-rationale">' + escapeHtml(d.rationale) + '</div>' : '') +
            '<div class="decision-scope">' + scopes + '</div>' +
          '</div>';
        }).join('') +
      '</div>';
    }

    html += '<div class="section-title">' +
      '<span>Session Turns & Interaction Trace</span>' +
      '<span class="badge badge-purple">' + ((session.turns || []).length) + '</span>' +
    '</div>';

    if (!session.turns || session.turns.length === 0) {
      html += '<p style="color: var(--text-muted);">No turns recorded for this session.</p>';
    } else {
      html += session.turns.map(function(t, idx) {
        var toolsHtml = '';
        if (t.toolCalls && t.toolCalls.length > 0) {
          toolsHtml = '<div style="margin-top: 10px;">' +
            '<strong style="font-size: 12px; color: var(--text-secondary);">Tools Executed (' + t.toolCalls.length + '):</strong>' +
            t.toolCalls.map(function(tc) {
              return '<div class="tool-call">' +
                '<span class="tool-name">' + escapeHtml(tc.toolName) + '</span>' +
                (tc.fileAffected ? ' <span style="color: var(--warning);">→ ' + escapeHtml(tc.fileAffected) + '</span>' : '') +
                (tc.summary ? '<div style="color: var(--text-muted); margin-top: 2px;">' + escapeHtml(tc.summary) + '</div>' : '') +
              '</div>';
            }).join('') +
          '</div>';
        }
        return '<div class="card">' +
          '<div class="turn-header">' +
            '<span class="turn-title">Turn #' + (idx + 1) + '</span>' +
            '<span style="font-size: 12px; color: var(--text-muted);">' + formatDate(t.startedAt) + '</span>' +
          '</div>' +
          (t.prompt ? '<div class="turn-prompt"><strong>Prompt:</strong> ' + escapeHtml(t.prompt) + '</div>' : '') +
          (t.response ? '<div class="turn-response">' + escapeHtml(t.response) + '</div>' : '') +
          toolsHtml +
        '</div>';
      }).join('');
    }

    contentEl.innerHTML = html;
  } catch (err) {
    contentEl.innerHTML = '<div style="color: var(--danger); padding: 40px;">Failed loading session: ' + escapeHtml(err.message) + '</div>';
  }
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function formatDate(iso) {
  if (!iso) return '';
  var d = new Date(iso);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) + ' ' + d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

loadSessions();
setInterval(loadSessions, 8000);
</script>
</body>
</html>`
}
