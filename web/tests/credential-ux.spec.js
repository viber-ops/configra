import { expect, test } from '@playwright/test';

test.use({ timezoneId: 'Asia/Singapore' });
const ca = { id: '00112233445566778899aabbccddeeff', display_name: 'yinxiaoban-production', fingerprint_sha256: '0123456789abcdef'.repeat(4), certificate_pem: 'public-test-certificate', not_before: '2026-01-01T00:00:00Z', not_after: '2031-10-15T00:00:00Z', revoked: false, client_certificate_count: 1 };
const certificate = { display_name: 'production-host', subject: 'CN=production-host', authority_id: ca.id, authority_name: ca.display_name, fingerprint_sha256: 'fedcba9876543210'.repeat(4), serial_hex: 'ab12', not_before: '2026-01-01T00:00:00Z', not_after: '2026-10-18T00:00:00Z', revoked: false };
const token = { public_id: '1234567890abcdef', display_name: 'old-operator', display_prefix: 'cfg_1234567890abcdef', environment_keys: ['production'], kind: 'write-scoped', allow_without_mtls: false, expires_at: '2026-10-14T00:00:00Z', revoked: false };

async function workspace(page, { maxDays = 90, authorities = [ca], tokens = [token], zh = false } = {}) {
  await page.clock.setFixedTime(new Date('2026-10-15T00:00:00Z'));
  if (zh) await page.addInitScript(() => localStorage.setItem('configra-language', 'zh'));
  await page.route('**/v1/**', route => {
    const path = new URL(route.request().url()).pathname;
    let json = { items: [], total: 0 };
    if (path === '/v1/me') json = { subject: 'admin', role: 'admin', write_token_max_ttl_days: maxDays };
    if (path === '/v1/certificate-authorities') json = { items: authorities, total: authorities.length };
    if (path === '/v1/client-certificates') json = { items: [certificate], total: 1 };
    if (path === '/v1/api-tokens') json = { items: tokens, total: tokens.length };
    if (path === '/v1/environments') json = { items: [{ key: 'production', display_name: 'Production' }], total: 1 };
    return route.fulfill({ json });
  });
  await page.goto('/ui/#/administration');
}

test('CA cards expose full identifiers and copy exact values with keyboard feedback', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await workspace(page);
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  const card = page.locator('.authority-card');
  await expect(card.getByText(ca.id, { exact: true })).toBeVisible();
  await expect(card.getByText(ca.fingerprint_sha256, { exact: true })).toBeVisible();
  await card.getByRole('button', { name: 'Copy CA ID', exact: true }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(ca.id);
  await expect(card.getByRole('status')).toContainText('Copied');
  const fingerprint = card.getByRole('button', { name: 'Copy Fingerprint SHA-256', exact: true });
  await fingerprint.focus();
  await page.keyboard.press('Enter');
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(ca.fingerprint_sha256);
  await expect(card).toContainText('1 client certificate');
  await expect(card).not.toContainText('1 client certificates');
});

test('copy denial explains manual copying and never claims success', async ({ page }) => {
  await page.addInitScript(() => Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: () => Promise.reject(new DOMException('denied', 'NotAllowedError')) } }));
  await workspace(page);
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  const card = page.locator('.authority-card');
  await card.getByRole('button', { name: 'Copy CA ID', exact: true }).click();
  await expect(card.getByRole('status')).toContainText('manually');
  await expect(card.getByRole('status')).not.toContainText('Copied');
  await expect(card.getByText(ca.id, { exact: true })).toBeVisible();
});

test('date selection shows elapsed-day lifetime and immediate limit overrun, with a safe maximum preset', async ({ page }) => {
  await workspace(page);
  await page.getByRole('button', { name: 'New API token' }).click();
  await page.getByLabel('Scoped write token', { exact: true }).check();
  const expiry = page.getByLabel('Expires at', { exact: true });
  await expiry.fill('2027-01-14T08:00');
  await expect(page.locator('.token-form')).toContainText('91 days');
  await expect(page.locator('.token-form')).toContainText('Exceeds the 90-day limit by 1 day');
  await expect(expiry).toHaveAttribute('aria-invalid', 'true');
  await page.getByRole('button', { name: 'Use maximum (90 days)', exact: true }).click();
  await expect(expiry).toHaveValue('2027-01-13T08:00');
  await expect(page.locator('.token-form')).toContainText('Selected lifetime: 90 days');
  await expect(page.locator('.token-form')).not.toContainText('Exceeds');
});

test('short policy presets and remaining time use the configured limit in Chinese', async ({ page }) => {
  await workspace(page, { maxDays: 2, zh: true });
  const row = page.getByRole('row', { name: /old-operator/ });
  await expect(row.locator('.status')).toHaveText('已过期');
  await page.getByRole('button', { name: '新建 API Token' }).click();
  await page.getByLabel('有范围的写令牌', { exact: true }).check();
  await page.getByRole('button', { name: '使用最长有效期（2 天）', exact: true }).click();
  await expect(page.locator('.token-form')).toContainText('所选有效期：2 天');
  await expect(page.getByRole('button', { name: '7 天', exact: true })).toHaveCount(0);
});

test('certificate and Token inventories make full operator identifiers copyable and show expired status', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await workspace(page);
  const row = page.getByRole('row', { name: /old-operator/ });
  await expect(row.locator('.status')).toHaveText('Expired');
  await row.getByRole('button', { name: 'Copy Token ID', exact: true }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(token.public_id);
  await page.getByRole('tab', { name: 'Client certificates' }).click();
  const client = page.getByRole('row', { name: /production-host/ });
  await client.getByRole('button', { name: 'Copy Fingerprint SHA-256', exact: true }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(certificate.fingerprint_sha256);
  await client.getByRole('button', { name: 'Copy CA ID', exact: true }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(ca.id);
  await expect(client).toContainText('3 days');
});

test('client issuance previews the effective date when the CA expires sooner than the requested lifetime', async ({ page }) => {
  await workspace(page, { authorities: [{ ...ca, not_after: '2026-10-18T00:00:00Z' }] });
  await page.getByRole('tab', { name: 'Client certificates' }).click();
  await page.getByRole('button', { name: 'Issue certificate', exact: true }).click();
  const form = page.locator('.credential-create-form');
  await expect(form).toContainText(ca.id);
  await expect(form).toContainText('Limited by CA expiry');
  await expect(form).toContainText('3 days');
  await expect(form).toContainText('2026');
});

test('Token creation prevents duplicate clicks and retains the same operation after an uncertain response', async ({ page }) => {
  await workspace(page);
  const operations = [];
  let release;
  const pending = new Promise(resolve => { release = resolve; });
  await page.route('**/v1/api-tokens', async route => {
    operations.push(route.request().headers()['idempotency-key']);
    if (operations.length === 1) {
      await pending;
      return route.fulfill({ status: 503, json: { error: { code: 'service_unavailable', request_id: '0123456789abcdef01234567' } } });
    }
    return route.fulfill({ json: { outcome: 'success', public_id: 'abcdef0123456789' } });
  });
  await page.getByRole('button', { name: 'New API token' }).click();
  await page.getByLabel('Token name').fill('new-operator');
  await page.getByRole('checkbox', { name: /Allowed environments Production/ }).check();
  const submit = page.locator('.token-form button[type="submit"]');
  await submit.click();
  await expect.poll(() => operations.length).toBe(1);
  await expect(submit).toBeDisabled();
  await expect(page.getByLabel('Token name')).toBeDisabled();
  release();
  await expect(submit).toBeEnabled();
  await expect(page.getByRole('alert')).toContainText('0123456789abcdef01234567');
  await submit.click();
  await expect(page.getByRole('alert')).toContainText('already completed');
  await expect(page.getByRole('alert')).toContainText('abcdef0123456789');
  expect(operations).toHaveLength(2);
  expect(operations[1]).toBe(operations[0]);
});

test('CA mutation errors retain a copyable audit request identifier', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await workspace(page);
  await page.route('**/v1/certificate-authorities', route => route.fulfill({ status: 422, json: { error: { code: 'validation_failed', request_id: '0123456789abcdef01234567' } } }));
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  await page.getByRole('button', { name: 'Create CA', exact: true }).click();
  await page.getByLabel('Authority name', { exact: true }).fill('New CA');
  await page.getByRole('button', { name: 'Generate certificate authority' }).click();
  await expect(page.getByRole('alert')).toContainText('0123456789abcdef01234567');
  await page.getByRole('button', { name: 'Copy Request ID 0123456789abcdef01234567', exact: true }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('0123456789abcdef01234567');
});

test('full CA identifiers fit a narrow screen and tabs scroll within their own strip', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await workspace(page);
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  await expect(page.getByText(ca.fingerprint_sha256, { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await page.getByRole('tab', { name: 'Deployment status', exact: true }).click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

test('returning to the page refreshes a Token that expired while the page was idle', async ({ page }) => {
  await workspace(page, { tokens: [{ ...token, expires_at: '2026-10-15T00:01:00Z' }] });
  const row = page.getByRole('row', { name: /old-operator/ });
  await expect(row.locator('.status')).toHaveText('Active');
  await page.clock.setFixedTime(new Date('2026-10-15T00:02:00Z'));
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
  await expect(row.locator('.status')).toHaveText('Expired');
});

test.describe('elapsed-time validity across daylight saving changes', () => {
  test.use({ timezoneId: 'America/New_York' });
  test('two calendar days across fall-back are 49 hours and exceed a two-day policy', async ({ page }) => {
    await workspace(page, { maxDays: 2 });
    await page.clock.setFixedTime(new Date('2026-10-31T16:00:00Z'));
    await page.getByRole('button', { name: 'New API token' }).click();
    await page.getByLabel('Scoped write token', { exact: true }).check();
    const expiry = page.getByLabel('Expires at', { exact: true });
    await expiry.fill('2026-11-02T12:00');
    await expect(page.locator('.expiry-summary')).toContainText('2 days 1 hour');
    await expect(page.locator('.expiry-summary')).toContainText('Exceeds the 2-day limit by 1 hour');
    await page.getByRole('button', { name: 'Use maximum (2 days)', exact: true }).click();
    await expect(expiry).toHaveValue('2026-11-02T11:00');
    await expect(page.locator('.expiry-summary')).toHaveText('Selected lifetime: 2 days');
  });
});
