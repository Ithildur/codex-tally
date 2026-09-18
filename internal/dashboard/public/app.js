(() => {
  let preference = 'system';
  try {
    const saved = localStorage.getItem('codex-dashboard-theme');
    if (saved === 'light' || saved === 'dark') preference = saved;
  } catch { /* Storage may be disabled; the switch still works for this page. */ }
  const theme = preference === 'system' ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : preference;
  document.documentElement.dataset.themePreference = preference;
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]').content = theme === 'light' ? '#f8f9fb' : '#111214';
})();

document.addEventListener('DOMContentLoaded', () => {
const $ = id => document.getElementById(id);
const esc = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
const full = value => value == null ? '—' : new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 }).format(value);
const compact = value => value == null ? '—' : new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 2 }).format(value);
const money = (value, currency = 'USD') => new Intl.NumberFormat('zh-CN', { style: 'currency', currency, currencyDisplay: 'narrowSymbol', maximumFractionDigits: 2 }).format(value);
const finite = value => typeof value === 'number' && Number.isFinite(value);
const text = (id, value) => { $(id).textContent = value; };
const systemTheme = matchMedia('(prefers-color-scheme: dark)');
let themePreference = document.documentElement.dataset.themePreference || 'system';
function setTheme(preference) {
  themePreference = ['light', 'dark'].includes(preference) ? preference : 'system';
  const theme = themePreference === 'system' ? (systemTheme.matches ? 'dark' : 'light') : themePreference;
  document.documentElement.dataset.themePreference = themePreference;
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]').content = theme === 'light' ? '#f8f9fb' : '#111214';
  const labels = { system: '系统', light: '浅色', dark: '深色' };
  text('theme-toggle', labels[themePreference]);
  $('theme-toggle').setAttribute('aria-label', `主题：${themePreference === 'system' ? '跟随系统' : labels[themePreference]}`);
  for (const button of $('theme-options').querySelectorAll('button')) button.setAttribute('aria-pressed', String(button.dataset.themeChoice === themePreference));
}
setTheme(themePreference);
systemTheme.addEventListener('change', () => { if (themePreference === 'system') setTheme('system'); });
function closeThemes() {
  $('theme-options').hidden = true;
  $('theme-toggle').setAttribute('aria-expanded', 'false');
}
$('theme-toggle').addEventListener('click', () => {
  const opening = $('theme-options').hidden;
  $('theme-options').hidden = !opening;
  $('theme-toggle').setAttribute('aria-expanded', String(opening));
  if (opening) $('theme-options').querySelector('[aria-pressed="true"]').focus();
});
$('theme-options').addEventListener('click', event => {
  const button = event.target.closest('button[data-theme-choice]');
  if (!button) return;
  setTheme(button.dataset.themeChoice);
  try { localStorage.setItem('codex-dashboard-theme', themePreference); } catch { /* Page-only preference. */ }
  closeThemes(); $('theme-toggle').focus();
});
document.addEventListener('click', event => { if (!event.target.closest('.theme-control')) closeThemes(); });
document.addEventListener('focusin', event => { if (!event.target.closest('.theme-control')) closeThemes(); });
document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && !$('theme-options').hidden) { closeThemes(); $('theme-toggle').focus(); }
});
window.addEventListener('storage', event => {
  if (event.key === 'codex-dashboard-theme' || event.key === null) setTheme(event.newValue);
});
let timezone = 'Local';
let sequence = 0;
let activeRequest;
let breakdown;
let tab = 'local';
const tabs = ['local', 'account', 'calculator'];
function tabControl(id, source = tab) {
  return $(source === 'local' ? id : `account-${id}`);
}
let refreshTimer;
let quotaTimer;
let refreshAt = Infinity, quotaAt = Infinity;
const retryDelay = 30 * 1000;
const accountTTL = 30 * 60 * 1000;
function remoteDelay(result) {
  return finite(result?.refreshAfterMs) && result.refreshAfterMs > 0 ? Math.max(1000, result.refreshAfterMs) : result?.error ? retryDelay : accountTTL;
}
function armRefreshTimers() {
  clearTimeout(refreshTimer); clearTimeout(quotaTimer);
  if (document.hidden || $('dashboard').hidden) return;
  if (Number.isFinite(refreshAt)) refreshTimer = setTimeout(() => load(false), Math.max(0, refreshAt - Date.now()));
  if (Number.isFinite(quotaAt)) quotaTimer = setTimeout(() => loadLimits(), Math.max(0, quotaAt - Date.now()));
}
document.addEventListener('visibilitychange', armRefreshTimers);
let shareRequest;
let sharePreviewURL = '';
let shareLayout;
let shareFrame;
function fitSharePreview() {
  if (!shareLayout || !shareFrame) return;
  const preview = $('share-preview'), available = Math.max(1, preview.clientWidth - 2);
  const narrow = available < 500;
  const width = narrow ? 320 : shareLayout.width;
  const height = narrow ? shareLayout.narrowHeight : shareLayout.height;
  const scale = Math.min(1, available / width);
  shareFrame.style.width = `${width}px`; shareFrame.style.height = `${height}px`;
  shareFrame.style.transform = `scale(${scale})`;
  preview.style.height = `${Math.ceil(height * scale) + 2}px`;
}
new ResizeObserver(fitSharePreview).observe($('share-preview'));
function shareEmbed(page, title, layout) {
  const script = new URL('/share/embed.js', page);
  return `<iframe data-codex-usage src="${esc(page)}" title="${esc(title)}" width="${layout.width}" height="${layout.height}" loading="lazy" style="display:block;width:100%;max-width:${layout.width}px;border:0"></iframe><script async src="${esc(script.href)}"></script>`;
}
$('share-link').addEventListener('click', () => {
  closeThemes(); closeQuota();
  $('share-hub').showModal();
  updateShare();
});
$('share-close').addEventListener('click', () => $('share-hub').close());
$('share-hub').addEventListener('click', event => {
  const bounds = event.currentTarget.getBoundingClientRect();
  if (event.target === event.currentTarget && (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom)) event.currentTarget.close();
});
$('share-hub').addEventListener('close', () => {
  shareRequest?.abort(); shareRequest = null; sharePreviewURL = '';
  shareLayout = null; shareFrame = null; $('share-preview').style.height = '';
  $('share-preview').replaceChildren();
});
$('share-options').addEventListener('submit', event => event.preventDefault());
$('share-options').addEventListener('change', updateShare);
async function updateShare() {
  const component = $('share-component').value, theme = $('share-theme').value, format = $('share-format').value;
  const selected = [...$('share-hub-components').querySelectorAll('input:checked')].map(input => input.value);
  $('share-hub-components').hidden = component !== 'hub';
  $('share-copy').disabled = false; $('share-open').hidden = false;
  if (component === 'hub' && !selected.length) {
    shareRequest?.abort(); sharePreviewURL = '';
    shareLayout = null; shareFrame = null; $('share-preview').style.height = '';
    $('share-preview').replaceChildren(); $('share-preview').setAttribute('aria-busy', 'false');
    $('share-code').value = ''; $('share-copy').disabled = true; $('share-open').hidden = true;
    text('share-copy-status', ''); text('share-preview-error', '至少选择一个组件');
    return;
  }
  const title = `Codex ${$('share-component').selectedOptions[0].textContent}`;
  const page = new URL(`/share/${component}`, window.location.origin);
  if (component === 'hub') page.searchParams.set('components', selected.join(','));
  page.searchParams.set('theme', theme);
  const svg = new URL(page);
  svg.pathname += '.svg';
  const isImage = format === 'svg' || format === 'markdown';
  const previewURL = (isImage ? svg : page).href;
  const codes = {
    web: page.href,
    svg: svg.href,
    markdown: `![${title}](<${svg.href}>)`,
    iframe: shareLayout ? shareEmbed(page.href, title, shareLayout) : '',
  };
  $('share-code').value = codes[format];
  text('share-copy', format === 'iframe' || format === 'markdown' ? '复制代码' : '复制链接');
  text('share-copy-status', '');
  text('share-preview-label', isImage ? 'SVG 预览' : '网页预览');
  $('share-open').href = previewURL;
  if (previewURL === sharePreviewURL) { $('share-copy').disabled = !$('share-code').value; return; }
  sharePreviewURL = previewURL;
  shareLayout = null; shareFrame = null;
  if (format === 'iframe') { $('share-code').value = ''; $('share-copy').disabled = true; }
  shareRequest?.abort();
  const request = shareRequest = new AbortController();
  const preview = $('share-preview');
  preview.style.height = '';
  preview.setAttribute('aria-busy', 'true');
  preview.innerHTML = '<span class="loading">加载预览…</span>';
  text('share-preview-error', '');
  try {
    const response = await fetch(previewURL, { credentials: 'omit', signal: request.signal });
    if (!response.ok) throw new Error(response.status === 503 ? '公开快照尚未生成，请稍后重新打开。' : response.status === 404 ? '公开展示未启用。' : '预览加载失败，请稍后重新打开。');
    if (request.signal.aborted) return;
    const media = document.createElement(isImage ? 'img' : 'iframe');
    if (isImage) media.alt = title;
    else { media.title = `${title}网页预览`; media.setAttribute('sandbox', 'allow-same-origin'); }
    media.addEventListener('load', () => {
      if (request.signal.aborted) return;
      if (!isImage) {
        const count = selected.filter(key => key !== 'models').length;
        const width = component === 'hub' ? Math.max(selected.includes('models') ? 720 : 480, count * 280 + 72) : component === 'overview' ? 960 : component === 'models' ? 720 : 480;
        const measure = width => {
          media.style.width = `${width}px`; media.style.height = '1px';
          return Math.ceil(media.contentDocument.documentElement.scrollHeight) + 2;
        };
        const narrowHeight = Math.max(measure(320), measure(700));
        shareLayout = { width, height: measure(width), narrowHeight };
        shareFrame = media;
        fitSharePreview();
        if ($('share-format').value === 'iframe') {
          $('share-code').value = shareEmbed(page.href, title, shareLayout);
          $('share-copy').disabled = false;
        }
      }
      preview.setAttribute('aria-busy', 'false');
    });
    media.addEventListener('error', () => {
      if (request.signal.aborted) return;
      preview.setAttribute('aria-busy', 'false');
      text('share-preview-error', '预览加载失败，链接仍可复制。');
    });
    if (isImage) media.src = previewURL;
    else {
      // Scripts remain disabled in this measuring frame. Exported pages run
      // only their hash-allowlisted resize script, without same-origin access.
      const html = await response.text();
      if (request.signal.aborted) return;
      const document = new DOMParser().parseFromString(html, 'text/html');
      document.querySelectorAll('script').forEach(script => script.remove());
      media.srcdoc = `<!doctype html>${document.documentElement.outerHTML}`;
    }
    preview.replaceChildren(media);
  } catch (error) {
    if (request.signal.aborted) return;
    preview.replaceChildren(); preview.setAttribute('aria-busy', 'false');
    text('share-preview-error', error.message || '预览加载失败，请稍后重新打开。');
  }
}
$('share-copy').addEventListener('click', async () => {
  const button = $('share-copy'), field = $('share-code'), value = field.value;
  button.disabled = true;
  try {
    await navigator.clipboard.writeText(value);
    if (field.value === value) text('share-copy-status', '已复制');
  } catch {
    if (field.value !== value || !$('share-hub').open) return;
    field.focus(); field.select();
    let copied = false;
    try { copied = document.execCommand('copy'); } catch { /* Leave the text selected for manual copying. */ }
    text('share-copy-status', copied ? '已复制' : '内容已选中，请手动复制');
  } finally { button.disabled = !$('share-code').value; }
});
let quotaRequest;
function closeQuota() {
  $('quota-popover').hidden = true;
  $('quota-toggle').setAttribute('aria-expanded', 'false');
}
$('quota-toggle').addEventListener('click', () => {
  const open = $('quota-popover').hidden;
  closeThemes();
  $('quota-popover').hidden = !open;
  $('quota-toggle').setAttribute('aria-expanded', String(open));
  if (open) loadLimits(true);
});
$('quota-refresh').addEventListener('click', () => loadLimits(true));
document.addEventListener('click', event => { if (!event.target.closest('.quota-control')) closeQuota(); });
document.addEventListener('focusin', event => { if (!event.target.closest('.quota-control')) closeQuota(); });
document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && !$('quota-popover').hidden) { closeQuota(); $('quota-toggle').focus(); }
});
async function loadLimits(refresh = false) {
  if (quotaRequest) return;
  clearTimeout(quotaTimer); quotaAt = Infinity;
  let nextDelay = retryDelay;
  const request = quotaRequest = new AbortController();
  $('quota-refresh').disabled = true;
  try {
    const result = await api(`/api/limits${refresh ? '?refresh=1' : ''}`, { signal: request.signal });
    if (!request.signal.aborted) { nextDelay = remoteDelay(result); renderLimits(result); }
  } catch (error) {
    if (error.name !== 'AbortError') text('limit-error', error.message);
  } finally {
    if (quotaRequest === request) {
      quotaRequest = null; $('quota-refresh').disabled = false;
      quotaAt = Date.now() + nextDelay; armRefreshTimers();
    }
  }
}
const views = new Map();
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');
reducedMotion.addEventListener('change', () => {
  for (const element of document.querySelectorAll('.split-number')) {
    const value = element.dataset.value;
    delete element.dataset.value;
    number(element, value);
  }
});

function number(id, value) {
  const element = typeof id === 'string' ? $(id) : id, next = String(value), previous = element.dataset.value || '';
  if (next === element.dataset.value) return;
  element.dataset.value = next;
  element.setAttribute('aria-label', next);
  if (element.tagName !== 'DD') element.setAttribute('role', 'img');
  element.classList.add('split-number');
  element.style.setProperty('--digits', Math.max(1, next.length));
  const old = previous.padStart(next.length, ' ');
  const animate = previous && previous !== '—' && next !== '—' && !reducedMotion.matches;
  const glyph = element.hasAttribute('data-flap-label')
    ? char => `<svg viewBox="0 0 112 130" aria-hidden="true" focusable="false"><text x="56" y="65" text-anchor="middle" dominant-baseline="central" font-size="96" fill="currentColor">${esc(char)}</text></svg>`
    : esc;
  element.innerHTML = [...next].map((char, i) => {
    const before = old[i] || ' ', changed = animate && char !== before;
    const width = /[\u2e80-\uffef]/u.test(char) ? 'flap-wide' : char === ' ' ? 'flap-space' : '';
    return `<span class="flap ${width} ${changed ? 'flipping' : ''}" aria-hidden="true" style="--delay:${Math.min(i * 22, 180)}ms"><span class="flap-top"><span>${glyph(char)}</span></span><span class="flap-bottom"><span>${glyph(changed ? before : char)}</span></span>${changed ? `<span class="flap-old"><span>${glyph(before)}</span></span><span class="flap-new"><span>${glyph(char)}</span></span>` : ''}</span>`;
  }).join('');
  for (const flap of element.querySelectorAll('.flipping')) {
    flap.addEventListener('animationend', event => {
      if (!event.target.classList.contains('flap-new')) return;
      flap.querySelector('.flap-bottom').innerHTML = flap.querySelector('.flap-new').innerHTML;
      flap.querySelector('.flap-old').remove(); flap.querySelector('.flap-new').remove(); flap.classList.remove('flipping');
    });
  }
}

for (const element of document.querySelectorAll('[data-flap-label]')) number(element, element.textContent);

function timestamp(value) {
  return value ? new Date(value).toLocaleString('zh-CN', { timeZone: timezone === 'Local' ? undefined : timezone, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) : '尚无数据';
}
async function api(path, options = {}) {
  const response = await fetch(path, { credentials: 'same-origin', ...options });
  const data = await response.json();
  if (response.status === 401 && path !== '/api/login') { showLogin(); throw new Error('登录已过期，请重新登录'); }
  if (!response.ok) throw new Error(data.error || '请求失败，请重试');
  return data;
}
function showLogin() {
  if ($('share-hub').open) $('share-hub').close();
  sequence++; activeRequest?.abort();
  quotaRequest?.abort(); quotaRequest = null; closeQuota();
  text('quota-five', '—'); text('quota-week', '—'); text('quota-updated', ''); text('limit-error', '');
  text('quota-resets', '—'); text('reset-updated', ''); text('reset-error', ''); $('reset-credits').replaceChildren();
  $('plan').hidden = true;
  $('quota-five-summary').hidden = true;
  $('limits').innerHTML = '<p class="loading">正在读取额度…</p>';
  clearTimeout(refreshTimer); clearTimeout(quotaTimer);
  refreshAt = quotaAt = Infinity; views.clear();
  resetPanel('local'); resetPanel('account');
  calculatorRequest?.abort(); calculatorPricesRequest?.abort();
  clearCalculatorUsage();
  $('dashboard').hidden = true; $('login').hidden = false; $('boot').hidden = true;
  $('password').value = ''; $('password').focus();
}
function showDashboard() {
  $('login').hidden = true; $('boot').hidden = true; $('dashboard').hidden = false;
  selectTab(tab, false); load(false, true);
  loadLimits();
}
function selectTab(next, fetchData = true) {
  tab = next;
  for (const source of tabs) {
    const selected = source === tab;
    $(`tab-${source}`).setAttribute('aria-selected', String(selected));
    $(`tab-${source}`).tabIndex = selected ? 0 : -1;
    $(`panel-${source}`).hidden = !selected;
  }
  if (tab === 'calculator') {
    activeRequest?.abort(); sequence++; clearTimeout(refreshTimer); refreshAt = Infinity;
    const address = new URL(location.href); address.searchParams.set('tab', tab); history.replaceState(null, '', address);
    loadCalculatorPrices();
    return;
  }
  if (fetchData) load(false, true);
}
for (const source of tabs) {
  $(`tab-${source}`).addEventListener('click', () => selectTab(source));
  $(`tab-${source}`).addEventListener('keydown', event => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault(); const next = event.key === 'Home' ? tabs[0] : event.key === 'End' ? tabs.at(-1) : tabs[(tabs.indexOf(source) + (event.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length];
    selectTab(next); $(`tab-${next}`).focus();
  });
}
$('login-form').addEventListener('submit', async event => {
  event.preventDefault();
  const button = event.currentTarget.querySelector('button');
  button.disabled = true; text('login-error', '');
  try {
    await api('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password: $('password').value }) });
    $('password').value = ''; showDashboard();
  } catch (error) { text('login-error', error.message); }
  finally { button.disabled = false; }
});
$('logout').addEventListener('click', async () => {
  try { await api('/api/logout', { method: 'POST' }); showLogin(); }
  catch (error) { text('local-error', error.message); }
});
function defaultEnd(type) {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: timezone === 'Local' ? undefined : timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).formatToParts(new Date());
  const p = Object.fromEntries(parts.map(part => [part.type, part.value]));
  return `${p.year}-${p.month}-${p.day}${type === 'datetime-local' ? `T${p.hour}:${p.minute}` : ''}`;
}
function configurePeriod(source = tab) {
  const period = tabControl('period', source), end = tabControl('window-end', source);
  const custom = period.value.startsWith('custom');
  tabControl('custom-control', source).hidden = !custom;
  if (custom) {
    end.type = period.value === 'custom5' ? 'datetime-local' : 'date';
    end.value = defaultEnd(end.type);
  }
}
for (const source of ['local', 'account']) {
  tabControl('period', source).addEventListener('change', () => { configurePeriod(source); load(false, true); });
  tabControl('period-form', source).addEventListener('submit', event => { event.preventDefault(); load(false, true); });
  tabControl('refresh', source).addEventListener('click', () => load(true));
}
$('breakdown-date').addEventListener('change', renderBreakdown);
$('heatmap').addEventListener('click', event => {
  const cell = event.target.closest('button[data-label]');
  if (!cell) return;
  for (const button of $('heatmap').querySelectorAll('button')) button.tabIndex = button === cell ? 0 : -1;
  text('busiest', cell.dataset.label);
});
$('heatmap').addEventListener('keydown', event => {
  const offsets = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -24, ArrowDown: 24 };
  if (!(event.key in offsets)) return;
  const cells = [...$('heatmap').querySelectorAll('button')], current = cells.indexOf(event.target);
  if (current < 0) return;
  event.preventDefault();
  const next = cells[Math.max(0, Math.min(cells.length - 1, current + offsets[event.key]))];
  next.focus(); next.click();
});

function resetPanel(source) {
  tabControl('range', source).textContent = '';
  if (source === 'local') {
    for (const id of ['total-tokens', 'total-cost', 'total-calls', 'cache-rate', 'session-detail', 'cache-detail', 'output-tokens', 'input-tokens', 'cost-detail']) { delete $(id).dataset.value; number(id, id === 'cost-detail' ? '' : '—'); }
    for (const id of ['habit-summary', 'busiest', 'local-error']) text(id, '');
    $('cost-detail').hidden = true;
    $('heatmap').replaceChildren();
    $('models').innerHTML = '<p class="empty">正在统计…</p>';
  } else {
    delete $('cloud-total').dataset.value; number('cloud-total', '—');
    for (const id of ['cloud-range', 'cloud-updated', 'cloud-error', 'breakdown-error']) text(id, '');
    for (const id of ['daily-chart', 'daily-table', 'breakdown-table', 'breakdown-date']) $(id).replaceChildren();
    $('clients').innerHTML = '<p class="empty">正在统计…</p>';
    breakdown = null;
  }
}
async function load(refresh, viewChanged = false) {
  if (tab === 'calculator') return;
  if (refresh) loadLimits(true);
  clearTimeout(refreshTimer); refreshAt = Infinity;
  let nextDelay = retryDelay;
  activeRequest?.abort(); activeRequest = new AbortController();
  const signal = activeRequest.signal, current = ++sequence;
  const source = tab;
  const params = new URLSearchParams({ period: tabControl('period', source).value });
  if (tabControl('period', source).value.startsWith('custom')) params.set('end', tabControl('window-end', source).value);
  const viewKey = `${source}:${params}`;
  let rendered = false;
  if (viewChanged && views.has(viewKey)) render(views.get(viewKey));
  const address = new URL(location.href);
  address.search = params.toString(); address.searchParams.set('tab', source); history.replaceState(null, '', address);
  tabControl('refresh', source).disabled = true; tabControl('refresh', source).lastChild.textContent = ' 刷新中';
  const section = $(source === 'local' ? 'local-section' : 'cloud-section'); section.setAttribute('aria-busy', 'true');
  function render(data) {
    if (viewChanged && !rendered && source === 'account' && !data.counts?.data) resetPanel(source);
    if (source === 'local') renderLocal(data); else renderAccount(data);
    if (data.range) { timezone = data.range.timezone; tabControl('range', source).textContent = `${timestamp(data.range.from)} 至 ${timestamp(data.range.to)}`; tabControl('range', source).title = timezone; }
    rendered = true;
  }
  function receive(data) {
    if (current !== sequence) return false;
    nextDelay = source === 'local' ? (data.error ? retryDelay : 60 * 60 * 1000) : data.error ? retryDelay : Math.min(remoteDelay(data.counts), remoteDelay(data.breakdown));
    views.set(viewKey, data); if (views.size > 24) views.delete(views.keys().next().value);
    render(data); return true;
  }
  try {
    if (source === 'local' && !refresh) {
      const cached = await api(`/api/local?${params}`, { signal });
      if (!receive(cached)) return;
      // Paint the cached numbers before starting the one-off revalidation.
      await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    }
    if (source === 'local' || refresh) params.set('refresh', '1');
    const fresh = await api(`/api/${source}?${params}`, { signal });
    receive(fresh);
  } catch (error) {
    if (current !== sequence || error.name === 'AbortError') return;
    nextDelay = retryDelay;
    if (viewChanged && !rendered) resetPanel(source);
    text(source === 'local' ? 'local-error' : 'cloud-error', error.message);
  } finally {
    if (current === sequence || source !== tab) {
      section.setAttribute('aria-busy', 'false');
      tabControl('refresh', source).disabled = false; tabControl('refresh', source).lastChild.textContent = ' 刷新';
    }
    if (current === sequence) { refreshAt = Date.now() + nextDelay; armRefreshTimers(); }
  }
}

function renderLocal(data) {
  timezone = data.range.timezone;
  number('total-tokens', compact(data.tokens)); $('total-tokens').title = `${full(data.tokens)} Token`;
  number('total-calls', full(data.calls));
  number('cache-rate', data.cacheRate == null ? '—' : `${data.cacheRate.toFixed(1)}%`);
  number('total-cost', data.cost == null ? '—' : money(data.cost, data.currency.code));
  number('output-tokens', compact(data.output));
  number('input-tokens', compact(data.input + data.cached + data.write));
  number('session-detail', full(data.sessions));
  number('cache-detail', compact(data.cached));
  $('cost-detail').hidden = !data.unpricedCalls;
  number('cost-detail', data.unpricedCalls ? `${full(data.unpricedCalls)} 次未计价` : '');
  text('local-error', data.error || (data.unreadable || data.malformed ? `${data.unreadable} 个文件、${data.malformed} 条记录不完整` : data.files === 0 ? '未发现本机会话记录' : ''));
  text('habit-summary', `${data.activeDays} 个活跃日`);
  text('timezone', timezone);
  text('busiest', data.busiestHour == null ? '暂无记录' : `活跃高峰 ${String(data.busiestHour).padStart(2, '0')}:00-${String(data.busiestHour + 1).padStart(2, '0')}:00`);
  const weekdays = ['一', '二', '三', '四', '五', '六', '日'];
  const max = Math.max(1, ...data.hours.flat());
  const colors = ['var(--heat-0)', 'var(--heat-1)', 'var(--heat-2)', 'var(--heat-3)', 'var(--heat-4)'];
  $('heatmap').innerHTML = '<span></span>' + Array.from({ length: 24 }, (_, i) => `<span class="heat-label">${i % 6 === 0 ? String(i).padStart(2, '0') : ''}</span>`).join('') + data.hours.map((hours, day) => `<span class="heat-label">${weekdays[day]}</span>${hours.map((count, hour) => {
    const label = `周${weekdays[day]} ${String(hour).padStart(2, '0')}:00，${full(count)} 次调用`;
    return `<button type="button" class="heat-cell" tabindex="${day === 0 && hour === 0 ? 0 : -1}" aria-label="${esc(label)}" title="${esc(label)}" data-label="${esc(label)}" style="background:${colors[count ? Math.max(1, Math.ceil(count / max * 4)) : 0]}"></button>`;
  }).join('')}`).join('');
  distribution('models', data.models.map(row => ({ label: row.model, value: row.tokens, suffix: `${compact(row.tokens)} · ${full(row.calls)} 次` })));
}
function distribution(id, rows) {
  if (!rows.length) { $(id).innerHTML = '<p class="empty">所选范围内暂无记录</p>'; return; }
  const max = Math.max(1, ...rows.map(row => row.value || 0));
  $(id).innerHTML = rows.map(row => {
    const values = String(row.suffix ?? full(row.value)).split(' · ');
    return `<div class="distribution-row"><div class="distribution-label"><span>${esc(row.label)}</span><span class="value"><span>${esc(values[0])}</span>${values[1] ? `<small>${esc(values[1])}</small>` : ''}</span></div><div class="distribution-track"><span style="width:${finite(row.value) ? Math.max(0, Math.min(100, row.value / max * 100)) : 0}%"></span></div></div>`;
  }).join('');
}

function renderAccount(account) {
  if (account.error) {
    text('cloud-error', account.error); return;
  }
  timezone = account.range.timezone;
  text('cloud-error', account.counts.error || '');
  if (account.counts.data) renderCounts(account.counts, account.range);
  else { $('clients').innerHTML = '<p class="empty">客户端统计暂不可用</p>'; }
  text('breakdown-error', account.breakdown.error || '');
  breakdown = account.breakdown.data;
  const previousDate = $('breakdown-date').value;
  $('breakdown-date').innerHTML = (breakdown?.data || []).map(row => `<option value="${esc(row.date)}">${esc(row.date)}</option>`).reverse().join('');
  if (breakdown?.data.some(row => row.date === previousDate)) $('breakdown-date').value = previousDate;
  renderBreakdown();
}
function renderLimits(result) {
  text('limit-error', result.error || '');
  renderResetCredits(result);
  const plan = result.data?.plan || '';
  $('plan').hidden = !plan; text('plan', plan.toUpperCase());
  $('quota-five-summary').hidden = !plan || plan.toLowerCase() === 'pro';
  const codex = result.data?.buckets.find(bucket => bucket.name === 'Codex');
  for (const [value, seconds] of [['quota5', 18000], ['quota7', 604800]]) {
    const option = $('period').querySelector(`option[value="${value}"]`);
    const window = [codex?.primary, codex?.secondary].find(window => window?.seconds === seconds && window.resetsAt);
    option.hidden = option.disabled = !window || (value === 'quota5' && plan.toLowerCase().startsWith('pro'));
    if (option.disabled && $('period').value === value) {
      $('period').value = 'week'; configurePeriod('local');
      if (tab === 'local') load(false, true);
    }
  }
  for (const [id, seconds] of [['quota-five', 18000], ['quota-week', 604800]]) {
    const window = [codex?.primary, codex?.secondary].find(window => window?.seconds === seconds);
    text(id, window ? `${full(Math.max(0, Math.min(100, 100 - window.usedPercent)))}%` : '—');
    $(id).title = window ? '剩余额度' : result.data ? '接口未提供此窗口' : '额度暂不可用';
  }
  if (!result.data) { $('plan').hidden = true; $('limits').innerHTML = '<p class="empty">额度暂不可用</p>'; text('quota-updated', '尚无数据'); return; }
  text('quota-updated', `${result.cached ? '缓存于' : '更新于'} ${timestamp(result.fetchedAt)}`);
  const windows = result.data.buckets.flatMap(bucket => [bucket.primary, bucket.secondary].filter(Boolean).map(window => ({ ...window, name: bucket.name })));
  $('limits').innerHTML = windows.length ? windows.map(window => {
    const duration = window.seconds == null ? '额度窗口' : window.seconds % 86400 === 0 ? `${window.seconds / 86400} 天` : `${Number((window.seconds / 3600).toFixed(1))} 小时`;
    const percent = Math.max(0, Math.min(100, 100 - window.usedPercent));
    return `<article class="limit-card"><h3>${esc(window.name)}</h3><div class="limit-top"><span>${duration}窗口</span><strong>${full(percent)}<small>%</small></strong></div><div class="progress ${percent <= 20 ? 'warning' : ''}" role="progressbar" aria-label="${esc(window.name)} ${duration}剩余额度" aria-valuenow="${percent}" aria-valuemin="0" aria-valuemax="100"><span style="width:${percent}%"></span></div><p class="metadata">${window.resetsAt ? `${timestamp(window.resetsAt * 1000)} 重置` : '未提供重置时间'}</p></article>`;
  }).join('') : '<p class="empty">账号未返回额度窗口</p>';
}

function renderResetCredits(result) {
  const details = result.resetCredits, data = details?.data;
  const count = details?.error ? result.data?.resetCount ?? data?.availableCount : data?.availableCount ?? result.data?.resetCount;
  text('quota-resets', count == null ? '—' : full(count));
  $('quota-resets').title = '剩余重置次数';
  text('reset-error', details?.error || '');
  text('reset-updated', details?.fetchedAt ? `${details.cached ? '缓存于' : '更新于'} ${timestamp(details.fetchedAt)}` : '');
  const credits = data?.credits;
  if (!credits?.length) {
    $('reset-credits').innerHTML = `<li><span>${count === 0 ? '暂无剩余重置' : '过期时间暂不可用'}</span></li>`;
    return;
  }
  const format = new Intl.DateTimeFormat('zh-CN', { timeZone: timezone === 'Local' ? undefined : timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
  const sorted = credits.toSorted((a, b) => (a.expiresAt ? Date.parse(a.expiresAt) : Infinity) - (b.expiresAt ? Date.parse(b.expiresAt) : Infinity));
  $('reset-credits').innerHTML = sorted.map((credit, i) => `<li><span>重置 ${i + 1}</span>${credit.expiresAt ? `<time datetime="${esc(credit.expiresAt)}" title="${esc(timezone)}">${esc(format.format(new Date(credit.expiresAt)))} 到期</time>` : '<span>未提供过期时间</span>'}</li>`).join('');
  if (count > credits.length) $('reset-credits').insertAdjacentHTML('beforeend', `<li><span>另 ${full(count - credits.length)} 次未返回明细</span></li>`);
}

function renderCounts(result, range) {
  const rows = result.data.data;
  const known = rows.filter(row => finite(row.totals.text_total_tokens));
  const total = known.reduce((sum, row) => sum + row.totals.text_total_tokens, 0);
  number('cloud-total', known.length ? compact(total) : '—');
  text('cloud-range', `${range.startDate.slice(5)} 至 ${range.endDate.slice(5)}`);
  text('cloud-updated', `${result.cached ? '缓存于' : '更新于'} ${timestamp(result.fetchedAt)}`);
  const byDate = new Map(rows.map(row => [row.date, row]));
  const dates = [];
  for (let day = new Date(`${range.startDate}T00:00:00Z`), end = new Date(`${range.endDate}T00:00:00Z`); day <= end; day.setUTCDate(day.getUTCDate() + 1)) dates.push(day.toISOString().slice(0, 10));
  const max = Math.max(1, ...known.map(row => row.totals.text_total_tokens));
  const keys = ['uncached_text_input_tokens', 'cached_text_input_tokens', 'text_output_tokens'];
  const colors = ['var(--chart-input)', 'var(--chart-cache)', 'var(--chart-output)'];
  $('daily-chart').innerHTML = dates.map(date => {
    const totals = byDate.get(date)?.totals;
    const value = totals?.text_total_tokens;
    const isKnown = finite(value);
    const complete = keys.every(key => finite(totals?.[key]));
    const tooltip = `${date}\n${isKnown ? full(value) + ' Token' : '未返回 Token 数据'}`;
    let segments = '';
    if (isKnown) {
      segments = complete ? keys.map((key, i) => `<span class="chart-segment" style="height:${totals[key] / max * 175}px;background:${colors[i]}"></span>`).join('') : `<span class="chart-segment" style="height:${value / max * 175}px;background:var(--muted)"></span>`;
    }
    return `<div tabindex="0" class="chart-day ${isKnown ? '' : 'missing'}" data-tooltip="${esc(tooltip)}" aria-label="${esc(tooltip)}"><div class="chart-bars" style="height:${isKnown ? Math.max(value / max * 175, value === 0 ? 1 : 0) : 6}px">${segments}</div><span class="chart-label" data-date="${date}"></span></div>`;
  }).join('');
  fitChartLabels();
  $('daily-chart').setAttribute('aria-label', `每日 Token 用量，${known.length} 天已返回数据，已知合计 ${full(total)} Token。完整数字见每日明细。`);
  $('daily-table').innerHTML = dates.map(date => `<tr><td>${date}</td>${[...keys, 'text_total_tokens'].map(key => `<td>${full(byDate.get(date)?.totals[key])}</td>`).join('')}</tr>`).join('');
  const clients = new Map();
  for (const row of rows) for (const client of row.clients) {
    const current = clients.get(client.client) || { label: client.client, value: 0, known: 0, missing: false };
    if (finite(client.turns)) { current.value += client.turns; current.known++; } else current.missing = true;
    clients.set(client.client, current);
  }
  const labels = { CODEX_CLI: 'Codex CLI', CODEX_DESKTOP_APP: 'Codex 桌面端', CODEX_UNKNOWN_DEFAULT: '其他 / 未识别', CODEX_WORK_WEB: 'Work 网页端' };
  distribution('clients', [...clients.values()].sort((a, b) => b.value - a.value).map(row => ({ label: labels[row.label] || row.label, value: row.known ? row.value : null, suffix: `${row.known ? full(row.value) : '—'}${row.missing ? '（部分缺失）' : ''}` })));
}
function fitChartLabels() {
  const chart = $('daily-chart'), labels = [...chart.querySelectorAll('.chart-label')];
  if (!labels.length || !chart.clientWidth) return;
  const space = Math.max(50, parseFloat(getComputedStyle(labels[0]).fontSize) * 3 + 12);
  const step = Math.max(1, Math.ceil(labels.length * space / chart.clientWidth));
  const visible = labels.map((_, i) => i).filter(i => i % step === 0);
  const last = labels.length - 1;
  if (visible.at(-1) !== last) {
    if (visible.length > 1 && last - visible.at(-1) < step) visible.pop();
    visible.push(last);
  }
  for (const [i, label] of labels.entries()) label.textContent = visible.includes(i) ? label.dataset.date.slice(5).replace('-', '/') : '';
}
new ResizeObserver(fitChartLabels).observe($('daily-chart'));
function renderBreakdown() {
  const row = breakdown?.data.find(item => item.date === $('breakdown-date').value);
  const models = (row?.models || []).toSorted((a, b) => {
    if (a.value == null) return b.value == null ? 0 : 1;
    if (b.value == null) return -1;
    return b.value - a.value;
  });
  const complete = models.every(model => finite(model.value) && model.value >= 0);
  const total = complete ? models.reduce((sum, model) => sum + model.value, 0) : null;
  if (models.length && total === 0) {
    $('breakdown-table').innerHTML = '<tr><td colspan="3" class="empty">当日暂无用量</td></tr>';
    return;
  }
  $('breakdown-table').innerHTML = models.length ? models.map(model => {
    const share = finite(total) && total > 0 ? model.value / total * 100 : null;
    return `<tr><td>${esc(model.model)}</td><td>${esc(model.speed)}</td><td>${share == null ? '—' : `${share.toFixed(3)}%`}</td></tr>`;
  }).join('') : '<tr><td colspan="3" class="empty">暂无模型用量记录</td></tr>';
}

const calculatorFields = ['input', 'cached', 'write', 'output'];
const calculatorPriceKeys = ['inputCostPerToken', 'cacheReadCostPerToken', 'cacheWriteCostPerToken', 'outputCostPerToken'];
const calculatorStorageKey = 'codex-tally-calculator-prices';
let customPrices = [null, null, null, null];
let calculatorPrices, calculatorPricesRequest, calculatorRequest;
try {
  const saved = JSON.parse(localStorage.getItem(calculatorStorageKey));
  if (Array.isArray(saved) && saved.length === 4 && saved.every(value => value === null || (finite(value) && value >= 0))) customPrices = saved;
} catch { /* Custom prices remain available without browser storage. */ }
function setCalculatorPrices(prices) {
  calculatorFields.forEach((field, i) => { $(`calc-${field}-price`).value = prices[i] ?? ''; });
  calculate();
}
function tokenValue(value) {
  const digits = value.trim().replaceAll(',', '');
  return /^\d+$/.test(digits) ? Number(digits) : NaN;
}
function formatTokenInput(input) {
  const value = tokenValue(input.value);
  if (!Number.isSafeInteger(value) || value < 0) return;
  const before = input.value.slice(0, input.selectionStart).replace(/\D/g, '').length;
  input.value = full(value);
  let position = 0, digits = 0;
  while (position < input.value.length && digits < before) {
    if (/\d/.test(input.value[position])) digits++;
    position++;
  }
  if (document.activeElement === input) input.setSelectionRange(position, position);
}
function calculate() {
  let total = 0, tokens = 0, invalid = false, missing = false;
  for (const field of calculatorFields) {
    const amount = $(`calc-${field}-tokens`), price = $(`calc-${field}-price`);
    const count = tokenValue(amount.value), rate = price.valueAsNumber;
    const validAmount = amount.validity.valid && Number.isSafeInteger(count) && count >= 0;
    const absentPrice = price.value === '' && !price.validity.badInput;
    const validPrice = !absentPrice && price.validity.valid && finite(rate) && rate >= 0;
    amount.setAttribute('aria-invalid', String(!validAmount));
    text(`calc-${field}-magnitude`, validAmount ? compact(count) : '—');
    price.setAttribute('aria-invalid', String(!validPrice && (!absentPrice || (validAmount && count > 0))));
    const cost = validAmount && (validPrice || (count === 0 && absentPrice)) ? count / 1e6 * (rate || 0) : NaN;
    text(`calc-${field}-cost`, finite(cost) ? new Intl.NumberFormat('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(cost) : '—');
    invalid ||= !validAmount || (!validPrice && !absentPrice) || (validPrice && validAmount && !finite(cost));
    missing ||= validAmount && count > 0 && absentPrice;
    tokens += validAmount ? count : 0;
    total += cost;
  }
  invalid ||= !Number.isSafeInteger(tokens) || (!missing && !finite(total));
  text('calculator-tokens', invalid ? '—' : full(tokens));
  text('calculator-magnitude', invalid ? '' : compact(tokens));
  text('calculator-total', invalid || missing ? '—' : money(total));
  text('calculator-error', invalid ? '请输入有效的非负数；Token 必须为安全范围内的整数。' : missing ? '请补全有用量项目的单价。' : '');
}
async function loadCalculatorPrices() {
  if (calculatorPrices || calculatorPricesRequest) return;
  const request = calculatorPricesRequest = new AbortController();
  try {
    const result = await api('/api/pricing', { signal: request.signal });
    if (request.signal.aborted) return;
    calculatorPrices = result.models;
    for (const name of Object.keys(calculatorPrices).sort()) $('calculator-model').add(new Option(name, name));
  } catch (error) {
    if (error.name !== 'AbortError') text('calculator-storage', '预设价格暂不可用，可手填单价。');
  } finally { if (calculatorPricesRequest === request) calculatorPricesRequest = null; }
}
$('calculator-model').addEventListener('change', () => {
  const model = calculatorPrices?.[$('calculator-model').value];
  setCalculatorPrices(model ? calculatorPriceKeys.map(key => finite(model[key]) ? Number((model[key] * 1e6).toPrecision(12)) : null) : customPrices);
});
$('calculator-form').addEventListener('submit', event => event.preventDefault());
$('calculator-form').addEventListener('input', event => {
  if (event.isComposing) return;
  if (event.target.id.endsWith('-price')) {
    $('calculator-model').value = '';
    const prices = calculatorFields.map(field => $(`calc-${field}-price`).value === '' ? null : $(`calc-${field}-price`).valueAsNumber);
    if (calculatorFields.every(field => $(`calc-${field}-price`).validity.valid) && prices.every(value => value === null || (finite(value) && value >= 0))) {
      customPrices = prices;
      try { localStorage.setItem(calculatorStorageKey, JSON.stringify(prices)); text('calculator-storage', ''); }
      catch { text('calculator-storage', '浏览器未保存单价，本页仍可计算。'); }
    }
  } else {
    formatTokenInput(event.target);
    calculatorRequest?.abort(); text('calculator-range', '');
  }
  calculate();
});
for (const field of calculatorFields) {
  const input = $(`calc-${field}-tokens`);
  input.addEventListener('keydown', event => {
    const position = input.selectionStart;
    if (position !== input.selectionEnd || event.ctrlKey || event.metaKey || event.altKey || event.shiftKey) return;
    if (event.key === 'Backspace' && input.value[position - 1] === ',') input.setSelectionRange(position - 2, position);
    if (event.key === 'Delete' && input.value[position] === ',') input.setSelectionRange(position, position + 2);
  });
  input.addEventListener('compositionend', () => input.dispatchEvent(new Event('input', { bubbles: true })));
}
function clearCalculatorUsage() {
  calculatorRequest?.abort();
  for (const field of calculatorFields) $(`calc-${field}-tokens`).value = '0';
  text('calculator-range', ''); calculate();
}
$('calculator-clear').addEventListener('click', clearCalculatorUsage);
$('calculator-import').disabled = false;
$('calculator-import').addEventListener('click', async () => {
  calculatorRequest?.abort();
  const request = calculatorRequest = new AbortController();
  const params = new URLSearchParams({ period: $('period').value });
  if ($('period').value.startsWith('custom')) params.set('end', $('window-end').value);
  $('calculator-import').disabled = true;
  try {
    const data = views.get(`local:${params}`) ?? await api(`/api/local?${params}`, { signal: request.signal });
    if (request.signal.aborted) return;
    if (data.error) throw new Error(data.error);
    if (!calculatorFields.every(field => Number.isSafeInteger(data[field]) && data[field] >= 0)) throw new Error('本机用量不完整，请刷新后重试。');
    for (const field of calculatorFields) $(`calc-${field}-tokens`).value = full(data[field]);
    text('calculator-range', `${timestamp(data.range.from)} 至 ${timestamp(data.range.to)}`);
    calculate();
  } catch (error) {
    if (error.name !== 'AbortError') text('calculator-error', error.message);
  } finally {
    if (calculatorRequest === request) { calculatorRequest = null; $('calculator-import').disabled = false; }
  }
});
setCalculatorPrices(customPrices);

(async () => {
  try {
    const status = await api('/api/session'); timezone = status.timezone;
    $('share-link').hidden = !status.publicShare;
    const query = new URLSearchParams(location.search);
    tab = tabs.includes(query.get('tab')) ? query.get('tab') : 'local';
    const source = tab === 'calculator' ? 'local' : tab;
    if ([...tabControl('period', source).options].some(option => option.value === query.get('period'))) tabControl('period', source).value = query.get('period');
    configurePeriod(source);
    if (query.get('end') && tabControl('period', source).value.startsWith('custom')) tabControl('window-end', source).value = query.get('end');
    if (status.authenticated) showDashboard(); else showLogin();
  } catch { text('boot', '无法连接仪表盘服务，请重新打开页面。'); }
})();
});
