import fs from 'node:fs';
import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

const file = `namespace: order_service
item: clients-conf
display_name: order_service_clients_conf
environments: [hk47, hk180]
fields:
  - key: lvl-download
    name: LVL download
    type: text
    value: "https://lvl.example.com/api/v1/download/start"
  - key: google-login
    name: Google login
    type: secret
    value: "s3cr3t-sentinel"
  - key: lvl-data
    type: text
    value: "https://lvl.example.com/api/v1/lvl"
`;
const draft = {
  namespace_key: 'order_service', item_key: 'clients-conf', display_name: 'order_service_clients_conf', environments: ['hk47', 'hk180'],
  fields: [{ key: 'lvl-download', name: 'LVL download', type: 'text' }, { key: 'google-login', name: 'Google login', type: 'secret' }, { key: 'lvl-data', name: 'lvl-data', type: 'text' }],
  values: { 'lvl-download': 'https://lvl.example.com/api/v1/download/start', 'google-login': 's3cr3t-sentinel', 'lvl-data': 'https://lvl.example.com/api/v1/lvl' },
};

async function vaultPage(page, { role = 'admin', preview = () => ({ problems: [], draft }) } = {}) {
  const calls = { previews: [], writes: [] };
  let created = null;
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role } }));
  await page.route(/\/v1\/environments(\?|$)/, route => route.fulfill({ body: inventoryPage(route, [
    { key: 'hk180', display_name: 'HK 180', archived: false }, { key: 'hk47', display_name: 'HK 47', archived: false },
  ]) }));
  await page.route('**/v1/vault-items**', route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname === '/v1/vault-items/import-preview') {
      calls.previews.push(request.postDataJSON().content);
      return route.fulfill({ json: preview(request.postDataJSON().content) });
    }
    if (request.method() === 'PUT') {
      created = request.postDataJSON();
      calls.writes.push({ path: pathname, body: created });
      return route.fulfill({ json: { outcome: 'success', revision: 1 } });
    }
    if (pathname === '/v1/vault-items/order_service/clients-conf') {
      return route.fulfill({ json: { namespace_key: 'order_service', key: 'clients-conf', display_name: created.display_name, revision: 1, archived: false,
        snapshot: { fields: created.snapshot.fields, variants: created.snapshot.variants.map(({ id, environments }) => ({ id, environments })) } } });
    }
    if (pathname.startsWith('/v1/vault-items/order_service/clients-conf/')) return route.fulfill({ json: { items: [], total: 0 } });
    return route.fulfill({ body: inventoryPage(route, []) });
  });
  await page.goto('/ui/#/vault');
  return calls;
}

const panel = page => page.getByRole('region', { name: 'Import a Vault Item from YAML' });

test('a pasted file fills the create form for review, then one create writes the whole Item and opens it', async ({ page }) => {
  const calls = await vaultPage(page);
  await page.getByRole('button', { name: 'Import YAML' }).click();
  await panel(page).getByLabel('Import file (YAML)').fill(file);
  await panel(page).getByRole('button', { name: 'Check file' }).click();

  // Nothing is written by checking; the ordinary create form now holds the file, in file order.
  await expect(panel(page)).toHaveCount(0);
  await expect(page.getByRole('status').filter({ hasText: 'Loaded order_service.clients-conf' })).toContainText('3 Fields for hk47, hk180');
  const form = page.locator('.vault-structured-form');
  await expect(form.getByLabel('Namespace')).toHaveValue('order_service');
  await expect(form.getByLabel('Resource key')).toHaveValue('clients-conf');
  await expect(form.getByLabel('Display name')).toHaveValue('order_service_clients_conf');
  for (const [index, key] of ['lvl-download', 'google-login', 'lvl-data'].entries()) await expect(form.getByLabel(`Field key ${index + 1}`)).toHaveValue(key);
  await expect(form.getByLabel('Variant 1 Google login value')).toHaveAttribute('type', 'password');
  await expect(form.getByLabel('Variant 1 Google login value')).toHaveValue('s3cr3t-sentinel');
  await expect(form.getByLabel('Variant 1 HK 47 hk47')).toBeChecked();
  await expect(form.getByLabel('Variant 1 HK 180 hk180')).toBeChecked();
  expect(calls.previews).toEqual([file]);
  expect(calls.writes).toHaveLength(0);

  await page.getByRole('button', { name: 'Create Vault item' }).click();
  await expect(page).toHaveURL(/#\/vault\/order_service\/clients-conf$/);
  expect(calls.writes).toHaveLength(1);
  const { path, body } = calls.writes[0];
  expect(path).toBe('/v1/vault-items/order_service/clients-conf');
  expect(body.display_name).toBe('order_service_clients_conf');
  expect(body.expected_revision).toBe(0);
  expect(body.snapshot.fields).toEqual(draft.fields);
  expect(body.snapshot.variants).toHaveLength(1);
  expect(body.snapshot.variants[0].environments).toEqual(['hk47', 'hk180']);
  expect(body.snapshot.variants[0].values).toEqual(Object.fromEntries(Object.entries(draft.values).map(([key, text]) => [key, { text }])));
});

test('an uploaded file is checked exactly like the same text pasted', async ({ page }) => {
  const calls = await vaultPage(page);
  await page.getByRole('button', { name: 'Import YAML' }).click();
  await panel(page).getByLabel('Choose .yml or .yaml file').setInputFiles({ name: 'clients.yaml', mimeType: 'application/yaml', buffer: Buffer.from(file) });
  await expect(panel(page).getByLabel('Import file (YAML)')).toHaveValue(file);
  await panel(page).getByRole('button', { name: 'Check file' }).click();
  await expect(page.locator('.vault-structured-form').getByLabel('Resource key')).toHaveValue('clients-conf');
  expect(calls.previews).toEqual([file]);
});

test('problems are listed by line and path, keep the file and load nothing', async ({ page }) => {
  const calls = await vaultPage(page, { preview: () => ({ problems: [
    { path: 'fields[1].type', line: 12, code: 'unsupported_type' },
    { path: 'fields[2].value', line: 15, code: 'missing' },
    { path: 'environments[1]', line: 4, code: 'environment_archived' },
    { path: 'item', line: 2, code: 'item_exists' },
  ] }) });
  await page.getByRole('button', { name: 'Import YAML' }).click();
  await panel(page).getByLabel('Import file (YAML)').fill(file);
  await panel(page).getByRole('button', { name: 'Check file' }).click();
  const problems = panel(page).getByRole('alert');
  await expect(problems.getByRole('listitem')).toHaveText([
    'line 12fields[1].typeType must be text or secret.',
    'line 15fields[2].valueMissing value. Write "" for an empty value.',
    'line 4environments[1]This Environment is archived.',
    'line 2itemAn Item with this Namespace and key already exists. Import only creates new Items.',
  ]);
  await expect(panel(page).getByLabel('Import file (YAML)')).toHaveValue(file);
  await expect(page.locator('.vault-structured-form')).toHaveCount(0);
  expect(calls.writes).toHaveLength(0);

  // Editing the file clears the old report.
  await panel(page).getByLabel('Import file (YAML)').press('End');
  await panel(page).getByLabel('Import file (YAML)').pressSequentially(' ');
  await expect(problems).toHaveCount(0);
});

test('the template downloads as a ready-to-edit import file', async ({ page }) => {
  await vaultPage(page);
  await page.getByRole('button', { name: 'Import YAML' }).click();
  const [download] = await Promise.all([page.waitForEvent('download'), panel(page).getByRole('button', { name: 'Download template' }).click()]);
  expect(download.suggestedFilename()).toBe('vault-item-import.yaml');
  const text = fs.readFileSync(await download.path(), 'utf8');
  expect(text).toContain('namespace: order_service\nitem: clients-conf\n');
  expect(text).toContain('    type: secret\n');
});

test('a viewer has no import', async ({ page }) => {
  await vaultPage(page, { role: 'viewer' });
  await expect(page.getByRole('heading', { name: 'Vault' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Import YAML' })).toHaveCount(0);
});
