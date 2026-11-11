import { expect } from '@playwright/test';

// Opens the searchable Environment picker (typing `search` when given) and returns the option for `key`.
export async function environmentOption(page, label, key, search) {
  const input = page.getByRole('combobox', { name: label, exact: true });
  if (await input.getAttribute('aria-expanded') !== 'true') await input.click();
  if (search !== undefined) await input.fill(search);
  const listbox = page.locator(`[id="${await input.getAttribute('aria-controls')}"]`);
  return listbox.getByRole('option').filter({ has: page.locator('code').getByText(key, { exact: true }) });
}

// Chooses an Environment the way a person does: open the picker, search for the key and click its option.
export async function chooseEnvironment(page, label, key) {
  await (await environmentOption(page, label, key, key)).click();
  await expect(page.getByRole('combobox', { name: label, exact: true })).toHaveAttribute('aria-expanded', 'false');
}

// Checks Environments in a searchable multi-select (a fieldset named by its legend) by their keys.
export async function checkEnvironments(page, label, keys) {
  const group = page.getByRole('group', { name: label, exact: true });
  for (const key of keys) await group.getByRole('checkbox', { name: new RegExp(` ${key}$`) }).check();
}
