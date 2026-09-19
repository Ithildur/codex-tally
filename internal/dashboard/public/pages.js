(async () => {
  await CodexI18n.ready;
  const { t, language } = CodexI18n;
  CodexI18n.localize();
  const $ = id => document.getElementById(id);
  const names = {tokens: '总 Token', calls: '调用', cache: '缓存率', models: '模型'};
  const fields = {tokens: 'tokens', calls: 'calls', cache: 'cacheRate', models: 'models'};
  // Relative URLs support project Pages, custom domains, and local previews.
  const base = new URL('./', location.href);
  const escape = text => text.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;');
  let available = [];
  let modelCount = 0;
  function update() {
    const selected = available.filter(key => $(`component-${key}`).checked);
    const theme = $('theme').value;
    const format = $('format').value;
    history.replaceState(null, '', `?components=${selected.join(',')}&theme=${theme}&format=${format}${CodexI18n.preference !== 'auto' ? `&lang=${CodexI18n.preference}` : ''}`);
    document.documentElement.dataset.theme = theme;
    $('status').textContent = '';
    $('copy').disabled = !selected.length;
    $('open').hidden = !selected.length;
    $('preview').hidden = !selected.length || format === 'svg' || format === 'markdown';
    $('image').hidden = !selected.length || !$('preview').hidden;
    if (!selected.length) {
      $('code').value = '';
      $('preview').removeAttribute('src');
      $('image').removeAttribute('src');
      $('status').textContent = t('选择至少一个组件');
      return;
    }
    const component = selected.length === 1 ? selected[0] : `hub/${selected.join('-')}`;
    const page = new URL(`${component}/${theme}${language === 'en' ? '.en' : ''}.html`, base).href;
    const svg = new URL(`${component}/${theme}${language === 'en' ? '.en' : ''}.svg`, base).href;
    const numeric = selected.filter(key => key !== 'models').length;
    const width = selected.length === 1 ? (selected[0] === 'models' ? 720 : 480) : Math.max(selected.includes('models') ? 720 : 480, numeric * 280 + 72);
    // A safe initial height; embed.js measures the actual responsive document.
    const height = 240 + numeric * 130 + (selected.includes('models') ? 100 + Math.max(1, modelCount) * 52 : 0);
    const frame = $('preview');
    frame.style.width = `${width}px`;
    if (frame.src !== page) { frame.style.height = `${height}px`; frame.src = page; }
    $('image').src = svg;
    $('open').href = format === 'svg' || format === 'markdown' ? svg : page;
    $('code').value = format === 'iframe'
      ? `<iframe data-codex-usage sandbox="allow-scripts" src="${escape(page)}" title="${t('Codex 用量')}" width="${width}" height="${height}" loading="lazy" style="display:block;width:100%;max-width:${width}px;border:0"></iframe>\n<script async src="${escape(new URL('embed.js', base).href)}"></script>`
      : format === 'markdown' ? `![${t('Codex 用量')}](${svg})` : format === 'svg' ? svg : page;
  }
  $('copy').addEventListener('click', async () => {
    try {
      if (navigator.clipboard?.writeText) await navigator.clipboard.writeText($('code').value);
      else { $('code').select(); if (!document.execCommand('copy')) throw new Error('copy'); }
      $('status').textContent = t('已复制');
    } catch { $('code').focus(); $('code').select(); $('status').textContent = t('请复制已选中的内容'); }
  });
  for (const id of ['components', 'theme', 'format']) $(id).addEventListener('change', update);
  async function load() {
    const response = await fetch(new URL('usage.json', base), {credentials: 'omit', cache: 'no-cache'});
    if (!response.ok) throw new Error('snapshot');
    const snapshot = await response.json();
    available = Object.keys(names).filter(key => snapshot[fields[key]] != null);
    modelCount = snapshot.models?.length || 0;
    $('month').textContent = snapshot.month;
    const params = new URLSearchParams(location.search);
    const selection = params.has('components') ? params.get('components').split(',') : available;
    for (const key of available) {
      const label = document.createElement('label');
      const input = document.createElement('input');
      input.type = 'checkbox'; input.id = `component-${key}`; input.checked = selection.includes(key);
      label.append(input, t(names[key])); $('components').append(label);
    }
    for (const id of ['theme','format']) {
      const value = params.get(id);
      if ([...$(id).options].some(option => option.value === value)) $(id).value = value;
    }
    update();
  }
  load().catch(() => { $('status').textContent = t('公开快照加载失败，请刷新重试'); });
})();
