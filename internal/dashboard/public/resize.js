(() => {
  if (parent === window) return;
  let queued = false;
  function measure() {
    if (queued) return;
    queued = true;
    requestAnimationFrame(() => {
      queued = false;
      parent.postMessage({ type: 'codex:height', height: Math.ceil(document.body.getBoundingClientRect().height) }, '*');
    });
  }
  new ResizeObserver(measure).observe(document.body);
  addEventListener('message', event => {
    if (event.source === parent && event.data?.type === 'codex:measure') measure();
  });
  document.fonts.ready.then(measure);
  measure();
})();
