// ═══════════════════════════════════════════════════════════════
// 1. APPLICATION BOOTSTRAP (DOM Ready)
// ═══════════════════════════════════════════════════════════════
document.addEventListener('DOMContentLoaded', () => {
  initInstallerTabs();
  initCopyCommand();
});

// ═══════════════════════════════════════════════════════════════
// 2. INSTALLER TABS (OS detection and command rendering)
// ═══════════════════════════════════════════════════════════════
function initInstallerTabs() {
  const origin = (window.location.origin && window.location.origin !== 'null') 
    ? window.location.origin 
    : 'https://get.scandrix.dev';

  const commands = {
    unix: `curl -fsSL ${origin}/install | sh`,
    windows: `powershell -NoProfile -ExecutionPolicy Bypass -Command "$tmp = Join-Path $env:TEMP 'scandrix-install.ps1'; Invoke-WebRequest ${origin}/install.ps1 -OutFile $tmp; & $tmp"`,
  };

  let currentPlatform = /Windows/i.test(navigator.userAgent) ? 'windows' : 'unix';

  const installCodeEl = document.getElementById('install-command');
  const tabs = document.querySelectorAll('.tab');

  function renderInstall() {
    if (installCodeEl) {
      installCodeEl.textContent = commands[currentPlatform];
    }
    tabs.forEach((button) => {
      button.classList.toggle('active', button.dataset.platform === currentPlatform);
    });
  }

  tabs.forEach((button) => {
    button.addEventListener('click', () => {
      currentPlatform = button.dataset.platform;
      renderInstall();
    });
  });

  renderInstall();
}

// ═══════════════════════════════════════════════════════════════
// 3. COPY COMMAND BUTTON (Clipboard API with visual feedback)
// ═══════════════════════════════════════════════════════════════
function initCopyCommand() {
  const button = document.getElementById('copyInstallBtn');
  const installCodeEl = document.getElementById('install-command');

  if (!button || !installCodeEl) return;

  button.addEventListener('click', async () => {
    const textToCopy = installCodeEl.textContent.trim();
    if (!textToCopy) return;

    try {
      await navigator.clipboard.writeText(textToCopy);
      button.classList.add('copied');
      button.innerHTML = `
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      `;

      setTimeout(() => {
        button.classList.remove('copied');
        button.innerHTML = `
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
          </svg>
        `;
      }, 2000);
    } catch (err) {
      console.error('Failed to copy install command', err);
    }
  });
}
