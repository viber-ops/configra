import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

const source = 'database:\n  host: db.internal\n  password: ""\n# shared settings\ncache:\n';
const variant = environments => ({ id: '0123456789abcdef0123456789abcdef', environments });
const vaultItems = {
  'order_service/clients-conf': {
    namespace_key: 'order_service', key: 'clients-conf', display_name: 'Clients', archived: false, revision: 3,
    snapshot: { fields: [{ key: 'google-login', name: 'Google login', type: 'secret' }, { key: 'lvl-data', name: 'Level data', type: 'text' }], variants: [variant(['production'])] },
  },
  'payments/storage': {
    namespace_key: 'payments', key: 'storage', display_name: 'Payment storage', archived: false, revision: 1,
    snapshot: { fields: [{ key: 'bucket', name: 'Bucket', type: 'text' }, { key: 'service-account', name: 'Service account', type: 'file' }], variants: [variant(['staging'])] },
  },
  'legacy/old': {
    namespace_key: 'legacy', key: 'old', display_name: 'Old credentials', archived: true, revision: 2,
    snapshot: { fields: [{ key: 'token', name: 'Token', type: 'secret' }], variants: [variant(['production'])] },
  },
};

async function detailPage(page, { role = 'admin', format = 'yaml', content = source } = {}) {
  const searches = [];
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role } }));
  await page.route(/\/v1\/configs(\?|$)/, route => route.fulfill({ body: inventoryPage(route, [
    { key: 'payment', display_name: 'Payment', archived: false, updated_at: '2026-11-01T00:00:00Z', environments: [{ key: 'production', revision: 2, archived: false }] },
  ]) }));
  await page.route(/\/v1\/vault-items(\?|$)/, route => {
    searches.push(new URL(route.request().url()).searchParams.get('q'));
    return route.fulfill({ body: inventoryPage(route, Object.values(vaultItems).map(({ snapshot, ...item }) => ({
      ...item, updated_at: '2026-11-01T00:00:00Z', environment_keys: snapshot.variants.flatMap(entry => entry.environments),
    }))) });
  });
  await page.route(/\/v1\/vault-items\/[^/?]+\/[^/?]+$/, route => {
    const [, namespace, key] = new URL(route.request().url()).pathname.match(/vault-items\/([^/]+)\/([^/]+)$/);
    return route.fulfill({ json: vaultItems[`${namespace}/${key}`] });
  });
  await page.route(/\/v1\/environments\?/, route => route.fulfill({ body: inventoryPage(route, [{ key: 'production', display_name: 'Production', revision: 2 }]) }));
  await page.route('**/v1/environments/production/configs/payment**', route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/revisions')) return route.fulfill({ json: { items: [2, 1].map(revision => ({ revision, format: 'yaml', created_at: '2026-11-01T00:00:00Z' })) } });
    const revision = url.pathname.match(/revisions\/(\d+)$/)?.[1];
    return route.fulfill({ json: { environment_key: 'production', config_key: 'payment', config_name: 'Payment', format, content: revision === '1' ? 'old: true\n' : content, revision: Number(revision || 2) } });
  });
  await page.goto('/ui/#/configs/payment/production');
  await expect(page.getByLabel('Configuration source').locator('.cm-line').first()).toHaveText(content.split('\n')[0]);
  return searches;
}

const editor = page => page.getByLabel('Configuration source');
async function expectSource(page, value) {
  await expect.poll(async () => (await editor(page).locator('.cm-line').allTextContents()).join('\n')).toBe(value);
}
// Puts the caret at the end of a line, then moves it left.
async function caret(page, line, left = 0) {
  await editor(page).locator('.cm-line').nth(line).click();
  await page.keyboard.press('End');
  for (let step = 0; step < left; step += 1) await page.keyboard.press('ArrowLeft');
}
async function choose(page, item, field) {
  const picker = page.getByRole('region', { name: 'Insert Vault reference' });
  if (!await picker.isVisible()) await page.getByRole('button', { name: 'Insert Vault reference' }).click();
  await picker.getByRole('group', { name: 'Vault Items' }).getByRole('button', { name: new RegExp(item.replace('.', '\\.')) }).click();
  if (field) await picker.getByRole('group', { name: 'Fields' }).getByRole('button', { name: new RegExp(field) }).click();
  return picker;
}

test('a reference goes inside empty quotes as one edit that undo removes', async ({ page }) => {
  await detailPage(page);
  await caret(page, 2, 1);
  const picker = await choose(page, 'order_service.clients-conf', 'google-login');
  await expect(picker.getByText('A Variant covers production.')).toBeVisible();
  await expect(picker.getByLabel('Reference')).toHaveText('{vault.order_service.clients-conf.google-login}');
  await picker.getByRole('button', { name: 'Insert', exact: true }).click();
  await expect(picker).toHaveCount(0);
  await expectSource(page, 'database:\n  host: db.internal\n  password: "{vault.order_service.clients-conf.google-login}"\n# shared settings\ncache:\n');
  await expect(page.getByRole('button', { name: 'Save changes' })).toBeEnabled();

  // The editor has focus again, so undo takes the whole reference back out.
  await page.keyboard.press('ControlOrMeta+z');
  await expectSource(page, source);
});

test('a reference replaces a selected value, or starts a new value after a colon', async ({ page }) => {
  await detailPage(page);
  await caret(page, 1);
  for (let step = 0; step < 'db.internal'.length; step += 1) await page.keyboard.press('Shift+ArrowLeft');
  await (await choose(page, 'order_service.clients-conf', 'lvl-data')).getByRole('button', { name: 'Insert', exact: true }).click();
  await expectSource(page, 'database:\n  host: "{vault.order_service.clients-conf.lvl-data}"\n  password: ""\n# shared settings\ncache:\n');

  await caret(page, 4);
  await (await choose(page, 'order_service.clients-conf', 'google-login')).getByRole('button', { name: 'Insert', exact: true }).click();
  await expectSource(page, 'database:\n  host: "{vault.order_service.clients-conf.lvl-data}"\n  password: ""\n# shared settings\ncache: "{vault.order_service.clients-conf.google-login}"\n');
});

test('a reference is never spliced into a key, a comment, a nested block or the middle of a value', async ({ page }) => {
  await detailPage(page);
  const attempts = [
    async () => { await caret(page, 1, ': db.internal'.length); for (let step = 0; step < 4; step += 1) await page.keyboard.press('Shift+ArrowLeft'); },
    () => caret(page, 3),
    () => caret(page, 0),
    () => caret(page, 1, 4),
  ];
  for (const place of attempts) {
    await place();
    const picker = await choose(page, 'order_service.clients-conf', 'google-login');
    await picker.getByRole('button', { name: 'Insert', exact: true }).click();
    await expect(picker.getByRole('alert')).toHaveText('Put the cursor where a value goes, inside empty quotes, or select a whole value, then insert again.');
    await expectSource(page, source);
  }
});

test('JSON gets the same placement rules', async ({ page }) => {
  const json = '{\n  "host": "",\n  "port": 5432\n}\n';
  await detailPage(page, { format: 'json', content: json });
  await caret(page, 2, ': 5432'.length);
  for (let step = 0; step < '"port"'.length; step += 1) await page.keyboard.press('Shift+ArrowLeft');
  const picker = await choose(page, 'order_service.clients-conf', 'lvl-data');
  await picker.getByRole('button', { name: 'Insert', exact: true }).click();
  await expect(picker.getByRole('alert')).toBeVisible();
  await expectSource(page, json);

  await caret(page, 1, 2);
  await picker.getByRole('button', { name: 'Insert', exact: true }).click();
  await expectSource(page, '{\n  "host": "{vault.order_service.clients-conf.lvl-data}",\n  "port": 5432\n}\n');
});

test('the picker warns about a missing Variant, keeps File Fields out and refuses archived Items', async ({ page }) => {
  const searches = await detailPage(page);
  await caret(page, 2, 1);
  await page.getByRole('button', { name: 'Insert Vault reference' }).click();
  const picker = page.getByRole('region', { name: 'Insert Vault reference' });
  await picker.getByRole('searchbox', { name: 'Search Namespace, Item, name or Field key' }).fill('payments');
  await expect.poll(() => searches.at(-1)).toBe('payments');
  await expect(picker.getByRole('group', { name: 'Vault Items' }).getByRole('button')).toHaveCount(1);

  await choose(page, 'payments.storage');
  await expect(picker.getByText('No Variant covers production yet, so this reference will not resolve there until one does.')).toBeVisible();
  const fields = picker.getByRole('group', { name: 'Fields' });
  await expect(fields.getByRole('button', { name: /Service account/ })).toBeDisabled();
  await expect(picker.getByText('File Fields are read through their own endpoint and cannot be inserted as a value.')).toBeVisible();
  await fields.getByRole('button', { name: /Bucket/ }).click();
  await picker.getByRole('button', { name: 'Insert', exact: true }).click();
  await expectSource(page, 'database:\n  host: db.internal\n  password: "{vault.payments.storage.bucket}"\n# shared settings\ncache:\n');

  await page.getByRole('button', { name: 'Insert Vault reference' }).click();
  await picker.getByRole('searchbox', { name: 'Search Namespace, Item, name or Field key' }).fill('');
  await choose(page, 'legacy.old');
  await expect(picker.getByRole('alert')).toHaveText('This Item is archived, so its references do not resolve. Restore it first.');
  await expect(picker.getByRole('group', { name: 'Fields' }).getByRole('button', { name: /Token/ })).toBeDisabled();
  await expect(picker.getByRole('button', { name: 'Insert', exact: true })).toBeDisabled();
});

test('a past revision has no way to insert a reference', async ({ page }) => {
  await detailPage(page);
  await expect(page.getByRole('button', { name: 'Insert Vault reference' })).toBeVisible();
  await page.locator('.revision-track button', { hasText: 'v1' }).click();
  await expect(page.getByLabel('Revision v1').locator('.cm-line').first()).toHaveText('old: true');
  await expect(page.getByRole('button', { name: 'Insert Vault reference' })).toHaveCount(0);
});

test('a viewer has no way to insert a reference', async ({ page }) => {
  await detailPage(page, { role: 'viewer' });
  await expect(page.getByRole('button', { name: 'Insert Vault reference' })).toHaveCount(0);
});
