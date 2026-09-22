import { expect, test } from '@playwright/test';
import { seed } from '../scripts/local-ui.mjs';
import { inventoryPage } from './inventory.js';

test('local seed follows every inventory page and preserves archived Configs on rerun', async ({ page }) => {
  const inventories = new Map([
    ['/v1/environments', []], ['/v1/vault-items', []], ['/v1/configs', []],
    ['/v1/notification-destinations', []], ['/v1/api-tokens', []],
    ['/v1/client-certificates', [
      { display_name: 'Edge reader 01 · Singapore', fingerprint_sha256: 'edge' },
      { display_name: 'Retired reader 02 · revoked example', fingerprint_sha256: 'retired', revoked: true },
    ]],
  ]);
  const revisions = new Map();
  const reads = [];
  const mutations = [];
  await page.route('**/v1/**', route => {
    const request = route.request();
    const url = new URL(request.url());
    const pathname = url.pathname;
    const reply = (body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    const inventory = inventories.get(pathname);
    if (inventory && request.method() === 'GET') {
      reads.push(url);
      return route.fulfill({ contentType: 'application/json', body: inventoryPage(route, inventory) });
    }
    if (pathname === '/v1/me') return reply({ subject: 'admin-1', role: 'admin' });
    if (pathname === '/v1/access' || pathname === '/v1/audit') return reply({ items: [], total: 0 });
    if (pathname.endsWith('/values') || pathname.endsWith('/resolved-preview')) return reply({});
    if (/\/(archive|revoke)$/.test(pathname)) {
      const parts = pathname.split('/');
      const item = inventories.get(`/v1/${parts[2]}`).find(item => (item.key || item.public_id) === parts.at(-2));
      item[parts.at(-1) === 'archive' ? 'archived' : 'revoked'] = true;
      return reply({ outcome: 'success' });
    }
    const config = pathname.match(/^\/v1\/environments\/([^/]+)\/configs\/([^/]+)$/);
    if (config) {
      const item = inventories.get('/v1/configs').find(item => item.key === config[2]);
      if (request.method() === 'GET') return revisions.has(pathname) && !item.archived ? reply({ revision: revisions.get(pathname) }) : reply({}, 404);
      if (item?.archived) return reply({ error: { code: 'resource_archived' } }, 409);
      const body = request.postDataJSON();
      expect(body.expected_revision).toBe(revisions.get(pathname) || 0);
      mutations.push(pathname);
      revisions.set(pathname, body.expected_revision + 1);
      if (!item) inventories.get('/v1/configs').push({ key: config[2], display_name: body.name });
      return reply({ revision: revisions.get(pathname) });
    }
    const body = request.postDataJSON();
    mutations.push(pathname);
    if (pathname.startsWith('/v1/vault-items/')) {
      const [, , , namespace_key, key] = pathname.split('/');
      const items = inventories.get('/v1/vault-items');
      let item = items.find(item => item.key === key && item.namespace_key === namespace_key);
      if (!item) items.push(item = { namespace_key, key });
      Object.assign(item, body, { revision: body.expected_revision + 1 });
      return reply(item);
    }
    if (pathname.startsWith('/v1/notification-destinations/')) {
      inventories.get('/v1/notification-destinations').push({ ...body, key: pathname.split('/').at(-1) });
    } else {
      inventory.push({ ...body, public_id: `token-${inventory.length}` });
    }
    return reply({ outcome: 'success' });
  });
  await page.goto('/ui/');
  const first = await seed(page);
  const configs = inventories.get('/v1/configs');
  expect(configs).toHaveLength(79);
  configs.sort((left, right) => left.key.localeCompare(right.key));
  const worker = configs.find(item => item.key === 'worker');
  expect(configs.indexOf(worker)).toBeGreaterThanOrEqual(50);
  worker.archived = true;
  mutations.length = 0;
  reads.length = 0;
  expect(await seed(page)).toEqual(first);
  expect(first.configs).toBe(79);
  expect(mutations).toEqual([]);
  expect(worker.archived).toBe(true);
  expect(reads.some(url => url.pathname === '/v1/configs' && url.searchParams.get('offset') === '50')).toBe(true);
});
