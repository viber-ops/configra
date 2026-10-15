import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

async function sourcePage(page) {
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/(configs|vault-items)(\?|$)/, route => route.fulfill({ body: inventoryPage(route, []) }));
  await page.route(/\/v1\/environments\?/, route => route.fulfill({ body: inventoryPage(route, [
    { key: 'test', display_name: 'Test', revision: 3 }, { key: 'production', display_name: 'Production' }, { key: 'retired', display_name: 'Retired', archived: true },
  ]) }));
  await page.route('**/v1/environments/test/configs/server**', route => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/revisions')) return route.fulfill({ json: { items: [{ revision: 3, format: 'yaml', created_at: '2026-10-15T00:00:00Z' }] } });
    return route.fulfill({ json: { environment_key: 'test', config_key: 'server', config_name: 'Server', format: 'yaml', content: "port: 6380\npassword: '{vault.app.db.password}'\n", revision: 3 } });
  });
  await page.goto('/ui/#/configs/server/test');
}

async function openClone(page) {
  await page.locator('summary[aria-label="More actions"]').click();
  await page.getByRole('button', { name: 'Clone', exact: true }).click();
}

test('copy to an empty environment pins the saved source and shows the resulting revision and Vault warnings', async ({ page }) => {
  await sourcePage(page);
  let mutation;
  await page.route('**/v1/environments/production/configs/server/replace', async route => {
    mutation = { body: route.request().postDataJSON(), id: route.request().headers()['idempotency-key'] };
    await route.fulfill({ json: { outcome: 'success', revision: 1, warnings: [{ code: 'missing_vault_environment_variant', namespace_key: 'app', item_key: 'db', field_key: 'password' }] } });
  });
  await page.getByRole('button', { name: 'Copy to environment', exact: true }).click();
  await page.getByLabel('Target environment', { exact: true }).selectOption('production');
  await expect(page.getByLabel('Target environment', { exact: true }).getByRole('option', { name: /Retired/ })).toHaveCount(0);
  await expect(page.getByText('test/server@v3', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Create first version in production' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Copied to production/server · v1' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open target config' })).toHaveAttribute('href', '#/configs/server/production');
  await expect(page.getByText('No matching Environment variant')).toBeVisible();
  expect(mutation.body).toEqual({ source_environment: 'test', source_config: 'server', source_revision: 3, target_revision: 0, expected_target_revision: 0 });
  expect(mutation.id).toMatch(/^[0-9a-f-]{36}$/);
});

test('copy rejection explains an existing target and keeps a copyable Request ID', async ({ page }) => {
  await sourcePage(page);
  await page.route('**/v1/environments/production/configs/server/replace', route => route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', field: 'expected_target_revision', reason: 'target_config_exists', request_id: 'copy-conflict-request' } } }));
  await page.getByRole('button', { name: 'Copy to environment', exact: true }).click();
  await page.getByLabel('Target environment', { exact: true }).selectOption('production');
  await page.getByRole('button', { name: 'Create first version in production' }).click();
  await expect(page.getByRole('alert')).toContainText('already has this Config');
  await expect(page.getByRole('alert')).toContainText('Merge or Replace');
  await expect(page.getByRole('button', { name: 'Copy Request ID copy-conflict-request' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open target config' })).toHaveAttribute('href', '#/configs/server/production');
});

test('clone explains global name collisions and retries a lost response with the same operation ID', async ({ page }) => {
  await sourcePage(page);
  const attempts = [];
  await page.route('**/v1/environments/test/configs/server/clone', route => {
    attempts.push({ body: route.request().postDataJSON(), id: route.request().headers()['idempotency-key'] });
    if (attempts.length === 1) return route.abort('failed');
    return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', field: 'target_config', reason: 'clone_target_exists', request_id: 'clone-conflict-request' } } });
  });
  await openClone(page);
  await expect(page.getByText(/new Config key across all environments/)).toBeVisible();
  await page.getByLabel('Target environment', { exact: true }).selectOption('production');
  await page.getByLabel('Target config', { exact: true }).fill('server');
  await page.getByLabel('Target name', { exact: true }).fill('Server copy');
  await page.getByRole('button', { name: 'Clone config', exact: true }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await page.getByRole('button', { name: 'Clone config', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('Copy to environment');
  await expect(page.getByRole('alert')).toContainText('clone-conflict-request');
  await expect(page.getByRole('alert')).toContainText(attempts[0].id);
  expect(attempts[0].id).toBe(attempts[1].id);
  expect(attempts[0].body.source_revision).toBe(3);
  await page.getByLabel('Target config', { exact: true }).fill('server_copy');
  await page.getByRole('button', { name: 'Clone config', exact: true }).click();
  expect(attempts[2].id).not.toBe(attempts[1].id);
});

test('transfer preserves preview errors and retries, then refreshes a stale target explicitly', async ({ page }) => {
  await sourcePage(page);
  let previews = 0;
  const attempts = [];
  await page.route('**/v1/environments/production/configs/server**', route => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/revisions')) return route.fulfill({ json: { items: [{ revision: 2, format: 'yaml', created_at: '2026-10-15T00:00:00Z' }] } });
    if (path.endsWith('/transfer-preview')) {
      previews++;
      if (previews === 1) return route.fulfill({ status: 503, json: { error: { code: 'service_unavailable', request_id: 'preview-unavailable' } } });
      return route.fulfill({ json: { format: 'yaml', content: 'port: 6380\n', expected_target_revision: previews === 2 ? 2 : 3 } });
    }
    if (path.endsWith('/replace')) {
      attempts.push({ body: route.request().postDataJSON(), id: route.request().headers()['idempotency-key'] });
      if (attempts.length === 1) return route.abort('failed');
      if (attempts.length === 2) return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', reason: 'target_revision_changed', request_id: 'target-changed' } } });
      return route.fulfill({ json: { outcome: 'success', revision: 4 } });
    }
    return route.fulfill({ json: { environment_key: 'production', config_key: 'server', config_name: 'Server', revision: 2, format: 'yaml', content: 'port: 80\n' } });
  });
  await page.getByRole('button', { name: 'Replace', exact: true }).click();
  await page.getByTestId('revision-test-3').getByRole('button', { name: /as Source/ }).click();
  await page.getByTestId('revision-production-2').getByRole('button', { name: /as Target/ }).click();
  await expect(page.getByRole('alert')).toContainText('preview-unavailable');
  await page.getByRole('button', { name: 'Refresh preview' }).click();
  await page.getByRole('button', { name: 'Replace production' }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await page.getByRole('button', { name: 'Replace production' }).click();
  await expect(page.getByRole('alert')).toContainText('target-changed');
  await expect(page.getByRole('alert')).toContainText('Refresh');
  expect(attempts[0].id).toBe(attempts[1].id);
  await page.getByRole('button', { name: 'Refresh preview' }).click();
  await expect(page.locator('.transfer-conflict-hint')).toContainText('v3');
  await page.getByRole('button', { name: 'Replace production' }).click();
  await expect(page).toHaveURL(/#\/configs\/server\/production$/);
  expect(attempts[2].id).not.toBe(attempts[1].id);
  expect(attempts[2].body.expected_target_revision).toBe(3);
});

test('restore disables duplicate submission and reuses its operation ID after a lost response', async ({ page }) => {
  await sourcePage(page);
  await page.route('**/v1/environments/test/configs/server/revisions*', route => route.fulfill({ json: { items: [3, 2].map(revision => ({ revision, format: 'yaml', created_at: '2026-10-15T00:00:00Z' })) } }));
  const attempts = [];
  let release;
  await page.route('**/v1/environments/test/configs/server/restore', async route => {
    attempts.push(route.request().headers()['idempotency-key']);
    if (attempts.length === 1) { await new Promise(resolve => { release = resolve; }); return route.abort('failed'); }
    return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', reason: 'target_revision_changed', request_id: 'restore-conflict' } } });
  });
  await page.reload();
  await page.getByRole('tab', { name: 'History', exact: true }).click();
  await page.getByRole('button', { name: 'Restore v2', exact: true }).click();
  const confirm = page.getByRole('button', { name: 'Confirm restore v2', exact: true });
  await confirm.click();
  await expect.poll(() => attempts.length).toBe(1);
  try { await expect(confirm).toBeDisabled(); } finally { release(); }
  await expect(page.getByRole('alert')).toBeVisible();
  await confirm.click();
  await expect(page.getByRole('alert')).toContainText('restore-conflict');
  expect(attempts[0]).toBe(attempts[1]);
});


test('copy form fits a narrow screen and explains the saved source in Chinese', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await sourcePage(page);
  await page.getByRole('button', { name: '中文', exact: true }).click();
  await page.getByRole('button', { name: '复制到环境', exact: true }).click();
  await page.getByLabel('目标 Environment', { exact: true }).selectOption('production');
  await expect(page.getByText(/复制已保存的 v3/)).toBeVisible();
  await expect(page.getByRole('button', { name: '在 production 创建首个版本' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
  await page.screenshot({ path: test.info().outputPath('copy-mobile.png'), fullPage: true });
});

test('restore waits for the complete snapshot before allowing a copy of its new revision', async ({ page }) => {
  await sourcePage(page);
  let restored = false;
  let reloadWaiting = false;
  let release;
  await page.route('**/v1/environments/test/configs/server**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/restore')) { restored = true; return route.fulfill({ json: { outcome: 'success', revision: 4 } }); }
    if (path.endsWith('/revisions')) return route.fulfill({ json: { items: [3, 2].map(revision => ({ revision, format: 'yaml', created_at: '2026-10-15T00:00:00Z' })) } });
    if (restored) { reloadWaiting = true; await new Promise(resolve => { release = resolve; }); }
    return route.fulfill({ json: { environment_key: 'test', config_key: 'server', config_name: 'Server', revision: restored ? 4 : 3, format: 'yaml', content: restored ? 'restored: true\n' : 'old: true\n' } });
  });
  await page.reload();
  await page.getByRole('tab', { name: 'History', exact: true }).click();
  await page.getByRole('button', { name: 'Restore v2', exact: true }).click();
  await page.getByRole('button', { name: 'Confirm restore v2', exact: true }).click();
  await expect.poll(() => reloadWaiting).toBe(true);
  try { await expect(page.getByRole('button', { name: 'Copy to environment', exact: true })).toHaveCount(0); } finally { release(); }
  await page.getByRole('button', { name: 'Copy to environment', exact: true }).click();
  await expect(page.getByText('test/server@v4', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Source', { exact: true })).toContainText('restored: true');
  await expect(page.getByLabel('Source', { exact: true })).not.toContainText('old: true');
});
