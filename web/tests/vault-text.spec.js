import { readFileSync } from 'node:fs';
import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';
import { isVaultEdgeSpace, normalizeVaultText } from '../src/vault-text.js';

// The editor applies the same Text rule as the server (#62); both sides read one set of vectors.
test('the editor and the server share one Vault Text rule', () => {
  const vectors = JSON.parse(readFileSync(new URL('../../internal/vaultdoc/testdata/text-normalization.json', import.meta.url), 'utf8'));
  expect(vectors.length).toBeGreaterThanOrEqual(10);
  for (const { input, want } of vectors) {
    expect(normalizeVaultText(input), JSON.stringify(input)).toBe(want);
  }
  for (let code = 0; code <= 0xffff; code += 1) {
    if (code >= 0xd800 && code <= 0xdfff) continue;
    const want = /\p{White_Space}/u.test(String.fromCharCode(code)) || code === 0x200b || code === 0xfeff;
    if (isVaultEdgeSpace(code) !== want) throw new Error(`U+${code.toString(16)}: ${!want}`);
  }
  // Linear on long interior whitespace (a regular expression with a trailing anchor backtracks here).
  const long = `a${' '.repeat(512 * 1024 - 3)}b `;
  const started = Date.now();
  expect(normalizeVaultText(long)).toBe(long.slice(0, -1));
  expect(Date.now() - started).toBeLessThan(500);
});

const certificate = '-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n';

async function legacyItem(page) {
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/environments(\?|$)/, route => route.fulfill({ body: inventoryPage(route, [
    { key: 'dev', display_name: 'Development', archived: false },
    { key: 'prod', display_name: 'Production', archived: false },
  ]) }));
  const metadata = {
    namespace_key: 'order_service', key: 'task-endpoints', display_name: 'Task endpoints', revision: 3, archived: false,
    snapshot: {
      fields: [
        { key: 'proxy_store_server_url', name: 'Proxy store URL', type: 'text' },
        { key: 'company', name: 'Company', type: 'text' },
        { key: 'certificate', name: 'Certificate', type: 'text' },
        { key: 'password', name: 'Password', type: 'secret' },
      ],
      variants: [{ id: 'a'.repeat(32), environments: ['dev'] }, { id: 'b'.repeat(32), environments: ['prod'] }],
    },
  };
  const valued = structuredClone(metadata);
  valued.snapshot.variants[0].values = {
    proxy_store_server_url: { text: ' http://dev.example.com:8918' }, company: { text: 'XOOM INTERNET LIMITED' },
    certificate: { text: certificate }, password: { text: ' edge-secret-value ' },
  };
  valued.snapshot.variants[1].values = {
    proxy_store_server_url: { text: 'http://prod.example.com:8918　\r\n' }, company: { text: 'XOOM INTERNET LIMITED' },
    certificate: { text: certificate }, password: { text: 'prod-secret-value' },
  };
  const commits = [];
  await page.route('**/v1/vault-items/order_service/task-endpoints**', route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname.endsWith('/revisions')) return route.fulfill({ json: { items: [{ revision: 3, actor_id: 'admin@example.com', created_at: '2026-11-11T01:00:00Z' }] } });
    if (pathname.endsWith('/usages') || pathname.endsWith('/impact-preview')) return route.fulfill({ json: { items: [], total: 0 } });
    if (pathname.endsWith('/values')) return route.fulfill({ json: valued });
    if (request.method() === 'PUT') {
      commits.push(request.postDataJSON());
      return route.fulfill({ json: { outcome: 'success', revision: 4 } });
    }
    return route.fulfill({ json: metadata });
  });
  return commits;
}

test('a legacy Text value with edge whitespace is flagged and cleaned without retyping, and multi-line Text and Secrets are left alone', async ({ page }) => {
  const commits = await legacyItem(page);
  await page.goto('/ui/#/vault/order_service/task-endpoints');

  await page.getByRole('button', { name: 'Reveal item values' }).click();
  const badges = page.getByText('Edge whitespace', { exact: true });
  await expect(badges).toHaveCount(1);
  await expect(badges.first()).toHaveAttribute('title', /Editing and saving the Item removes it/);

  await page.getByRole('button', { name: 'Edit item' }).click();
  const save = page.getByRole('button', { name: 'Save item' });
  const summary = page.locator('.vault-text-cleanup');
  // Every Variant is listed, not just the one on screen, by Field key and Environment and without values.
  await expect(summary).toContainText('Saving removes leading and trailing whitespace from these Text values:');
  await expect(summary.getByRole('listitem')).toHaveText(['proxy_store_server_urldev', 'proxy_store_server_urlprod']);
  await expect(summary).not.toContainText('example.com');
  await expect(page.getByText('Leading and trailing whitespace is removed when you save.')).toBeVisible();
  const secretWarning = page.getByText('This Secret starts or ends with whitespace.', { exact: false });
  await expect(secretWarning).toBeVisible();
  await expect(page.getByLabel('Variant 1 Password value')).toHaveAttribute('aria-describedby', /-edge$/);
  await expect(page.locator('.vault-snapshot-editor')).not.toContainText('edge-secret-value');

  // Tabbing through untouched multi-line Text changes nothing.
  await page.getByLabel('Variant 1 Certificate value').focus();
  await page.getByLabel('Variant 1 Company value').focus();
  await expect(save).toBeDisabled();

  // Leaving the edited field applies the rule to it.
  await page.getByLabel('Variant 1 Proxy store URL value').focus();
  await page.getByLabel('Variant 1 Company value').focus();
  await expect(page.getByLabel('Variant 1 Proxy store URL value')).toHaveValue('http://dev.example.com:8918');
  await expect(summary.getByRole('listitem')).toHaveText(['proxy_store_server_urlprod']);
  await expect(save).toBeEnabled();

  await summary.getByRole('button', { name: 'Remove now' }).click();
  await expect(summary).toHaveCount(0);
  await page.getByRole('button', { name: /^Variant 2/ }).click();
  await expect(page.getByLabel('Variant 2 Proxy store URL value')).toHaveValue('http://prod.example.com:8918');
  await expect(secretWarning).toHaveCount(0);

  await save.click();
  await expect.poll(() => commits.length).toBe(1);
  const [dev, prod] = commits[0].snapshot.variants;
  expect(dev.values.proxy_store_server_url.text).toBe('http://dev.example.com:8918');
  expect(prod.values.proxy_store_server_url.text).toBe('http://prod.example.com:8918');
  expect(dev.values.certificate.text).toBe(certificate);
  expect(dev.values.company.text).toBe('XOOM INTERNET LIMITED');
  expect(dev.values.password.text).toBe(' edge-secret-value ');
});

test('a value typed with edge whitespace is cleaned when the field loses focus', async ({ page }) => {
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/environments(\?|$)/, route => route.fulfill({ body: inventoryPage(route, [{ key: 'hk47', display_name: 'HK 47', archived: false }]) }));
  let created = null;
  await page.route('**/v1/vault-items**', route => {
    if (route.request().method() === 'PUT') {
      created = route.request().postDataJSON();
      return route.fulfill({ json: { outcome: 'success', revision: 1 } });
    }
    return route.fulfill({ body: inventoryPage(route, []) });
  });
  await page.goto('/ui/#/vault');
  await page.getByRole('button', { name: 'New Vault item' }).click();
  await page.locator('.vault-structured-form').getByLabel('Namespace').fill('order_service');
  await page.getByLabel('Resource key').fill('task-endpoints');
  await page.getByLabel('Display name').fill('Task endpoints');
  await page.getByLabel('Field key 1').fill('url');
  await page.getByLabel('Field name 1').fill('URL');
  await page.getByLabel('Field type 1').selectOption('text');
  const value = page.getByLabel('Variant 1 URL value');
  const notice = page.getByText('Leading and trailing whitespace is removed when you save.');
  // Nothing flickers while typing; leaving the field applies the rule.
  await value.fill('\t http://service.example.com:8918 \n');
  await expect(notice).toHaveCount(0);
  await expect(page.locator('.vault-text-cleanup')).toHaveCount(0);
  await page.getByLabel('Variant 1 HK 47 hk47').check();
  await expect(value).toHaveValue('http://service.example.com:8918');
  await expect(notice).toHaveCount(0);

  // A typed multi-line value keeps its final newline.
  await value.fill('line one\nline two\n');
  await page.getByLabel('Field name 1').focus();
  await expect(value).toHaveValue('line one\nline two\n');
  await expect(notice).toHaveCount(0);

  // Pressing Create straight from the field still submits on the first click, with the cleaned value.
  await value.fill('  http://service.example.com:8918  ');
  await page.getByRole('button', { name: 'Create Vault item' }).click();
  await expect.poll(() => created?.snapshot.variants[0].values.url.text).toBe('http://service.example.com:8918');
});

// An empty Text value is valid on the server, in the import and in Config resolution, and a value of only whitespace
// now becomes one, so the editor no longer blocks it. Secrets are not trimmed and still need a value.
test('an empty Text value can be created, from the import or after cleaning a value of only whitespace', async ({ page }) => {
  const draft = { namespace_key: 'order_service', item_key: 'task-endpoints', display_name: 'Task endpoints', environments: ['hk47'],
    fields: [{ key: 'prefix', name: 'Prefix', type: 'text' }, { key: 'suffix', name: 'Suffix', type: 'text' }, { key: 'password', name: 'Password', type: 'secret' }],
    values: { prefix: '', suffix: 'kept', password: '' } };
  const writes = [];
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/environments(\?|$)/, route => route.fulfill({ body: inventoryPage(route, [{ key: 'hk47', display_name: 'HK 47', archived: false }]) }));
  await page.route('**/v1/vault-items**', route => {
    const request = route.request();
    if (new URL(request.url()).pathname === '/v1/vault-items/import-preview') return route.fulfill({ json: { problems: [], draft } });
    if (request.method() === 'PUT') {
      writes.push(request.postDataJSON());
      return route.fulfill({ json: { outcome: 'success', revision: 1 } });
    }
    return route.fulfill({ body: inventoryPage(route, []) });
  });
  await page.goto('/ui/#/vault');
  await page.getByRole('button', { name: 'Import YAML' }).click();
  await page.getByLabel('Import file (YAML)').fill('namespace: order_service\n');
  await page.getByRole('button', { name: 'Check file' }).click();
  const create = page.getByRole('button', { name: 'Create Vault item' });
  await create.click();
  // The empty Secret is still required.
  expect(await page.getByLabel('Variant 1 Password value').evaluate(input => input.validity.valueMissing)).toBe(true);
  expect(writes).toHaveLength(0);
  await page.getByLabel('Variant 1 Password value').fill('s3cr3t');
  await page.getByLabel('Variant 1 Suffix value').fill(' \t　 ');
  await create.click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].snapshot.variants[0].values).toEqual({ prefix: { text: '' }, suffix: { text: '' }, password: { text: 's3cr3t' } });
});
