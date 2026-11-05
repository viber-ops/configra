import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

const configs = Array.from({ length: 60 }, (_, index) => {
  const key = `service-${String(index + 1).padStart(4, '0')}`;
  return { key, display_name: `Service ${index + 1}`, archived: false, updated_at: '2026-10-01T00:00:00Z', environments: [{ key: 'production', revision: index + 1, archived: false }] };
});

async function configsPage(page, hash = '#/configs') {
  await page.route('**/v1/me', route => route.fulfill({ json: { issuer: 'https://id.example.com', subject: 'admin-1', email: 'admin@example.com', role: 'admin' } }));
  await page.route(/\/v1\/environments\?/, route => route.fulfill({ body: inventoryPage(route, [
    { key: 'production', display_name: 'Production' }, { key: 'staging', display_name: 'Staging' },
  ]) }));
  await page.route(/\/v1\/configs\?/, route => route.fulfill({ body: inventoryPage(route, configs) }));
  await page.route(/\/v1\/environments\/[a-z]+\/configs\/service-\d+$/, route => {
    if (route.request().method() !== 'GET') return route.fallback();
    const [, environment, key] = new URL(route.request().url()).pathname.match(/environments\/([a-z]+)\/configs\/(service-\d+)$/);
    return route.fulfill({ json: { environment_key: environment, config_key: key, config_name: key, format: 'yaml', content: `name: ${key}\nenvironment: ${environment}\n`, revision: 4 } });
  });
  await page.goto(`/ui/${hash}`);
}

async function expectSource(page, value) {
  await expect.poll(async () => (await page.getByLabel('Configuration source').locator('.cm-line').allTextContents()).join('\n')).toBe(value);
}

test('the template picker searches every Config and loads into an empty editor', async ({ page }) => {
  await configsPage(page);
  await page.getByRole('button', { name: 'New config' }).click();
  // service-0060 is not on the list's first page; the picker searches on its own.
  await page.getByRole('searchbox', { name: 'Template from existing config · Search' }).fill('service-0060');
  await page.getByRole('combobox', { name: 'Template from existing config', exact: true }).selectOption('service-0060');
  await expect(page.getByRole('combobox', { name: 'Template from existing config · Environment', exact: true })).toHaveValue('production');
  await expectSource(page, 'name: service-0060\nenvironment: production\n');
  await expect(page.getByText('Loaded production/service-0060@v4.')).toBeVisible();
  await expect(page.getByLabel('Display name')).toHaveValue('');
});

test('a template never overwrites typed content without an explicit choice', async ({ page }) => {
  await configsPage(page);
  await page.getByRole('button', { name: 'New config' }).click();
  await page.getByLabel('Configuration source').fill('mine: true\n');
  await page.getByRole('combobox', { name: 'Template from existing config', exact: true }).selectOption('service-0001');
  const replace = page.getByRole('button', { name: 'Replace the editor content with production/service-0001@v4' });
  await expect(replace).toBeVisible();
  await expectSource(page, 'mine: true\n');
  await replace.click();
  await expectSource(page, 'name: service-0001\nenvironment: production\n');

  // Untouched template content may be swapped for another template without asking.
  await page.getByRole('combobox', { name: 'Template from existing config', exact: true }).selectOption('service-0002');
  await expectSource(page, 'name: service-0002\nenvironment: production\n');
});

test('New Config from this template opens the form with the template and its environment', async ({ page }) => {
  await configsPage(page, '#/configs?new=1&template=service-0007&template_environment=staging');
  await expectSource(page, 'name: service-0007\nenvironment: staging\n');
  await expect(page.getByRole('combobox', { name: 'Environment', exact: true })).toHaveValue('staging');
  await expect.poll(() => page.evaluate(() => window.location.hash)).toBe('#/configs');
});

test('creating a Config retries a lost response with the same operation ID', async ({ page }) => {
  await configsPage(page);
  const attempts = [];
  await page.route('**/v1/environments/production/configs/payment', async route => {
    attempts.push(route.request().headers()['idempotency-key']);
    if (attempts.length === 1) return route.fulfill({ status: 503, json: { error: { code: 'service_unavailable', request_id: 'req-lost' } } });
    return route.fulfill({ json: { outcome: 'success', revision: 1 } });
  });
  await page.getByRole('button', { name: 'New config' }).click();
  await page.getByRole('combobox', { name: 'Environment', exact: true }).selectOption('production');
  await page.getByLabel('Resource key').fill('payment');
  await page.getByLabel('Display name').fill('Payment service');
  await page.getByLabel('Configuration source').fill('port: 6379\n');
  await page.getByRole('button', { name: 'Create config' }).click();
  await expect(page.getByRole('alert')).toContainText('req-lost');
  await page.getByRole('button', { name: 'Create config' }).click();
  await expect(page.locator('.config-create-form')).toHaveCount(0);
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toBe(attempts[0]);
});
