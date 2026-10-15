import { expect, test } from '@playwright/test';

async function form(page, days = 90) {
  await page.clock.setFixedTime(new Date('2026-10-15T12:00:00Z'));
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin', role: 'admin', write_token_max_ttl_days: days } }));
  await page.route('**/v1/environments*', route => route.fulfill({ json: { items: [{ key: 'production', display_name: 'Production' }], total: 1 } }));
  const writes = [];
  await page.route('**/v1/api-tokens*', route => {
    if (route.request().method() !== 'POST') return route.fulfill({ json: { items: [], total: 0 } });
    writes.push(route.request().postDataJSON());
    return route.fulfill({ json: { outcome: 'success', token: 'one-time-test-token' } });
  });
  await page.goto('/ui/#/administration');
  await page.getByRole('button', { name: 'New API token' }).click();
  await page.getByLabel('Token name').fill('yinxiaoban-prod-operator');
  await page.getByRole('checkbox', { name: /Production/ }).check();
  await page.getByLabel('Scoped write token', { exact: true }).check();
  await page.getByLabel('Config key allowlist').fill('yinxiaoban-server');
  await page.getByLabel('Vault namespace allowlist').fill('yxb');
  return writes;
}

test('the screenshot expiry marks its field before any Token is issued', async ({ page }) => {
  const writes = await form(page);
  const expiry = page.getByLabel('Expires at', { exact: true });
  await expiry.fill('2027-01-14T00:00');
  await page.getByRole('button', { name: 'Create API token', exact: true }).click();
  await expect(expiry).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByRole('alert')).toContainText('90 days');
  expect(writes).toHaveLength(0);
  await expiry.fill('2027-01-12T00:00');
  await page.getByRole('button', { name: 'Create API token', exact: true }).click();
  await expect(page.getByText('one-time-test-token', { exact: true })).toBeVisible();
  expect(writes).toHaveLength(1);
});

test('write token expiry uses the shorter server policy', async ({ page }) => {
  const writes = await form(page, 7);
  await page.getByLabel('Expires at', { exact: true }).fill('2026-10-25T00:00');
  await page.getByRole('button', { name: 'Create API token', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('7 days');
  expect(writes).toHaveLength(0);
});

test('Unicode scope punctuation highlights the allowlist without reflecting it in errors', async ({ page }) => {
  const writes = await form(page);
  const keys = page.getByLabel('Config key allowlist');
  await keys.fill('private–sentinel');
  await page.getByLabel('Expires at', { exact: true }).fill('2026-10-20T00:00');
  await page.getByRole('button', { name: 'Create API token', exact: true }).click();
  await expect(keys).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByRole('alert')).toContainText('ASCII');
  await expect(page.getByRole('alert')).not.toContainText('private');
  expect(writes).toHaveLength(0);
});

test('server validation retains field guidance and a searchable request ID', async ({ page }) => {
  await form(page);
  await page.route('**/v1/api-tokens', route => route.fulfill({ status: 422, json: { error: { code: 'validation_failed', field: 'expires_at', reason: 'write_token_expiry_limit', max_ttl_days: 2, request_id: '0123456789abcdef01234567' } } }));
  await page.getByLabel('Expires at', { exact: true }).fill('2026-10-20T00:00');
  await page.getByRole('button', { name: 'Create API token', exact: true }).click();
  await expect(page.getByLabel('Expires at', { exact: true })).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByRole('alert')).toContainText('2 days');
  await expect(page.getByRole('alert')).toContainText('0123456789abcdef01234567');
});

test('older server failures never claim that an unidentified field was highlighted', async ({ page }) => {
  await form(page);
  await page.route('**/v1/api-tokens', route => route.fulfill({ status: 422, json: { error: { code: 'validation_failed', request_id: '0123456789abcdef01234567', field: 'private-sentinel', reason: 'private-sentinel' } } }));
  await page.getByLabel('Expires at', { exact: true }).fill('2026-10-20T00:00');
  await page.getByRole('button', { name: 'Create API token', exact: true }).click();
  await expect(page.getByRole('alert')).not.toContainText('highlighted');
  await expect(page.getByRole('alert')).not.toContainText('private-sentinel');
  await expect(page.getByRole('alert')).toContainText('expiry');
});

test('an incomplete read-only expiry is rejected instead of issuing the default lifetime', async ({ page }) => {
  const writes = await form(page);
  await page.getByLabel('Scoped write token', { exact: true }).uncheck();
  const expiry = page.getByLabel('Expires at', { exact: true });
  await expiry.fill('2026-10-20T00:00');
  await expiry.press('Delete');
  await page.getByRole('button', { name: 'Create API token', exact: true }).click();
  await expect(expiry).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByRole('alert')).toContainText('valid date');
  expect(writes).toHaveLength(0);
});

test('Chinese validation explains the expiry policy and missing environment selection', async ({ page }) => {
  await form(page, 7);
  await page.evaluate(() => localStorage.setItem('configra-language', 'zh'));
  await page.reload();
  await page.getByRole('button', { name: '新建 API Token' }).click();
  await page.getByLabel('Token 名称').fill('operator');
  await page.getByLabel('有范围的写令牌', { exact: true }).check();
  await page.getByLabel('过期时间', { exact: true }).fill('2026-10-25T00:00');
  await page.getByRole('button', { name: '创建 API Token', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('至少选择一个环境');
  await expect(page.getByRole('group', { name: '允许访问的 Environment' })).toHaveAttribute('aria-invalid', 'true');
  await page.getByRole('checkbox', { name: /Production/ }).check();
  await page.getByRole('button', { name: '创建 API Token', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('7 天');
  await expect(page.getByLabel('过期时间', { exact: true })).toHaveAttribute('aria-invalid', 'true');
});
