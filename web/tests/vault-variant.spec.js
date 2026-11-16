import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

// Adding a Variant to an existing Item: by copying another Variant's values (#66) or from a YAML file (#67).

const fileValue = { filename: 'ca.pem', content_type: 'application/x-pem-file', bytes: 'Q0EtQllURVM=' };
const fields = [
  { key: 'lvl_data', name: 'LVL data', type: 'text' },
  { key: 'google_login', name: 'Google login', type: 'secret' },
  { key: 'lvl_download', name: 'LVL download', type: 'text' },
];
const variantA = { id: 'a'.repeat(32), environments: ['hk47'], values: {
  lvl_data: { text: 'https://lvl.example.com/api/v1/lvl' }, google_login: { text: 'hk47-secret-sentinel' }, lvl_download: { text: '' },
} };
const variantB = { id: 'b'.repeat(32), environments: ['prod', 'staging'], values: {
  lvl_data: { text: 'https://lvl.prod.example.com' }, google_login: { text: 'prod-secret-sentinel' }, lvl_download: { text: 'https://download.prod.example.com' },
} };

async function existingItem(page, { withFile = false, valued: changeValued, preview } = {}) {
  const itemFields = withFile ? [...fields, { key: 'ca', name: 'CA bundle', type: 'file' }] : fields;
  const variants = [variantA, variantB].map(variant => ({ ...variant, values: withFile ? { ...variant.values, ca: { file: fileValue } } : variant.values }));
  const metadata = { namespace_key: 'order_service', key: 'clients-conf', display_name: 'order_service_clients_conf', revision: 7, archived: false,
    snapshot: { fields: itemFields, variants: variants.map(({ id, environments }) => ({ id, environments })) } };
  const valued = { ...metadata, snapshot: { fields: itemFields, variants } };
  changeValued?.(valued);
  const calls = { previews: [], writes: [], valueReads: 0 };
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/environments(\?|$)/, route => route.fulfill({ body: inventoryPage(route, [
    { key: 'hk180', display_name: 'HK 180', archived: false }, { key: 'hk47', display_name: 'HK 47', archived: false },
    { key: 'prod', display_name: 'Production', archived: false }, { key: 'staging', display_name: 'Staging', archived: false },
  ]) }));
  await page.route('**/v1/vault-items**', route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname === '/v1/vault-items/import-preview') {
      calls.previews.push(request.postDataJSON());
      return route.fulfill({ json: preview(request.postDataJSON()) });
    }
    if (pathname.endsWith('/revisions')) return route.fulfill({ json: { items: [{ revision: metadata.revision, actor_id: 'admin@example.com', created_at: '2026-11-13T01:00:00Z' }] } });
    if (pathname.endsWith('/usages') || pathname.endsWith('/impact-preview')) return route.fulfill({ json: { items: [], total: 0 } });
    if (pathname.endsWith('/values')) {
      calls.valueReads += 1;
      return route.fulfill({ json: valued });
    }
    if (request.method() === 'PUT') {
      const body = request.postDataJSON();
      calls.writes.push(body);
      if (calls.conflict) return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', request_id: 'conflict-request' } } });
      metadata.revision += 1;
      metadata.snapshot = { fields: body.snapshot.fields, variants: body.snapshot.variants.map(({ id, environments }) => ({ id, environments })) };
      return route.fulfill({ json: { outcome: 'success', revision: metadata.revision } });
    }
    if (pathname === '/v1/vault-items/order_service/clients-conf') return route.fulfill({ json: metadata });
    return route.fulfill({ body: inventoryPage(route, []) });
  });
  return { calls, variants };
}

test('a new Variant can start as a copy of another Variant, with its own ID and no Environments', async ({ page }) => {
  const { calls, variants } = await existingItem(page, { withFile: true });
  await page.goto('/ui/#/vault/order_service/clients-conf');
  await page.getByRole('button', { name: 'Edit item' }).click();

  // The source list names each Variant with the Environments that share its values.
  const source = page.getByLabel('Values for a new Variant');
  await expect(source.getByRole('option')).toHaveText(['Blank', 'Copy Variant 1 · hk47', 'Copy Variant 2 · prod, staging']);
  await source.selectOption({ label: 'Copy Variant 1 · hk47' });
  await page.getByRole('button', { name: 'Add Variant' }).click();

  // The copy is selected, holds every value (Secret hidden, File kept) and is bound to nothing.
  await expect(page.locator('.vault-editor-variant').nth(2)).toHaveAttribute('aria-pressed', 'true');
  await expect(source).toHaveValue('');
  await expect(page.getByLabel('Variant 3 LVL data value')).toHaveValue('https://lvl.example.com/api/v1/lvl');
  await expect(page.getByLabel('Variant 3 Google login value')).toHaveAttribute('type', 'password');
  await expect(page.getByLabel('Variant 3 Google login value')).toHaveValue('hk47-secret-sentinel');
  await expect(page.locator('.vault-file-editor strong')).toHaveText('ca.pem');
  for (const environment of ['HK 180 hk180', 'HK 47 hk47', 'Production prod', 'Staging staging']) await expect(page.getByLabel(`Variant 3 ${environment}`)).not.toBeChecked();

  // An Environment held by another Variant is labelled. Taking it is announced and can be undone.
  const picker = page.locator('.vault-environment-picker');
  await expect(picker.locator('label', { hasText: 'hk47' })).toContainText('Variant 1');
  await expect(picker.locator('label', { hasText: 'hk180' }).locator('.choice-note')).toHaveCount(0);
  await page.getByLabel('Variant 3 Production prod').check();
  const moved = page.getByRole('status').filter({ hasText: 'prod moved here from Variant 2' });
  await expect(moved).toBeVisible();
  await moved.getByRole('button', { name: 'Undo' }).click();
  await expect(moved).toHaveCount(0);
  await expect(page.getByLabel('Variant 3 Production prod')).not.toBeChecked();
  await expect(picker.locator('label', { hasText: 'prod' })).toContainText('Variant 2');
  // Undo puts the Environment back where it was, so the other Variant is saved unchanged (asserted on the write below).

  await page.getByLabel('Variant 3 HK 180 hk180').check();
  await page.getByLabel('Variant 3 LVL data value').fill('https://lvl-test.example.com/api/v1/lvl');
  await page.getByRole('button', { name: 'Save item' }).click();
  await expect(page.getByText('Saved as v8')).toBeVisible();

  expect(calls.writes).toHaveLength(1);
  const [write] = calls.writes;
  expect(write.expected_revision).toBe(7);
  expect(write.snapshot.variants.slice(0, 2)).toEqual(variants);
  const copy = write.snapshot.variants[2];
  expect(copy.id).toMatch(/^[0-9a-f]{32}$/);
  expect(variants.map(variant => variant.id)).not.toContain(copy.id);
  expect(copy.environments).toEqual(['hk180']);
  expect(copy.values).toEqual({ ...variants[0].values, lvl_data: { text: 'https://lvl-test.example.com/api/v1/lvl' } });
});

test('a blank Variant is still the default, and cancelling a copy writes nothing', async ({ page }) => {
  const { calls } = await existingItem(page);
  await page.goto('/ui/#/vault/order_service/clients-conf');
  await page.getByRole('button', { name: 'Edit item' }).click();
  await page.getByRole('button', { name: 'Add Variant' }).click();
  await expect(page.getByLabel('Variant 3 LVL data value')).toHaveValue('');
  await page.getByLabel('Values for a new Variant').selectOption({ label: 'Copy Variant 2 · prod, staging' });
  await page.getByRole('button', { name: 'Add Variant' }).click();
  await expect(page.getByLabel('Variant 4 LVL data value')).toHaveValue('https://lvl.prod.example.com');
  page.once('dialog', dialog => dialog.accept());
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('button', { name: 'Edit item' })).toBeVisible();
  expect(calls.writes).toHaveLength(0);
});

const file = `namespace: order_service
item: clients-conf
display_name: Another name
environments: [hk180]
fields:
  - {key: lvl_download, type: text, value: "https://download-test.example.com/api/v1/download/start"}
  - {key: lvl_data, name: Renamed, type: text, value: "https://lvl-test.example.com/api/v1/lvl"}
  - {key: google_login, type: secret, value: "hk180-secret-sentinel"}
`;
const imported = {
  namespace_key: 'order_service', item_key: 'clients-conf', display_name: 'order_service_clients_conf', environments: ['hk180'], fields,
  values: { lvl_download: 'https://download-test.example.com/api/v1/download/start', lvl_data: 'https://lvl-test.example.com/api/v1/lvl', google_login: 'hk180-secret-sentinel' },
};
const previewExisting = body => body.target === 'existing_item'
  ? { problems: [], draft: imported, existing: { revision: 7, variants: 2 } }
  : { problems: [{ path: 'item', line: 2, code: 'item_exists' }] };
const panel = page => page.getByRole('region', { name: 'Import a Vault Item from YAML' });

async function checkAsVariant(page) {
  await page.goto('/ui/#/vault');
  await page.getByRole('button', { name: 'Import YAML' }).click();
  await panel(page).getByLabel('Import file (YAML)').fill(file);
  await panel(page).getByRole('button', { name: 'Check file' }).click();
  // An existing Item is offered as a choice, never switched to silently.
  await expect(panel(page).getByRole('alert')).toHaveCount(0);
  await expect(panel(page).getByRole('status')).toContainText('The file can add a new Variant to it instead');
  await panel(page).getByRole('button', { name: 'Add as a new Variant' }).click();
}

test('a file adds a Variant to an existing Item after a preview and a review in the editor', async ({ page }) => {
  const { calls, variants } = await existingItem(page, { preview: previewExisting });
  await checkAsVariant(page);

  const summary = panel(page).getByRole('status');
  await expect(summary).toContainText('Ready to add a Variant to order_service.clients-conf (order_service_clients_conf), now at v7 with 2 Variant(s).');
  await expect(summary).toContainText('Environments for the new Varianthk180');
  await expect(summary).toContainText('Fields with a value from the file3');
  await expect(summary).not.toContainText('sentinel');
  expect(calls.previews).toEqual([{ content: file }, { content: file, target: 'existing_item' }]);
  // The preview neither writes nor reads the Item's values.
  expect(calls.writes).toHaveLength(0);
  expect(calls.valueReads).toBe(0);

  await panel(page).getByRole('button', { name: 'Review in the editor' }).click();
  await expect(page).toHaveURL(/#\/vault\/order_service\/clients-conf$/);
  await expect(page.getByRole('status').filter({ hasText: 'Added a Variant for hk180 from the file, with 3 Fields.' })).toBeVisible();
  // The new Variant is selected. Field names and order are the Item's, not the file's.
  await expect(page.locator('.vault-editor-variant').nth(2)).toHaveAttribute('aria-pressed', 'true');
  for (const [index, key] of ['lvl_data', 'google_login', 'lvl_download'].entries()) await expect(page.getByLabel(`Field key ${index + 1}`)).toHaveValue(key);
  await expect(page.getByLabel('Field name 1')).toHaveValue('LVL data');
  await expect(page.getByLabel('Display name')).toHaveValue('order_service_clients_conf');
  await expect(page.getByLabel('Variant 3 LVL data value')).toHaveValue('https://lvl-test.example.com/api/v1/lvl');
  await expect(page.getByLabel('Variant 3 Google login value')).toHaveAttribute('type', 'password');
  await expect(page.getByLabel('Variant 3 HK 180 hk180')).toBeChecked();
  await expect(page.getByLabel('Variant 3 HK 47 hk47')).not.toBeChecked();
  expect(calls.writes).toHaveLength(0);

  // A concurrent change is a conflict that keeps the draft.
  calls.conflict = true;
  const save = page.getByRole('button', { name: 'Save item' });
  await save.click();
  await expect(page.getByText('This resource changed after you opened it.')).toBeVisible();
  await expect(page.getByLabel('Variant 3 LVL data value')).toHaveValue('https://lvl-test.example.com/api/v1/lvl');
  calls.conflict = false;
  await save.click();
  await expect(page.getByText('Saved as v8')).toBeVisible();

  expect(calls.writes).toHaveLength(2);
  const write = calls.writes[1];
  expect(write.expected_revision).toBe(7);
  expect(write.display_name).toBe('order_service_clients_conf');
  expect(write.snapshot.fields).toEqual(fields);
  expect(write.snapshot.variants.slice(0, 2)).toEqual(variants);
  expect(write.snapshot.variants[2].environments).toEqual(['hk180']);
  expect(write.snapshot.variants[2].values).toEqual(Object.fromEntries(Object.entries(imported.values).map(([key, text]) => [key, { text }])));
  // The saved Item opens on the new Variant.
  await expect(page.getByRole('button', { name: 'Variant 3 · hk180' })).toHaveAttribute('aria-pressed', 'true');
});

test('a file that does not match the existing Item is reported by Field and Environment, and loads nothing', async ({ page }) => {
  const { calls } = await existingItem(page, { preview: body => body.target === 'existing_item'
    ? { problems: [
      { path: 'fields[0].key', line: 6, code: 'field_unknown' },
      { path: 'fields[1].type', line: 7, code: 'field_type_mismatch' },
      { path: 'fields', line: 5, code: 'field_missing', key: 'lvl_download' },
      { path: 'environments[0]', line: 4, code: 'environment_bound' },
      { path: 'item', line: 2, code: 'item_has_file_field', key: 'ca' },
    ] }
    : { problems: [{ path: 'item', line: 2, code: 'item_exists' }] } });
  await checkAsVariant(page);
  await expect(panel(page).getByRole('alert').getByRole('listitem')).toHaveText([
    'line 6fields[0].keyThe Item has no Field with this key. Import does not add Fields to an existing Item.',
    'line 7fields[1].typeThe Item stores this Field with a different type.',
    'line 5fields: lvl_downloadThe Item has this Field, but the file gives it no value. Write "" for an empty value.',
    'line 4environments[0]A Variant of this Item already serves this Environment. Import does not move or overwrite it.',
    'line 2item: caThis Item has a File Field, so a Variant cannot be added from a file. Add the Variant in the editor.',
  ]);
  await expect(panel(page).getByRole('button', { name: 'Review in the editor' })).toHaveCount(0);
  // The choice stays for the next check, and can be changed back.
  await expect(panel(page).getByRole('button', { name: 'Check as a new Variant' })).toBeVisible();
  await panel(page).getByRole('button', { name: 'Check as a new Item' }).click();
  await expect(panel(page).getByRole('button', { name: 'Check file' })).toBeVisible();
  await expect(panel(page).getByLabel('Import file (YAML)')).toHaveValue(file);
  expect(calls.writes).toHaveLength(0);
  expect(calls.valueReads).toBe(0);
});

test('an Item that changed after the check loads nothing and says so', async ({ page }) => {
  const { calls } = await existingItem(page, { preview: previewExisting, valued: valued => { valued.snapshot.variants[1].environments = ['prod', 'hk180']; } });
  await checkAsVariant(page);
  await panel(page).getByRole('button', { name: 'Review in the editor' }).click();
  await expect(page.getByRole('alert').filter({ hasText: 'The Item changed after the file was checked' })).toBeVisible();
  await expect(page.locator('.vault-snapshot-editor')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Edit item' })).toBeVisible();
  expect(calls.writes).toHaveLength(0);
});
