import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';
import { checkEnvironments } from './environment.js';

const environments = [
  { key: 'hk180', display_name: 'HK 180', archived: false },
  { key: 'hk47', display_name: 'HK 47', archived: false },
  { key: 'qa', display_name: 'QA', archived: false },
  { key: 'retired', display_name: 'Retired', archived: true },
];

async function configsPage(page, { failFirstWriteIn, invalid = false } = {}) {
  const writes = [];
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/configs(\?|$)/, route => route.fulfill({ body: inventoryPage(route, []) }));
  await page.route(/\/v1\/environments(\?|$)/, route => {
    const query = new URL(route.request().url()).searchParams;
    // order_service_clients already exists in qa only.
    const rows = query.get('config') ? environments.filter(environment => query.get('config') === 'order_service_clients' && environment.key === 'qa') : environments;
    return route.fulfill({ body: inventoryPage(route, rows.map(environment => ({ ...environment, revision: query.get('config') ? 3 : 0 }))) });
  });
  await page.route('**/v1/configs/validate', route => {
    const { environment } = route.request().postDataJSON();
    if (invalid) return route.fulfill({ status: 422, json: { error: { code: 'config_document_invalid', request_id: 'req-422' } } });
    return route.fulfill({ json: { format: 'yaml', warnings: environment === 'hk47' ? [{ code: 'missing_vault_environment_variant', namespace_key: 'platform', item_key: 'nats', field_key: 'url' }] : [] } });
  });
  await page.route(/\/v1\/environments\/[^/]+\/configs\/[^/?]+$/, route => {
    const environment = new URL(route.request().url()).pathname.split('/')[3];
    writes.push({ environment, body: route.request().postDataJSON(), operation: route.request().headers()['idempotency-key'] });
    if (environment === failFirstWriteIn && writes.filter(write => write.environment === environment).length === 1) {
      return route.fulfill({ status: 503, json: { error: { code: 'service_unavailable', request_id: 'req-503' } } });
    }
    return route.fulfill({ json: { outcome: 'success', revision: 1, warnings: [] } });
  });
  await page.goto('/ui/#/configs');
  await page.getByRole('button', { name: 'New config' }).click();
  return writes;
}

async function fillConfig(page) {
  await page.getByLabel('Resource key').fill('order_service_clients');
  await page.getByLabel('Display name').fill('Order service clients');
  await page.getByLabel('Configuration source').fill('url: "{vault.platform.nats.url}"\n');
}

test('creating in several Environments writes each one, skips conflicts and reports every target', async ({ page }) => {
  const writes = await configsPage(page);
  const picker = page.getByRole('group', { name: 'Environments', exact: true });
  await expect(picker.getByRole('checkbox', { name: / retired$/ })).toBeDisabled();
  await checkEnvironments(page, 'Environments', ['hk180', 'hk47', 'qa']);
  await fillConfig(page);

  const targets = page.getByRole('region', { name: 'Target Environments' });
  await expect(targets).toContainText('Batch create');
  await expect(targets.locator('[data-environment=qa]')).toContainText('Already has this Config');
  await expect(targets.locator('[data-environment=hk47]')).toContainText('platform');
  await expect(targets.locator('[data-environment=hk180]')).toContainText('every Vault reference has a value');
  const submit = page.getByRole('button', { name: 'Create in 2 Environments' });
  await expect(submit).toBeEnabled();
  await submit.click();

  await expect(targets.locator('[data-environment=hk180]')).toContainText('Created v1');
  await expect(targets.locator('[data-environment=hk47]')).toContainText('Created v1');
  await expect(targets.locator('[data-environment=hk47]').getByRole('link', { name: 'Open target config' })).toHaveAttribute('href', '#/configs/order_service_clients/hk47');
  expect(writes.map(write => write.environment).sort()).toEqual(['hk180', 'hk47']);
  expect(writes[0].body).toEqual(writes[1].body);
  expect(writes[0].body).toMatchObject({ name: 'Order service clients', expected_revision: 0, content: 'url: "{vault.platform.nats.url}"\n' });
  expect(writes[0].operation).not.toBe(writes[1].operation);
  await expect(page.getByRole('button', { name: /Create in|Retry failed/ })).toBeDisabled();
});

test('a partial failure keeps the successes and retries only the failed Environment with its first operation', async ({ page }) => {
  const writes = await configsPage(page, { failFirstWriteIn: 'hk47' });
  await checkEnvironments(page, 'Environments', ['hk180', 'hk47']);
  await fillConfig(page);
  await page.getByRole('button', { name: 'Create in 2 Environments' }).click();
  const targets = page.getByRole('region', { name: 'Target Environments' });
  await expect(targets.locator('[data-environment=hk180]')).toContainText('Created v1');
  await expect(targets.locator('[data-environment=hk47]').getByRole('alert')).toBeVisible();

  await page.getByRole('button', { name: 'Retry failed Environments' }).click();
  await expect(targets.locator('[data-environment=hk47]')).toContainText('Created v1');
  const hk47 = writes.filter(write => write.environment === 'hk47');
  expect(hk47).toHaveLength(2);
  expect(hk47[1].operation).toBe(hk47[0].operation);
  expect(writes.filter(write => write.environment === 'hk180')).toHaveLength(1);
});

test('one Environment keeps the single create', async ({ page }) => {
  const writes = await configsPage(page);
  await checkEnvironments(page, 'Environments', ['hk47']);
  await fillConfig(page);
  await expect(page.getByRole('region', { name: 'Target Environments' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Create config' }).click();
  await expect(page.locator('.config-create-form')).toHaveCount(0);
  expect(writes.map(write => write.environment)).toEqual(['hk47']);
});

test('a document that does not validate blocks the whole batch', async ({ page }) => {
  const writes = await configsPage(page, { invalid: true });
  await checkEnvironments(page, 'Environments', ['hk180', 'hk47']);
  await fillConfig(page);
  const targets = page.getByRole('region', { name: 'Target Environments' });
  await expect(targets.getByRole('alert')).toContainText('Check its YAML/JSON syntax');
  await expect(targets.getByRole('alert')).toContainText('req-422');
  await expect(page.getByRole('button', { name: 'Create in 2 Environments' })).toBeDisabled();
  expect(writes).toHaveLength(0);
});
