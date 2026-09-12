import { expect, test } from '@playwright/test';

const principal = { issuer: 'https://identity.example.com', subject: 'admin', email: 'admin@example.com', role: 'admin' };

test('Vault navigation requires a new explicit reveal for each item', async ({ page }) => {
  const metadata = key => ({
    namespace_key: 'platform', key, display_name: key === 'redis' ? 'Redis credentials' : 'Postgres credentials', revision: 1, archived: false,
    snapshot: { fields: [{ key: 'password', name: 'Password', type: 'secret' }], variants: [{ id: '01010101010101010101010101010101', environments: ['production'] }] },
  });
  await page.route('**/v1/**', async route => {
    const path = new URL(route.request().url()).pathname;
    let body = { items: [] };
    if (path === '/v1/me') body = principal;
    else if (path.startsWith('/v1/vault-items/platform/')) {
      const key = path.split('/')[4];
      if (path.endsWith('/revisions')) body = { items: [{ revision: 1, actor_id: 'admin', created_at: '2026-09-11T00:00:00Z' }] };
      else if (!path.endsWith('/usages')) {
        body = metadata(key);
        if (path.endsWith('/values')) body.snapshot.variants[0].values = { password: { text: `private-value-for-${key}` } };
      }
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
  await page.goto('/ui/#/vault/platform/redis');
  await page.getByRole('button', { name: 'Reveal item values' }).click();
  await expect(page.getByLabel('Password', { exact: true })).toHaveValue('private-value-for-redis');
  await page.evaluate(() => { location.hash = '#/vault/platform/postgres'; });
  await expect(page.getByRole('heading', { name: 'Postgres credentials' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Reveal item values' })).toBeVisible();
  await expect(page.getByLabel('Password', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Reveal item values' }).click();
  await expect(page.getByLabel('Password', { exact: true })).toHaveValue('private-value-for-postgres');
});

test('API token creation uses the server expiry default unless explicitly overridden', async ({ page }) => {
  let created;
  await page.route('**/v1/**', async route => {
    const path = new URL(route.request().url()).pathname;
    let body = path === '/v1/me' ? principal : { items: [] };
    if (path === '/v1/api-tokens' && route.request().method() === 'POST') {
      created = route.request().postDataJSON();
      body = { outcome: 'success', token: 'one-time-test-token' };
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
  await page.goto('/ui/#/administration');
  await page.getByRole('button', { name: 'New API token' }).click();
  await expect(page.getByRole('checkbox', { name: 'Never expires' })).not.toBeChecked();
  await page.getByLabel('Token name').fill('Default expiry');
  await page.getByRole('button', { name: 'Create API token' }).click();
  await expect.poll(() => created).toBeTruthy();
  expect(created.never_expires).toBe(false);
  expect(created.expires_at).toBeUndefined();
});
