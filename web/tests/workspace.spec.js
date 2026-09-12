import { expect, test } from '@playwright/test';

test('resource workspace provides keyboard search, bounded fields, and light/dark/mobile layouts', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const items = [
    { namespace_key: 'platform', key: 'postgres', display_name: 'Application database', revision: 12, environment_keys: ['production', 'staging'], archived: false },
    { namespace_key: 'platform', key: 'redis', display_name: 'Cache connection', revision: 4, environment_keys: ['production'], archived: false },
    { namespace_key: 'cloud', key: 'storage', display_name: 'Object storage', revision: 2, environment_keys: ['production'], archived: false },
  ];
  const fields = [{ key: 'username', name: 'Username', type: 'text' }, { key: 'password', name: 'Password', type: 'secret' }, ...Array.from({ length: 118 }, (_, index) => ({ key: `field${index + 3}`, name: `Field ${index + 3}`, type: 'text' }))];
  const metadata = { ...items[0], snapshot: { fields, variants: [
    { id: '01010101010101010101010101010101', environments: ['production'] },
    { id: '02020202020202020202020202020202', environments: ['staging'] },
  ] } };
  await page.route('**/v1/**', async route => {
    const path = new URL(route.request().url()).pathname;
    let body = { items: [] };
    if (path === '/v1/me') body = { subject: 'operator', email: 'alex@example.com', role: 'admin' };
    if (path === '/v1/environments') body = { items: [{ key: 'production', display_name: 'Production' }, { key: 'staging', display_name: 'Staging' }] };
    if (path === '/v1/vault-items') body = { items };
    if (path === '/v1/configs') body = { items: [{ key: 'payments', display_name: 'Payments service', environments: [] }] };
    if (path.endsWith('/postgres')) body = metadata;
    if (path.endsWith('/postgres/revisions')) body = { items: [12, 11, 10, 9].map(revision => ({ revision, actor_id: 'alex@example.com', created_at: '2026-09-10T10:30:00Z' })) };
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
  await page.goto('/ui/#/vault/platform/postgres');
  await expect(page.getByRole('heading', { name: 'Application database' })).toBeVisible();
  await expect(page.locator('.resource-rail-items a')).toHaveCount(3);
  await expect(page.locator('.field-table .field-row')).toHaveCount(25);
  await page.getByRole('searchbox', { name: /Search Fields/i }).fill('field119');
  await expect(page.locator('.field-table .field-row')).toHaveCount(1);
  await page.getByRole('searchbox', { name: /Search Fields/i }).fill('');
  await page.keyboard.press('ControlOrMeta+k');
  await expect(page.getByRole('combobox', { name: 'Workspace search' })).toBeFocused();
  await page.getByRole('combobox', { name: 'Workspace search' }).fill('redis');
  await expect(page.getByRole('option')).toHaveCount(1);
  await page.keyboard.press('Escape');
  if (process.env.CONFIGRA_WORKSPACE_SCREENSHOTS) await page.screenshot({ path: `${process.env.CONFIGRA_WORKSPACE_SCREENSHOTS}/vault-light.png`, animations: 'disabled' });
  await page.getByRole('button', { name: /dark theme/i }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  if (process.env.CONFIGRA_WORKSPACE_SCREENSHOTS) await page.screenshot({ path: `${process.env.CONFIGRA_WORKSPACE_SCREENSHOTS}/vault-dark.png`, animations: 'disabled' });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByRole('heading', { name: 'Application database' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Application database' })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (process.env.CONFIGRA_WORKSPACE_SCREENSHOTS) await page.screenshot({ path: `${process.env.CONFIGRA_WORKSPACE_SCREENSHOTS}/vault-mobile.png`, animations: 'disabled' });
});
