import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

// order_service and order_service_clients both exist in hk47; billing exists only in hk180.
const membership = { order_service: ['hk47', 'hk180'], order_service_clients: ['hk47'], billing: ['hk180'] };
const configs = Object.entries(membership).map(([key, environments]) => ({
  key, display_name: key.replaceAll('_', ' '), archived: false, updated_at: '2026-11-01T00:00:00Z',
  environments: environments.map(environment => ({ key: environment, revision: 2, archived: false })),
}));
const environments = [{ key: 'hk47', display_name: 'HK 47' }, { key: 'hk180', display_name: 'HK 180' }];

async function mock(page) {
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/configs(\?|$)/, route => route.fulfill({ body: inventoryPage(route, configs) }));
  await page.route(/\/v1\/environments(\?|$)/, route => {
    const config = new URL(route.request().url()).searchParams.get('config');
    const rows = environments
      .filter(environment => !config || membership[config]?.includes(environment.key))
      .map(environment => ({ ...environment, archived: false, revision: config ? 2 : 0 }));
    return route.fulfill({ body: inventoryPage(route, rows) });
  });
  await page.route(/\/v1\/environments\/[^/]+\/configs\/[^/?]+/, route => {
    const [, environment, config] = new URL(route.request().url()).pathname.match(/environments\/([^/]+)\/configs\/([^/]+)/);
    if (route.request().url().includes('/revisions')) return route.fulfill({ json: { items: [2, 1].map(revision => ({ revision, format: 'yaml', created_at: '2026-11-01T00:00:00Z' })) } });
    return route.fulfill({ json: { environment_key: environment, config_key: config, config_name: config.replaceAll('_', ' '), format: 'yaml', content: `name: ${config}\n`, revision: 2 } });
  });
}

const railLink = (page, key) => page.locator('.resource-rail-items a').filter({ has: page.locator('code').getByText(key, { exact: true }) });

test('switching Configs keeps the current Environment when the target Config has it', async ({ page }) => {
  await mock(page);
  await page.goto('/ui/#/configs/order_service/hk47');
  await expect(page.locator('.config-detail-heading h1')).toHaveText('order service');
  await railLink(page, 'order_service_clients').click();
  await expect(page).toHaveURL(/#\/configs\/order_service_clients\/hk47$/);
  await expect(page.locator('.config-detail-heading h1')).toHaveText('order service clients');
  await expect(railLink(page, 'order_service_clients')).toHaveAttribute('aria-current', 'true');
  // The forward replaces the choice page in history, so Back returns to the previous Config.
  await page.goBack();
  await expect(page).toHaveURL(/#\/configs\/order_service\/hk47$/);
  await expect(page.locator('.config-detail-heading h1')).toHaveText('order service');
});

test('a Config without the current Environment opens its Environment choice instead of another Environment', async ({ page }) => {
  await mock(page);
  await page.goto('/ui/#/configs/order_service/hk47');
  await railLink(page, 'billing').click();
  await expect(page.getByRole('heading', { name: 'billing' })).toBeVisible();
  await expect(page.locator('.config-home')).toBeVisible();
  await expect(page).toHaveURL(/#\/configs\/billing\?environment=hk47$/);
  await expect(page.locator('.config-detail-heading')).toHaveCount(0);
});
