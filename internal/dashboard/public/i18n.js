(() => {
  const scriptURL = document.currentScript.src;
  const storedKey = 'codex-tally-language';
  const query = new URLSearchParams(location.search).get('lang');
  let preference = 'auto';
  try { preference = localStorage.getItem(storedKey) || 'auto'; } catch { /* Device preference is optional. */ }
  if (!['auto', 'en', 'zh-CN'].includes(preference)) preference = 'auto';
  if (query === 'en' || query === 'zh-CN') preference = query;
  let language = preference === 'en' || preference === 'zh-CN' ? preference : navigator.language.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en';
  document.documentElement.lang = language;
  let messages = {};
  function t(message, values = {}) {
    const source = String(message ?? '');
    const upstreamStatus = source.match(/^用量接口暂不可用（(\d+)）$/);
    if (upstreamStatus) return t('用量接口暂不可用（{status}）', { status: upstreamStatus[1] });
    const translated = language === 'en' ? messages[source] ?? source : source;
    return translated.replace(/\{(\w+)\}/g, (match, key) => values[key] ?? match);
  }
  function localize(root = document) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      const node = walker.currentNode;
      if (node.parentElement?.closest('script, style, textarea, code, [data-no-translate]')) continue;
      const key = node.nodeValue.trim();
      if (key) node.nodeValue = node.nodeValue.replace(key, t(key));
    }
    for (const element of root.querySelectorAll('[aria-label], [title], [placeholder], [alt]')) {
      for (const attribute of ['aria-label', 'title', 'placeholder', 'alt']) {
        if (element.hasAttribute(attribute)) element.setAttribute(attribute, t(element.getAttribute(attribute)));
      }
    }
    for (const select of root.querySelectorAll('[data-language]')) {
      select.value = preference;
      select.addEventListener('change', () => {
        window.dispatchEvent(new Event('codex-language-change'));
        try { localStorage.setItem(storedKey, select.value); } catch { /* The URL also preserves the choice. */ }
        const url = new URL(location.href);
        if (select.value === 'auto') url.searchParams.delete('lang');
        else url.searchParams.set('lang', select.value);
        history.replaceState(history.state, '', url);
        location.reload();
      });
    }
  }
  const request = new AbortController();
  const timeout = setTimeout(() => request.abort(), 5000);
  const ready = fetch(new URL('i18n.json', scriptURL), { signal: request.signal }).then(response => {
    if (!response.ok) throw new Error('Could not load translations. Reload the page.');
    return response.json();
  }).then(value => { messages = value; }).catch(error => {
    language = 'zh-CN';
    document.documentElement.lang = language;
    console.warn('Translations unavailable; using Chinese.', error);
  }).finally(() => clearTimeout(timeout));
  window.CodexI18n = { get language() { return language; }, preference, t, localize, ready };
})();
