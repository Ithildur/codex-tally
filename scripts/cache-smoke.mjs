import assert from 'node:assert/strict';
const password = process.env.DASHBOARD_PASSWORD;
if (!password) throw new Error('Set DASHBOARD_PASSWORD to the running server password');
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const origin = process.env.DASHBOARD_URL || 'http://localhost:4318';
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, args: ['--no-sandbox'] });
try {
  const page = await browser.newPage();
  await page.clock.install();
  await page.clock.pauseAt(new Date(Date.now() + 1000));
  let calls = 0, quotaCalls = 0, fail = false;
  const result = data => ({ data, fetchedAt: new Date().toISOString(), cached: false, error: fail ? 'temporary failure' : null, refreshAfterMs: fail ? 30000 : 1800000 });
  await page.route('**/api/limits*', route => {
    quotaCalls++;
    return route.fulfill({ json: result({ plan: 'pro', buckets: [] }) });
  });
  await page.route('**/api/account?**', route => {
    calls++;
    return route.fulfill({ json: {
      range: { from: 0, to: 1, startDate: '2026-09-17', endDate: '2026-09-17', timezone: 'Asia/Hong_Kong' },
      counts: result({ data: [] }), breakdown: result({ data: [] }), limits: result({ plan: 'pro', buckets: [] }),
    } });
  });
  await page.goto(`${origin}/?tab=account`);
  await page.locator('#password').waitFor({ state: 'visible' });
  await page.locator('#password').fill(password);
  await page.locator('#password').press('Enter');
  const settled = async () => {
    await page.locator('#dashboard').waitFor({ state: 'visible' });
    await page.locator('#account-refresh:not([disabled])').waitFor();
    await page.locator('#quota-refresh:not([disabled])').waitFor({ state: 'attached' });
  };
  await settled();
  assert.equal(calls, 1); assert.equal(quotaCalls, 1);
  await page.clock.runFor(1799000);
  assert.equal(calls, 1); assert.equal(quotaCalls, 1);
  fail = true;
  await page.clock.runFor(1000); await settled();
  assert.equal(calls, 2); assert.equal(quotaCalls, 2);
  await page.clock.runFor(29000);
  assert.equal(calls, 2); assert.equal(quotaCalls, 2);
  fail = false;
  await page.clock.runFor(1000); await settled();
  assert.equal(calls, 3); assert.equal(quotaCalls, 3);
  assert.equal(await page.locator('#cloud-error').textContent(), '');
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await page.clock.runFor(1800000);
  assert.equal(calls, 3); assert.equal(quotaCalls, 3);
  await page.evaluate(() => {
    delete document.hidden;
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await page.clock.runFor(1); await settled();
  assert.equal(calls, 4); assert.equal(quotaCalls, 4);
  await page.locator('#account-refresh').click(); await settled();
  assert.equal(calls, 5); assert.equal(quotaCalls, 5);
  await page.locator('#logout').click();
  await page.locator('#login').waitFor({ state: 'visible' });
  await page.clock.runFor(3600000);
  assert.equal(calls, 5); assert.equal(quotaCalls, 5);
  console.log('PASS: 30-minute refresh, 30-second retry, recovery, visibility pause/resume, manual refresh and logout cancellation');
} finally { await browser.close(); }
