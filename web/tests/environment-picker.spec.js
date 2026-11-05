import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';
import { environmentOption } from './environment.js';

const environments = Array.from({ length: 60 }, (_, index) => {
  const number = String(index + 1).padStart(2, '0');
  return { key: `env-${number}`, display_name: `Region ${number}`, archived: false, updated_at: '2026-11-01T00:00:00Z' };
});

async function configsPage(page, searches = []) {
  await page.route('**/v1/me', route => route.fulfill({ json: { subject: 'admin-1', role: 'admin' } }));
  await page.route(/\/v1\/configs(\?|$)/, route => route.fulfill({ body: inventoryPage(route, []) }));
  await page.route(/\/v1\/environments\?/, route => {
    searches.push(new URL(route.request().url()).searchParams.get('q'));
    return route.fulfill({ body: inventoryPage(route, environments) });
  });
  await page.goto('/ui/#/configs');
  await expect(page.getByRole('combobox', { name: 'Environment filter', exact: true })).toBeVisible();
}

test('the Environment picker works from the keyboard and Tab leaves it without opening a list', async ({ page }) => {
  const searches = [];
  await configsPage(page, searches);
  const picker = page.getByRole('combobox', { name: 'Environment filter', exact: true });
  await page.getByLabel('Search Configs').focus();
  await page.keyboard.press('Tab');
  await expect(picker).toBeFocused();
  await expect(picker).toHaveAttribute('aria-expanded', 'false');

  await page.keyboard.press('ArrowDown');
  await expect(picker).toHaveAttribute('aria-expanded', 'true');
  const listbox = page.getByRole('listbox', { name: 'Environment filter' });
  // Arrow keys move within the options already loaded, so wait for the first page.
  await expect(listbox.getByRole('option', { name: 'Region 02 · env-02' })).toBeVisible();
  const activeName = async () => listbox.locator(`[id="${await picker.getAttribute('aria-activedescendant')}"]`).textContent();
  expect(await activeName()).toBe('All environments');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  expect(await activeName()).toBe('Region 02 · env-02');
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/environment=env-02/);
  await expect(picker).toHaveValue('Region 02 · env-02');
  await expect(picker).toHaveAttribute('aria-expanded', 'false');

  // Typing searches on the server; Escape abandons the search and restores the choice.
  await picker.fill('Region 4');
  await expect.poll(() => searches.at(-1)).toBe('Region 4');
  await expect(listbox.getByRole('option')).toHaveCount(10);
  await page.keyboard.press('Escape');
  await expect(picker).toHaveValue('Region 02 · env-02');
  await expect(picker).toHaveAttribute('aria-expanded', 'false');
  await expect(page).toHaveURL(/environment=env-02/);

  // Tab never stops inside the list.
  await picker.click();
  await page.keyboard.press('Tab');
  await expect(picker).not.toBeFocused();
  await expect(page.locator('.environment-combobox')).not.toContainText('Region 01');
  expect(await page.evaluate(() => document.activeElement.closest('.environment-combobox'))).toBeNull();
});

test('the Environment picker pages past the first fifty Environments', async ({ page }) => {
  await configsPage(page);
  const picker = page.getByRole('combobox', { name: 'Environment filter', exact: true });
  await picker.click();
  const menu = page.locator('.environment-combobox-menu');
  await expect(menu.getByText('1–50 / 60')).toBeVisible();
  await menu.getByRole('button', { name: 'Next' }).click();
  await expect(menu.getByText('51–60 / 60')).toBeVisible();
  await expect(picker).toBeFocused();
  await (await environmentOption(page, 'Environment filter', 'env-55')).click();
  await expect(page).toHaveURL(/environment=env-55/);
  await expect(picker).toHaveValue('Region 55 · env-55');

  // PageUp and PageDown page through the list without leaving the input.
  await picker.press('ArrowDown');
  await expect(menu.getByText('51–60 / 60')).toBeVisible();
  await picker.press('PageUp');
  await expect(menu.getByText('1–50 / 60')).toBeVisible();
  await picker.press('PageDown');
  await expect(menu.getByText('51–60 / 60')).toBeVisible();
});

test('a typed search without a choice leaves a required Environment empty', async ({ page }) => {
  await configsPage(page);
  await page.getByRole('button', { name: 'New config' }).click();
  const picker = page.getByRole('combobox', { name: 'Environment', exact: true });
  await picker.fill('Region 1');
  await page.getByLabel('Resource key').click();
  await expect(picker).toHaveValue('');
  expect(await picker.evaluate(element => element.validity.valueMissing)).toBe(true);
  expect(await page.locator('input[type=hidden][name=environment]').inputValue()).toBe('');
});
