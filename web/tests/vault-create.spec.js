import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

test('a Vault Item create retried after a lost response keeps its operation and opens the new Item', async ({ page }) => {
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/environments(\?|$)/, route => route.fulfill({ body: inventoryPage(route, [{ key: 'hk47', display_name: 'HK 47', archived: false }]) }));
  const operations = [];
  let created = null;
  await page.route('**/v1/vault-items**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === 'PUT') {
      operations.push(request.headers()['idempotency-key']);
      // The first attempt is applied but its response is lost.
      created = request.postDataJSON();
      if (operations.length === 1) return route.abort('connectionreset');
      return route.fulfill({ json: { outcome: 'success', revision: 1 } });
    }
    if (pathname === '/v1/vault-items/order/ctit') {
      return route.fulfill({ json: { namespace_key: 'order', key: 'ctit', display_name: created.display_name, revision: 1, archived: false,
        snapshot: { fields: created.snapshot.fields, variants: created.snapshot.variants.map(({ id, environments }) => ({ id, environments })) } } });
    }
    if (pathname.startsWith('/v1/vault-items/order/ctit/')) return route.fulfill({ json: { items: [], total: 0 } });
    return route.fulfill({ body: inventoryPage(route, []) });
  });
  await page.goto('/ui/#/vault');
  await page.getByRole('button', { name: 'New Vault item' }).click();
  await page.locator('.vault-structured-form').getByLabel('Namespace').fill('order');
  await page.getByLabel('Resource key').fill('ctit');
  await page.getByLabel('Display name').fill('CTIT service');
  await page.getByLabel('Field key 1').fill('url');
  await page.getByLabel('Field name 1').fill('URL');
  await page.getByLabel('Variant 1 URL value').fill('https://ctit.example.com');
  await page.getByLabel('Variant 1 HK 47 hk47').check();

  await page.getByRole('button', { name: 'Create Vault item' }).click();
  // The failure keeps the form and everything typed into it.
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page).toHaveURL(/#\/vault$/);
  await expect(page.getByLabel('Resource key')).toHaveValue('ctit');

  await page.getByRole('button', { name: 'Create Vault item' }).click();
  await expect(page).toHaveURL(/#\/vault\/order\/ctit$/);
  await expect(page.getByRole('heading', { name: 'CTIT service' })).toBeVisible();
  expect(operations).toHaveLength(2);
  expect(operations[1]).toBe(operations[0]);
});
