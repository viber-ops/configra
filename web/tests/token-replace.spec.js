import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

const day = 86400000;
const inDays = days => new Date(Date.now() + days * day).toISOString();
const environments = ['canary', 'development', 'production', 'staging'].map(key => ({ key, display_name: key }));

function tokenFixtures() {
  return [
    {
      public_id: 'aaaaaaaaaaaaaaaa', display_name: 'Edge readers', display_prefix: 'cfg_aaaaaaaaaaaaaaaa', kind: 'read-only',
      environment_keys: ['canary', 'development', 'production'], environment_count: 4, allow_without_mtls: true,
      expires_at: inDays(5), revoked: false, created_at: '2026-08-01T00:00:00Z',
    },
    {
      public_id: 'bbbbbbbbbbbbbbbb', display_name: 'Deploy writer', display_prefix: 'cfg_bbbbbbbbbbbbbbbb', kind: 'write-scoped',
      environment_keys: ['production'], environment_count: 1, allow_without_mtls: false, config_keys: ['payment'], namespace_keys: [],
      expires_at: inDays(40), revoked: false, created_at: '2026-09-01T00:00:00Z',
    },
    {
      public_id: 'cccccccccccccccc', display_name: 'Host credential', display_prefix: 'cfg_cccccccccccccccc', kind: 'read-only',
      parent_public_id: 'bbbbbbbbbbbbbbbb', environment_keys: ['production'], environment_count: 1, allow_without_mtls: false,
      expires_at: inDays(20), revoked: false, created_at: '2026-09-02T00:00:00Z',
    },
    {
      public_id: 'dddddddddddddddd', display_name: 'Old batch', display_prefix: 'cfg_dddddddddddddddd', kind: 'read-only',
      environment_keys: ['staging'], environment_count: 1, allow_without_mtls: false,
      expires_at: new Date(Date.now() - 2 * day).toISOString(), revoked: false, created_at: '2026-06-01T00:00:00Z',
    },
  ];
}

const grantsByToken = {
  aaaaaaaaaaaaaaaa: ['canary', 'development', 'production', 'staging'],
  bbbbbbbbbbbbbbbb: ['production'],
};

async function mockTokens(page, { role = 'admin', mfa = true } = {}) {
  const tokens = tokenFixtures();
  const calls = { creates: [], revokes: [], lists: [] };
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: `${role}-1`, email: `${role}@example.com`, role, mfa_verified: mfa } }));
  await page.route(/\/v1\/(configs|vault-items)(\?|$)/, route => route.fulfill({ body: inventoryPage(route, []) }));
  await page.route('**/v1/audit*', route => route.fulfill({ json: { items: [] } }));
  await page.route('**/health/ready', route => route.fulfill({ status: 204 }));
  await page.route(/\/v1\/environments\?/, route => {
    const query = new URL(route.request().url()).searchParams;
    const granted = grantsByToken[query.get('token')] || [];
    return route.fulfill({ body: inventoryPage(route, environments.map(item => ({ ...item, granted: granted.includes(item.key) }))) });
  });
  await page.route('**/v1/api-tokens**', async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === 'POST' && url.pathname === '/v1/api-tokens') {
      const body = request.postDataJSON();
      calls.creates.push(body);
      const created = { outcome: 'success', public_id: 'eeeeeeeeeeeeeeee', display_prefix: 'cfg_eeeeeeeeeeeeeeee', token: 'cfg_eeeeeeeeeeeeeeee_new-secret' };
      tokens.unshift({ ...body, public_id: created.public_id, display_prefix: created.display_prefix, environment_count: body.environment_keys.length, revoked: false, created_at: new Date().toISOString(), expires_at: body.expires_at || null });
      return route.fulfill({ json: created });
    }
    if (request.method() === 'POST' && url.pathname.endsWith('/revoke')) {
      const publicID = url.pathname.split('/')[3];
      calls.revokes.push({ publicID, operation: request.headers()['idempotency-key'] });
      tokens.find(token => token.public_id === publicID).revoked = true;
      return route.fulfill({ json: { outcome: 'success', public_id: publicID, revoked: true } });
    }
    calls.lists.push(url.search);
    const window = Number(url.searchParams.get('expiring_within_days') || 0);
    const key = url.searchParams.get('key');
    const visible = tokens.filter(token => (!window || (token.expires_at && Date.parse(token.expires_at) <= Date.now() + window * day)) && (!key || token.public_id === key));
    return route.fulfill({ body: inventoryPage(route, visible) });
  });
  return calls;
}

test('replacing a read-only Token copies its full access and then retires the old Token', async ({ page }) => {
  const calls = await mockTokens(page);
  await page.goto('/ui/#/administration');
  await page.getByRole('button', { name: 'Replace Edge readers' }).click();
  await expect(page.getByRole('heading', { name: 'Replace Edge readers · cfg_aaaaaaaaaaaaaaaa' })).toBeVisible();
  await expect(page.getByLabel('Token name')).toHaveValue('Edge readers');
  // The list previews three Environments; the replacement loads all four grants.
  for (const key of ['canary', 'development', 'production', 'staging']) {
    await expect(page.getByRole('checkbox', { name: `Allowed environments ${key} ${key}` })).toBeChecked();
  }
  await expect(page.getByRole('checkbox', { name: 'Allow this token without mTLS' })).toBeChecked();
  await page.getByRole('button', { name: 'Create API token' }).click();

  expect(calls.creates).toHaveLength(1);
  expect(calls.creates[0]).toMatchObject({ display_name: 'Edge readers', environment_keys: ['canary', 'development', 'production', 'staging'], allow_without_mtls: true, never_expires: false });
  expect(calls.creates[0].kind).toBeUndefined();
  await expect(page.getByText('cfg_eeeeeeeeeeeeeeee_new-secret')).toBeVisible();
  const steps = page.getByRole('region', { name: 'Finish replacing Edge readers · cfg_aaaaaaaaaaaaaaaa' });
  await expect(steps).toContainText('Give the new Token to every consumer');
  await expect(steps).not.toContainText('deployment credential');

  await steps.getByRole('button', { name: 'Revoke Edge readers · cfg_aaaaaaaaaaaaaaaa' }).click();
  expect(calls.revokes).toHaveLength(0);
  await steps.getByRole('button', { name: 'Confirm revoke Edge readers · cfg_aaaaaaaaaaaaaaaa' }).click();
  await expect(steps.getByRole('status')).toHaveText('Edge readers · cfg_aaaaaaaaaaaaaaaa was revoked.');
  expect(calls.revokes).toEqual([{ publicID: 'aaaaaaaaaaaaaaaa', operation: expect.stringMatching(/^[0-9a-f-]{36}$/) }]);
});

test('replacing a write-scoped Token keeps its exact scopes and warns about deployment credentials', async ({ page }) => {
  const calls = await mockTokens(page);
  await page.goto('/ui/#/administration');
  await page.getByRole('button', { name: 'Replace Deploy writer' }).click();
  await expect(page.getByRole('checkbox', { name: 'Scoped write token' })).toHaveCount(0);
  const fixed = page.locator('.replace-fixed');
  await expect(fixed).toContainText('production');
  await expect(fixed).toContainText('payment');
  await expect(fixed).toContainText('∅');
  await page.getByRole('button', { name: '7 days' }).click();
  await page.getByRole('button', { name: 'Create API token' }).click();

  expect(calls.creates).toHaveLength(1);
  // An explicit empty Namespace list denies everything; it must not be widened by omitting it.
  expect(calls.creates[0]).toMatchObject({ kind: 'write-scoped', environment_keys: ['production'], config_keys: ['payment'], namespace_keys: [], allow_without_mtls: false });
  expect(calls.creates[0].expires_at).toBeTruthy();
  await expect(page.locator('.replacement-cascade')).toContainText('also revokes every deployment credential');
});

test('deployment credentials are not offered for replacement in the UI', async ({ page }) => {
  await mockTokens(page);
  await page.goto('/ui/#/administration');
  await expect(page.getByRole('button', { name: 'Revoke Host credential' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Replace Host credential' })).toHaveCount(0);
});

test('Tokens that expire soon are marked and can be listed on their own', async ({ page }) => {
  const calls = await mockTokens(page);
  await page.goto('/ui/#/administration');
  await expect(page.getByRole('row', { name: /Edge readers/ }).locator('.status')).toHaveText('Expires soon');
  await expect(page.getByRole('row', { name: /Old batch/ }).locator('.status')).toHaveText('Expired');
  await expect(page.getByRole('row', { name: /Deploy writer/ }).locator('.status')).toHaveText('Active');
  await page.getByLabel('Status').selectOption('expiring');
  await expect(page.getByRole('row', { name: /Deploy writer/ })).toHaveCount(0);
  await expect(page.getByRole('row', { name: /Edge readers/ })).toBeVisible();
  await expect(page.getByRole('row', { name: /Old batch/ })).toBeVisible();
  expect(calls.lists.at(-1)).toContain('expiring_within_days=14');
  expect(calls.lists.at(-1)).toContain('status=active');
});

test('the Overview lists Tokens expiring soon and opens their replacement', async ({ page }) => {
  await mockTokens(page);
  await page.goto('/ui/#/overview');
  const card = page.getByRole('region', { name: 'API Tokens expiring soon' });
  await expect(card.getByRole('listitem')).toHaveCount(2);
  await expect(card.getByRole('listitem').first()).toContainText('Old batch');
  await expect(card.getByRole('link', { name: 'View all 2' })).toHaveAttribute('href', '#/administration?tokens=expiring');
  await card.getByRole('link', { name: 'Replace Edge readers' }).click();
  await expect(page.getByRole('heading', { name: 'Replace Edge readers · cfg_aaaaaaaaaaaaaaaa' })).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.location.hash)).toBe('#/administration');
});

test('viewers see no Token expiry card', async ({ page }) => {
  const calls = await mockTokens(page, { role: 'viewer' });
  await page.goto('/ui/#/overview');
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'API Tokens expiring soon' })).toHaveCount(0);
  expect(calls.lists).toHaveLength(0);
});
