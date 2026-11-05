import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

const longName = 'Payment service · checkout, ledger reconciliation and settlement';

async function detailPage(page) {
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/(configs|vault-items)(\?|$)/, route => route.fulfill({ body: inventoryPage(route, [
    { key: 'payment', display_name: longName, archived: false, updated_at: '2026-11-01T00:00:00Z', environments: [{ key: 'production', revision: 8, archived: false }] },
  ]) }));
  await page.route(/\/v1\/environments\?/, route => route.fulfill({ body: inventoryPage(route, [
    { key: 'production', display_name: 'Production · Singapore', revision: 8 }, { key: 'staging', display_name: 'Staging / Integration' },
  ]) }));
  await page.route('**/v1/environments/production/configs/payment**', route => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/revisions')) {
      return route.fulfill({ json: { items: [8, 7, 6, 5, 4, 3, 2, 1].map(revision => ({ revision, format: 'yaml', created_at: '2026-11-01T00:00:00Z' })) } });
    }
    return route.fulfill({ json: { environment_key: 'production', config_key: 'payment', config_name: longName, format: 'yaml', content: 'port: 8007\n', revision: 8 } });
  });
  await page.goto('/ui/#/configs/payment/production');
  await expect(page.locator('.revision-track button')).toHaveCount(8);
}

async function currentRevisionInView(page) {
  return page.evaluate(() => {
    const track = document.querySelector('.revision-track');
    const current = track.querySelector('.current-revision').getBoundingClientRect();
    const frame = track.getBoundingClientRect();
    return track.scrollWidth <= track.clientWidth + 1 && current.left >= frame.left - 1 && current.right <= frame.right + 1;
  });
}

test('the revision track keeps the current version in view instead of scrolling it away', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await detailPage(page);
  await expect(page.locator('.revision-track .current-revision')).toContainText('v8');
  expect(await currentRevisionInView(page)).toBe(true);
});

test('on a phone the long title wraps, the page does not scroll sideways and the current version stays visible', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await detailPage(page);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  expect(await currentRevisionInView(page)).toBe(true);
  const title = await page.locator('.config-detail-heading h1').boundingBox();
  expect(title.width).toBeLessThanOrEqual(390);
});

test('the header Environment picker is one searchable control that names the current Environment', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await detailPage(page);
  const picker = page.getByRole('combobox', { name: 'Environment', exact: true });
  await expect(picker).toHaveValue('Production · Singapore · production');
  await expect(page.locator('.detail-heading-actions input[type=search]')).toHaveCount(0);
  await picker.click();
  await expect(page.getByRole('listbox', { name: 'Environment' }).getByRole('option', { name: /Staging \/ Integration/ })).toBeVisible();
  expect(await currentRevisionInView(page)).toBe(true);
});
