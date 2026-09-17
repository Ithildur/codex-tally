// Run against a local instance with PUBLIC_SHARE=1 and an installed Playwright.
import assert from 'node:assert/strict';
import { readFile, mkdir } from 'node:fs/promises';
import { createServer } from 'node:http';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const origin = process.env.DASHBOARD_URL || 'http://localhost:4318';
const artifacts = process.env.SCREENSHOT_DIR || '/tmp/codex-ui-review';
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, args: ['--no-sandbox'] });
try {
  await mkdir(artifacts, { recursive: true });
  const page = await browser.newPage({ viewport: { width: 1578, height: 984 } });
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto(origin);
  await page.locator('#password').waitFor({ state: 'visible' });
  await page.locator('#password').fill((await readFile(process.env.PASSWORD_FILE || '.state/password', 'utf8')).trim());
  await page.locator('#password').press('Enter');
  await page.locator('#dashboard').waitFor({ state: 'visible' });
  await page.locator('#refresh:not([disabled])').waitFor({ timeout: 120000 });
  assert.equal(await page.locator('#account-period option[value="24"],#account-period option[value="custom5"]').count(), 0);
  const limits = await page.evaluate(async () => (await (await fetch('/api/limits')).json()).data);
  if (limits?.plan?.startsWith('pro')) {
    assert.equal(await page.locator('#period option[value="quota5"]').isDisabled(), true);
  }
  const weekly = limits?.buckets.find(x => x.name === 'Codex');
  const window = [weekly?.primary, weekly?.secondary].find(x => x?.seconds === 604800);
  if (window?.resetsAt * 1000 > Date.now()) {
    const range = await page.evaluate(async () => (await (await fetch('/api/local?period=quota7')).json()).range);
    assert.equal(range.from, (window.resetsAt - window.seconds) * 1000);
    assert.equal(range.to, window.resetsAt * 1000);
  }
  await page.locator('#share-link').click();
  await page.locator('#share-format').selectOption('iframe');
  const embed = await browser.newPage();
  await embed.goto(origin);
  const keys = ['tokens', 'calls', 'cache', 'models'];
  // Layout depends on numeric column count and presence of the model table.
  // Selection membership and all 15 masks are covered by the Go boundary tests.
  for (const mask of [1, 3, 7, 8, 9, 11, 15]) {
    for (let i = 0; i < keys.length; i++) {
      await page.locator(`#share-hub-components input[value="${keys[i]}"]`).setChecked(Boolean(mask & (1 << i)));
    }
    await page.locator('#share-preview[aria-busy="false"] iframe').waitFor();
    await page.locator('#share-copy:not([disabled])').waitFor();
    const code = await page.locator('#share-code').inputValue();
    assert.ok(code.includes(new URL(origin).origin));
    for (const width of [1100, 700, 320]) {
      await embed.setViewportSize({ width, height: 1000 });
      await embed.setContent(`<style>body{margin:0}</style>${code}`);
      const content = embed.frameLocator('iframe');
      await content.locator('.hub-grid').waitFor();
      await embed.frames()[1].waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
      const size = await content.locator('body').evaluate(() => ({
        width: innerWidth, height: innerHeight,
        scrollWidth: document.documentElement.scrollWidth,
        scrollHeight: document.documentElement.scrollHeight,
        components: [...document.querySelectorAll('[data-component]')].map(x => x.dataset.component),
      }));
      assert.ok(size.scrollHeight <= size.height + 1, `mask ${mask}, width ${width}: vertical overflow ${JSON.stringify(size)}`);
      assert.ok(size.scrollWidth <= size.width + 1, `mask ${mask}, width ${width}: horizontal overflow`);
      assert.deepEqual(size.components, keys.filter((_, i) => mask & (1 << i)));
      if (mask === 15 && width !== 700) await embed.screenshot({ path: `${artifacts}/hub-embed-${width}.png`, fullPage: true });
    }
  }
  await page.screenshot({ path: `${artifacts}/hub-composer.png`, fullPage: true });
  for (const component of ['overview', ...keys]) {
    await page.locator('#share-component').selectOption(component);
    await page.locator('#share-preview[aria-busy="false"] iframe').waitFor();
    await page.locator('#share-copy:not([disabled])').waitFor();
    const code = await page.locator('#share-code').inputValue();
    for (const width of [1100, 700, 320]) {
      await embed.setViewportSize({ width, height: 1000 });
      await embed.setContent(`<style>body{margin:0}</style>${code}`);
      const frame = embed.frameLocator('iframe');
      await frame.locator('main').waitFor();
      await embed.frames()[1].waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
      assert.equal(await frame.locator('body').evaluate(() => document.documentElement.scrollHeight <= innerHeight + 1 && document.documentElement.scrollWidth <= innerWidth + 1), true, `${component} overflows at ${width}`);
    }
    await page.locator('#share-format').selectOption('svg');
    await page.locator('#share-preview[aria-busy="false"] img').waitFor();
    assert.ok((await page.locator('#share-code').inputValue()).includes(`/${component}.svg`));
    await page.locator('#share-format').selectOption('iframe');
  }
  await page.locator('#share-component').selectOption('hub');
  await page.locator('#share-theme').selectOption('dark');
  await page.locator('#share-preview[aria-busy="false"] iframe').waitFor();
  await page.screenshot({ path: `${artifacts}/hub-composer-dark.png`, fullPage: true });
  // Existing embed code must survive a later snapshot with more model rows.
  const growingCode = await page.locator('#share-code').inputValue();
  let grow = false;
  await embed.route('**/share/hub?**', async route => {
    const response = await route.fetch();
    let body = await response.text();
    if (grow) body = body.replace('</tbody>', '<tr><td>additional-model</td><td>1M</td><td>10</td></tr>'.repeat(8) + '</tbody>');
    await route.fulfill({ response, body });
  });
  await embed.setViewportSize({ width: 1100, height: 1200 });
  await embed.setContent(`<style>body{margin:0}</style>${growingCode}`);
  await embed.frameLocator('iframe').locator('.hub-grid').waitFor();
  await embed.frames()[1].waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
  const originalHeight = await embed.locator('iframe').evaluate(el => el.clientHeight);
  grow = true;
  await embed.frames()[1].goto(await embed.locator('iframe').getAttribute('src'));
  await embed.frames()[1].waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
  const grownHeight = await embed.locator('iframe').evaluate(el => el.clientHeight);
  assert.ok(grownHeight > originalHeight + 300, 'existing embed did not grow with the next snapshot');
  await embed.screenshot({ path: `${artifacts}/hub-growing-snapshot.png`, fullPage: true });
  grow = false;
  await embed.frames()[1].goto(await embed.locator('iframe').getAttribute('src'));
  await embed.frames()[1].waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
  assert.equal(await embed.locator('iframe').evaluate(el => el.clientHeight), originalHeight, 'embed did not shrink');
  await embed.setViewportSize({ width: 320, height: 1200 });
  await embed.frames()[1].waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
  const mobileHeight = await embed.locator('iframe').evaluate(el => el.clientHeight);
  await embed.evaluate(() => {
    dispatchEvent(new MessageEvent('message', { origin: 'null', source: window, data: { type: 'codex:height', height: 12345 } }));
    dispatchEvent(new MessageEvent('message', { origin: 'null', source: frames[0], data: { type: 'codex:height', height: -1 } }));
  });
  assert.equal(await embed.locator('iframe').evaluate(el => el.clientHeight), mobileHeight, 'untrusted resize message accepted');
  assert.equal(await embed.frames()[1].evaluate(() => { try { document.cookie; return false; } catch { return true; } }), true, 'public iframe lost its opaque sandbox');
  const cross = await browser.newPage({ viewport: { width: 700, height: 1200 } });
  const privateRequests = [];
  cross.on('request', request => { if (request.url().startsWith(`${origin}/api/`)) privateRequests.push(request.url()); });
  const host = createServer((_, response) => {
    response.setHeader('Content-Type', 'text/html; charset=utf-8');
    response.end(`<style>body{margin:0}</style>${growingCode}${growingCode}`);
  });
  await new Promise(resolve => host.listen(0, '127.0.0.1', resolve));
  try {
    await cross.goto(`http://127.0.0.1:${host.address().port}/`);
    await cross.frameLocator('iframe').first().locator('.hub-grid').waitFor();
    await cross.frameLocator('iframe').last().locator('.hub-grid').waitFor();
    for (const frame of cross.frames().slice(1)) await frame.waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
    assert.deepEqual(privateRequests, [], 'anonymous embed requested private data');
  } finally { await cross.close(); host.close(); }
  for (const key of keys) await page.locator(`#share-hub-components input[value="${key}"]`).uncheck();
  assert.equal(await page.locator('#share-copy').isDisabled(), true);
  assert.equal(await page.locator('#share-code').inputValue(), '');
  await page.keyboard.press('Escape');
  assert.equal(await page.locator('#share-link').evaluate(el => el === document.activeElement), true);
  // Native system zones (including Windows) can be labelled "Local" rather
  // than an IANA name. Boot and custom-date controls must still work.
  await page.route(`${origin}/api/**`, async route => {
    const response = await route.fetch();
    const data = await response.json();
    if (data.timezone) data.timezone = 'Local';
    if (data.range) data.range.timezone = 'Local';
    await route.fulfill({ response, json: data });
  });
  await page.reload();
  await page.locator('#dashboard').waitFor({ state: 'visible' });
  await page.locator('#refresh:not([disabled])').waitFor({ timeout: 120000 });
  await page.locator('#period').selectOption('custom7d');
  assert.match(await page.locator('#window-end').inputValue(), /^\d{4}-\d{2}-\d{2}$/);
  assert.deepEqual(errors, []);
  console.log('PASS: quota boundaries, Pro options, 36 embed sizes, snapshot growth/shrink, live resizing, message validation, sandbox, SVG, dialog state and native Local timezone');
} finally {
  await browser.close();
}
