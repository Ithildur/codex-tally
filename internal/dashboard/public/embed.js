(() => {
  const installed = Symbol.for('codex.usage.embed');
  if (window[installed]) removeEventListener('message', window[installed]);
  const resize = event => {
    // Public frames have opaque origins because their sandbox deliberately
    // excludes allow-same-origin. Validate the exact source window instead.
    if (event.origin !== 'null' || event.data?.type !== 'codex:height') return;
    const height = event.data.height;
    if (!Number.isInteger(height) || height < 1 || height > 50000) return;
    for (const frame of document.querySelectorAll('iframe[data-codex-usage]')) {
      if (event.source === frame.contentWindow) {
        frame.style.height = `${height}px`;
        break;
      }
    }
  };
  window[installed] = resize;
  addEventListener('message', resize);
  // Also handles scripts loaded after their iframe, and multiple embeds.
  for (const frame of document.querySelectorAll('iframe[data-codex-usage]')) {
    frame.contentWindow?.postMessage({ type: 'codex:measure' }, '*');
  }
})();
