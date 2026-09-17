import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtemp, readFile, mkdir, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const temporary = await mkdtemp(path.join(tmpdir(), 'codex-pages-'));
const site = path.join(temporary, 'site');
const screenshots = process.env.SCREENSHOT_DIR || '/tmp/codex-pages-review';
await mkdir(screenshots, {recursive: true});
execFileSync('go', ['run', './cmd/codex-tally', 'build-pages', '-input', 'internal/dashboard/testdata/public-usage.json', '-out', site], {cwd: root});
const types = {'.html':'text/html', '.svg':'image/svg+xml', '.js':'text/javascript', '.css':'text/css', '.json':'application/json'};
const server = createServer(async (request, response) => {
  try {
    const url = new URL(request.url, 'http://localhost');
    if (url.pathname === '/favicon.ico') { response.writeHead(204).end(); return; }
    if (url.pathname === '/embed-test') {
      response.setHeader('Content-Type', 'text/html');
      response.end('<!doctype html><html><body><div id="embed"></div></body></html>');
      return;
    }
    const relative = url.pathname.replace(/^\/project\//, '').replace(/^\//, '') || 'index.html';
    const file = path.resolve(site, relative);
    if (!file.startsWith(`${site}${path.sep}`)) throw new Error('path');
    response.setHeader('Content-Type', types[path.extname(file)] || 'application/octet-stream');
    response.end(await readFile(file));
  } catch { response.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const browser = await chromium.launch({executablePath: process.env.CHROMIUM_PATH || undefined, args:['--no-sandbox']});
try {
  const context = await browser.newContext({viewport:{width:1200,height:1000}});
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', error => failures.push(error.message));
  page.on('console', message => { if (message.type() === 'error') failures.push(message.text()); });
  page.on('request', request => assert.ok(!request.url().includes('/api/'), 'static page contacted private API'));
  for (const prefix of ['/', '/project/']) {
    await page.goto(origin + prefix);
    await page.waitForFunction(() => !document.getElementById('copy').disabled);
    await page.selectOption('#format','iframe');
    const code = await page.inputValue('#code');
    assert.ok(code.includes(`${origin}${prefix}hub/tokens-calls-cache-models/auto.html`));
    assert.ok(code.includes('sandbox="allow-scripts"'));
    for (const width of [1200, 700, 360]) {
      await page.setViewportSize({width,height:1100});
      await page.waitForFunction(() => {
        const frame = document.getElementById('preview');
        return frame.getBoundingClientRect().height > 100;
      });
      const frame = page.frames().find(frame => frame.url().includes('/hub/'));
      await frame.waitForSelector('footer');
      await frame.waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
      const actual = await frame.evaluate(() => ({height:Math.ceil(document.body.getBoundingClientRect().height), overflow:document.documentElement.scrollWidth > innerWidth}));
      const height = await page.locator('#preview').evaluate(node => node.getBoundingClientRect().height);
      assert.ok(Math.abs(actual.height-height) <= 2, `incorrect iframe height: ${actual.height}/${height}`);
      assert.equal(actual.overflow,false);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
      if (prefix === '/project/') await page.screenshot({path:path.join(screenshots,`pages-${width}.png`),fullPage:true});
    }
    await page.setViewportSize({width:1200,height:1000});
    await page.selectOption('#theme','light');
    await page.screenshot({path:path.join(screenshots,'pages-light.png'),fullPage:true});
    await page.selectOption('#theme','dark');
    await page.selectOption('#format','svg');
    await page.waitForFunction(() => document.getElementById('image').naturalWidth > 0);
    assert.ok((await page.inputValue('#code')).endsWith('/dark.svg'));
    await page.screenshot({path:path.join(screenshots,'pages-svg.png'),fullPage:true});
    for (const key of ['tokens','calls','cache','models']) await page.uncheck(`#component-${key}`);
    assert.equal(await page.isDisabled('#copy'),true);
    await page.check('#component-tokens');
    await page.selectOption('#format','html');
    assert.equal(await page.inputValue('#code'), `${origin}${prefix}tokens/dark.html`);
    await page.reload();
    await page.waitForFunction(() => !document.getElementById('copy').disabled);
    assert.equal(await page.isChecked('#component-models'),false);
  }
  // Execute the actual copied iframe+script on a separate parent page.
  await page.selectOption('#format','iframe');
  const code = await page.inputValue('#code');
  const parent = await context.newPage();
  await parent.goto(origin+'/embed-test');
  await parent.evaluate(code => {
    const target = document.getElementById('embed');
    target.innerHTML = code;
    const script = document.createElement('script');
    script.src = target.querySelector('script').src;
    target.querySelector('script').replaceWith(script);
  },code);
  const embedded = await parent.waitForSelector('iframe');
  const frame = await embedded.contentFrame();
  await frame.waitForSelector('footer');
  await frame.waitForFunction(() => innerHeight === Math.ceil(document.body.getBoundingClientRect().height));
  assert.ok(await frame.evaluate(() => { try { return parent.document === undefined; } catch { return true; } }), 'iframe can access parent');
  assert.ok(Math.abs(await embedded.evaluate(node => node.getBoundingClientRect().height) - await frame.evaluate(() => Math.ceil(document.body.getBoundingClientRect().height))) <= 2);
  assert.deepEqual(failures, []);
  console.log(`Pages smoke passed: root/project URLs, responsive iframe, SVG, themes, selection, sandbox. Screenshots: ${screenshots}`);
} finally {
  await browser.close();
  await new Promise(resolve => server.close(resolve));
  await rm(temporary,{recursive:true,force:true});
}
