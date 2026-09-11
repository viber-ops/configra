import { expect, test } from '@playwright/test';

async function mockIdentity(page, role = 'admin') {
  await page.route('**/v1/me', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ issuer: 'https://id.example.com', subject: `${role}-1`, email: `${role}@example.com`, role }),
  }));
}

async function expectEditorText(editor, value) {
  await expect.poll(async () => (await editor.locator('.cm-line').allTextContents()).join('\n')).toBe(value);
}

test('administrator creates a peer Environment with an immutable resource key', async ({ page }) => {
  await mockIdentity(page);
  const items = [
    { key: 'production', display_name: 'Production', archived: false, updated_at: '2026-08-27T01:00:00Z' },
    { key: 'recovery', display_name: 'Recovery room', archived: true, updated_at: '2026-08-26T01:00:00Z' },
  ];
  let mutation;
  await page.route('**/v1/environments*', async route => {
    if (route.request().method() === 'POST') {
      mutation = {
        body: route.request().postDataJSON(),
        operation: route.request().headers()['idempotency-key'],
      };
      items.push({ key: mutation.body.key, display_name: mutation.body.display_name, archived: false, updated_at: '2026-08-27T02:00:00Z' });
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', ...mutation.body, archived: false }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items }) });
  });
  await page.goto('/ui/#/environments');

  await expect(page.getByRole('heading', { name: 'Environments' })).toBeVisible();
  await expect(page.getByText('production', { exact: true })).toBeVisible();
  await expect(page.getByText('Recovery room')).toBeVisible();
  await expect(page.getByText('Archived', { exact: true })).toBeVisible();

  await page.getByRole('button', { name: 'New environment' }).click();
  await page.getByLabel('Resource key').fill('staging');
  await page.getByLabel('Display name').fill('Staging');
  await page.getByRole('button', { name: 'Create environment' }).click();

  await expect(page.getByText('staging', { exact: true })).toBeVisible();
  expect(mutation.body).toEqual({ key: 'staging', display_name: 'Staging' });
  expect(mutation.operation).toMatch(/^[0-9a-f-]{36}$/);
});

test('administrator renames, archives, and restores a peer Environment', async ({ page }) => {
  await mockIdentity(page);
  const environment = { key: 'production', display_name: 'Production', archived: false, updated_at: '2026-08-27T01:00:00Z' };
  const mutations = [];
  await page.route('**/v1/environments**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === 'PATCH') {
      mutations.push({ method: 'PATCH', body: request.postDataJSON(), operation: request.headers()['idempotency-key'] });
      environment.display_name = request.postDataJSON().display_name;
    } else if (request.method() === 'POST') {
      const action = pathname.endsWith('/unarchive') ? 'unarchive' : 'archive';
      mutations.push({ method: action, operation: request.headers()['idempotency-key'] });
      environment.archived = action === 'archive';
    } else {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [environment] }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', ...environment }) });
  });
  await page.goto('/ui/#/environments');

  await page.getByRole('button', { name: 'Rename Production' }).click();
  await page.getByLabel('Display name').fill('Primary runtime');
  await page.getByRole('button', { name: 'Save name' }).click();
  await expect(page.getByRole('heading', { name: 'Primary runtime' })).toBeVisible();
  await page.getByRole('button', { name: 'Archive Primary runtime' }).click();
  await page.getByRole('button', { name: 'Confirm archive Primary runtime' }).click();
  await expect(page.getByText('Archived', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Unarchive Primary runtime' }).click();

  expect(mutations.map(item => item.method)).toEqual(['PATCH', 'archive', 'unarchive']);
  expect(mutations[0].body).toEqual({ display_name: 'Primary runtime' });
  for (const mutation of mutations) expect(mutation.operation).toMatch(/^[0-9a-f-]{36}$/);
});

test('viewer opens an Environment and carries its exact filter into Config and Vault inventories', async ({ page }) => {
  await mockIdentity(page, 'viewer');
  const environments = [
    { key: 'production', display_name: 'Production', archived: false, updated_at: '2026-08-27T01:00:00Z' },
    { key: 'staging', display_name: 'Staging', archived: false, updated_at: '2026-08-26T01:00:00Z' },
  ];
  const configs = [
    { key: 'payment', display_name: 'Payment service', archived: false, updated_at: '2026-08-27T01:00:00Z', environments: [{ key: 'production', revision: 7, archived: false }] },
    { key: 'worker', display_name: 'Worker', archived: false, updated_at: '2026-08-26T01:00:00Z', environments: [{ key: 'staging', revision: 2, archived: false }] },
  ];
  const vault = [
    { namespace_key: 'platform', key: 'mysql', display_name: 'MySQL credentials', revision: 5, environment_keys: ['canary', 'development', 'eu-west', 'production', 'qa', 'recovery', 'staging'], archived: false, updated_at: '2026-08-27T01:00:00Z' },
    { namespace_key: 'platform', key: 'redis', display_name: 'Redis credentials', revision: 3, environment_keys: ['staging'], archived: false, updated_at: '2026-08-26T01:00:00Z' },
  ];
  await page.route('**/v1/environments*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: environments }) }));
  await page.route('**/v1/configs*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: configs }) }));
  await page.route('**/v1/vault-items*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: vault }) }));

  await page.goto('/ui/#/environments');
  const environmentLink = page.getByRole('link', { name: 'Open Environment Production' });
  await expect(environmentLink).toHaveAttribute('href', '#/environments/production');
  await environmentLink.click();

  await expect(page).toHaveURL(/#\/environments\/production$/);
  await expect(page.getByRole('heading', { name: 'Production' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Payment service v7' })).toHaveAttribute('href', '#/configs/payment/production');
  await expect(page.getByRole('link', { name: 'platform.mysql v5' })).toHaveAttribute('href', '#/vault/platform/mysql');

  await page.getByRole('link', { name: 'View all Configs in Production' }).click();
  await expect(page).toHaveURL(/#\/configs\?environment=production$/);
  await expect(page.getByLabel('Environment filter')).toHaveValue('production');
  await expect(page.getByRole('link', { name: 'Open Config Payment service' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open Config Worker' })).toHaveCount(0);

  await page.goto('/ui/#/vault?environment=production');
  await expect(page.getByLabel('Environment filter')).toHaveValue('production');
  await expect(page.getByRole('link', { name: 'platform.mysql · MySQL credentials · v5' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'recovery', exact: true })).toBeVisible();
  await expect(page.getByText('+4', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'platform.redis · Redis credentials · v3' })).toHaveCount(0);
});

test('viewer reads Config identities and their independent Environment revisions without mutation controls', async ({ page }) => {
  await mockIdentity(page, 'viewer');
  await page.route('**/v1/configs*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [{
      key: 'payment', display_name: 'Payment service', archived: false,
      updated_at: '2026-08-27T01:00:00Z',
      environments: [{ key: 'production', revision: 17, archived: false }, { key: 'recovery', revision: 4, archived: true }],
    }] }),
  }));
  await page.goto('/ui/#/configs');

  await expect(page.getByRole('heading', { name: 'Configs' })).toBeVisible();
  await expect(page.getByText('payment', { exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open Config Payment service' })).toHaveAttribute('href', '#/configs/payment');
  await expect(page.getByRole('link', { name: 'production v17' })).toHaveAttribute('href', '#/configs/payment/production');
  await expect(page.getByRole('link', { name: 'recovery v4 archived' })).toHaveCount(0);
  await expect(page.getByLabel('recovery v4 Archived')).toBeVisible();
  await expect(page.getByRole('button', { name: /new config/i })).toHaveCount(0);
});

test('Config index searches and paginates a thousand-scale resource list', async ({ page }) => {
  if (process.env.CONFIGRA_UI_MOBILE === '1') await page.setViewportSize({ width: 390, height: 844 });
  await mockIdentity(page, 'viewer');
  const items = Array.from({ length: 120 }, (_, index) => {
    const suffix = String(index + 1).padStart(4, '0');
    return {
      key: `service_${suffix}`, display_name: `Service ${suffix}`, archived: (index + 1) % 10 === 0,
      updated_at: '2026-08-27T01:00:00Z', environments: [{ key: 'production', revision: index + 1, archived: false }],
    };
  });
  await page.route('**/v1/configs*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items }) }));
  await page.goto('/ui/#/configs');

  await expect(page.locator('.config-index-table tbody tr')).toHaveCount(50);
  await expect(page.getByRole('link', { name: 'Open Config Service 0001' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open Config Service 0051' })).toHaveCount(0);
  if (process.env.CONFIGRA_CONFIG_SCALE_SCREENSHOT) await page.screenshot({ path: process.env.CONFIGRA_CONFIG_SCALE_SCREENSHOT, fullPage: true });
  await page.getByRole('button', { name: 'Next' }).click();
  await expect(page.getByRole('link', { name: 'Open Config Service 0051' })).toBeVisible();
  await page.getByRole('searchbox', { name: 'Search configs' }).fill('service_0100');
  await expect(page.getByRole('link', { name: 'Open Config Service 0100' })).toBeVisible();
  await page.getByLabel('Status').selectOption('archived');
  await expect(page.locator('.config-index-table tbody tr')).toHaveCount(1);
});

test('viewer opens a Config identity before choosing its Environment context', async ({ page }) => {
  await mockIdentity(page, 'viewer');
  await page.route('**/v1/configs*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [{
      key: 'payment', display_name: 'Payment service', archived: false,
      updated_at: '2026-08-27T01:00:00Z',
      environments: [{ key: 'production', revision: 17, archived: false }, { key: 'recovery', revision: 4, archived: true }],
    }] }),
  }));
  await page.route('**/v1/environments*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [
      { key: 'production', display_name: 'Production', archived: false },
      { key: 'recovery', display_name: 'Recovery', archived: true },
    ] }),
  }));
  await page.goto('/ui/#/configs');

  if (process.env.CONFIGRA_CONFIG_INDEX_SCREENSHOT) await page.screenshot({ path: process.env.CONFIGRA_CONFIG_INDEX_SCREENSHOT, fullPage: true });
  await page.getByRole('link', { name: 'Open Config Payment service' }).click();
  await expect(page).toHaveURL(/#\/configs\/payment$/);
  await expect(page.getByRole('heading', { name: 'Payment service' })).toBeVisible();
  await expect(page.getByText('Config identity', { exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: /Open context: Production.*production.*v17/ })).toHaveAttribute('href', '#/configs/payment/production');
  await expect(page.getByRole('link', { name: /Recovery.*recovery.*v4/ })).toHaveCount(0);
  await expect(page.getByLabel(/Recovery.*recovery.*v4.*Archived/)).toBeVisible();
  if (process.env.CONFIGRA_CONFIG_HOME_SCREENSHOT) await page.screenshot({ path: process.env.CONFIGRA_CONFIG_HOME_SCREENSHOT, fullPage: true });
});

test('administrator creates, archives, and restores a Config identity', async ({ page }) => {
  await mockIdentity(page);
  const items = [];
  const lifecycle = [];
  let commit;
  await page.route('**/v1/environments', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
    { key: 'production', display_name: 'Production', archived: false },
    { key: 'staging', display_name: 'Staging', archived: false },
  ] }) }));
  await page.route('**/v1/configs**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === 'POST') {
      const action = pathname.endsWith('/unarchive') ? 'unarchive' : 'archive';
      lifecycle.push({ action, operation: request.headers()['idempotency-key'] });
      items[0].archived = action === 'archive';
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', key: 'payment', archived: items[0].archived }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items }) });
  });
  await page.route('**/v1/environments/production/configs/payment', async route => {
    commit = { body: route.request().postDataJSON(), operation: route.request().headers()['idempotency-key'] };
    items.push({ key: 'payment', display_name: commit.body.name, archived: false, updated_at: '2026-08-27T01:00:00Z', environments: [{ key: 'production', revision: 1, archived: false }] });
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
      outcome: 'success', revision: 1,
      warnings: [{ code: 'missing_vault_field', namespace_key: 'platform', item_key: 'redis', field_key: 'username' }],
    }) });
  });
  await page.goto('/ui/#/configs');

  await page.getByRole('button', { name: 'New config' }).click();
  if (process.env.CONFIGRA_CONFIG_CREATE_SCREENSHOT) await page.screenshot({ path: process.env.CONFIGRA_CONFIG_CREATE_SCREENSHOT, fullPage: true });
  await page.getByLabel('Environment').selectOption('production');
  await page.getByLabel('Resource key').fill('payment');
  await page.getByLabel('Display name').fill('Payment service');
  await page.getByLabel('Format').selectOption('yaml');
  await page.getByLabel('Configuration source').fill('port: 6379\n');
  await page.getByRole('button', { name: 'Create config' }).click();

  expect(commit.body).toEqual({ name: 'Payment service', expected_revision: 0, format: 'yaml', content: 'port: 6379\n' });
  expect(commit.operation).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByText(/Vault field does not exist.*vault\.platform\.redis\.username/)).toBeVisible();
  await page.getByRole('button', { name: 'Archive Payment service' }).click();
  await page.getByRole('button', { name: 'Confirm archive Payment service' }).click();
  await expect(page.getByRole('link', { name: 'production v1' })).toHaveCount(0);
  await expect(page.getByLabel('production v1')).toBeVisible();
  await page.getByRole('button', { name: 'Unarchive Payment service' }).click();
  expect(lifecycle.map(item => item.action)).toEqual(['archive', 'unarchive']);
  for (const mutation of lifecycle) expect(mutation.operation).toMatch(/^[0-9a-f-]{36}$/);
});

test('viewer can inspect Raw Config history but cannot reveal or mutate values', async ({ page }) => {
  await mockIdentity(page, 'viewer');
  await page.route('**/v1/environments/production/configs/payment**', async route => {
    if (new URL(route.request().url()).pathname.endsWith('/revisions')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ revision: 2, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T01:00:00Z' }] }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ environment_key: 'production', config_key: 'payment', config_name: 'Payment service', format: 'yaml', content: "password: '{vault.platform.mysql.password}'\n", revision: 2 }) });
  });
  await page.goto('/ui/#/configs/payment/production');

  await expect(page.getByLabel('Configuration source')).toHaveAttribute('aria-readonly', 'true');
  await expect(page.getByRole('tab', { name: 'History' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Resolved preview' })).toHaveCount(0);
  await expect(page.getByRole('tab', { name: 'Transfer' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Save changes' })).toHaveCount(0);
});

test('administrator edits canonical Config source against the visible current Revision and reads history', async ({ page }) => {
  await mockIdentity(page);
  let current = {
    environment_key: 'production', config_key: 'payment', config_name: 'Payment service',
    format: 'yaml', content: 'port: 6379\n', revision: 17,
  };
  const revisions = [{
    revision: 17, format: 'yaml', operation_id: 'save-17', actor_type: 'user',
    actor_id: 'https://id.example.com|admin-1', created_at: '2026-08-27T01:00:00Z',
  }, {
    revision: 16, format: 'yaml', operation_id: 'save-16', actor_type: 'user',
    actor_id: 'bob@example.com', created_at: '2026-08-26T01:00:00Z',
  }];
  let mutation;
  let validation;
  await page.route('**/v1/configs/validate', async route => {
    validation = route.request().postDataJSON();
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
      format: 'yaml', content: 'port: 6380\n', warnings: [],
    }) });
  });
  await page.route('**/v1/environments/production/configs/payment**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/revisions')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: revisions }) });
      return;
    }
    if (route.request().method() === 'PUT') {
      mutation = { body: route.request().postDataJSON(), operation: route.request().headers()['idempotency-key'] };
      current = { ...current, content: mutation.body.content, revision: 18 };
      revisions.unshift({ revision: 18, format: 'yaml', operation_id: mutation.operation, actor_type: 'user', actor_id: 'admin@example.com', created_at: '2026-08-27T02:00:00Z' });
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
        outcome: 'success', revision: 18,
        warnings: [{ code: 'missing_vault_item', namespace_key: 'platform', item_key: 'redis', field_key: 'password' }],
      }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(current) });
  });
  await page.goto('/ui/#/configs/payment/production');

  await expect(page.getByRole('heading', { name: 'Payment service' })).toBeVisible();
  await expect(page.getByLabel('Environment')).toHaveValue('production');
  await expect(page.getByRole('button', { name: 'v17 Current' })).toBeVisible();
  await expect(page.locator('.config-meta-panel .actor-identity')).toContainText('admin@example.com');
  await expect(page.locator('.config-meta-panel .actor-identity')).toHaveAttribute('title', 'https://id.example.com|admin-1');
  const source = page.getByLabel('Configuration source');
  await expectEditorText(source, 'port: 6379\n');
  await source.fill('port:   6380\n');
  await page.getByRole('button', { name: 'Format source' }).click();
  await expectEditorText(source, 'port: 6380\n');
  await expect(page.getByText('Source formatted.')).toBeVisible();
  await page.getByRole('button', { name: 'Save changes' }).click();

  await expect(page.getByRole('button', { name: 'v18 Current' })).toBeVisible();
  await expect(page.getByText(/Vault item does not exist.*vault\.platform\.redis\.password/)).toBeVisible();
  expect(mutation.body).toEqual({ name: 'Payment service', expected_revision: 17, format: 'yaml', content: 'port: 6380\n' });
  expect(validation).toEqual({ environment: 'production', format: 'yaml', content: 'port:   6380\n' });
  expect(mutation.operation).toMatch(/^[0-9a-f-]{36}$/);

  await page.getByRole('tab', { name: 'History' }).click();
  await expect(page.getByRole('row', { name: /v16.*bob@example.com/ })).toBeVisible();
});

test('administrator opens an old Config Revision beyond the rail and restores it as a new current Revision', async ({ page }) => {
  await mockIdentity(page);
  let current = { environment_key: 'production', config_key: 'payment', config_name: 'Payment service', format: 'yaml', content: 'revision: 12\n', revision: 12 };
  const revisions = Array.from({ length: 12 }, (_, index) => ({ revision: 12 - index, format: 'yaml', actor_id: 'admin@example.com', created_at: `2026-08-${String(27 - index).padStart(2, '0')}T01:00:00Z` }));
  let restore;
  await page.route('**/v1/environments/production/configs/payment**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname.endsWith('/revisions/1')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ...current, content: 'revision: 1\n', revision: 1 }) });
    if (pathname.endsWith('/revisions')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: revisions }) });
    if (pathname.endsWith('/restore')) {
      restore = request.postDataJSON();
      current = { ...current, content: 'revision: 1\n', revision: 13 };
      revisions.unshift({ revision: 13, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T02:00:00Z' });
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":13}' });
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(current) });
  });
  await page.goto('/ui/#/configs/payment/production');

  await expect(page.getByLabel('Revision history').getByText('v1', { exact: true })).toHaveCount(0);
  await page.getByRole('tab', { name: 'History' }).click();
  await page.getByRole('button', { name: 'View v1', exact: true }).click();
  await expectEditorText(page.getByLabel('Revision v1'), 'revision: 1\n');
  await expect(page.getByLabel('Revision v1')).toHaveAttribute('aria-readonly', 'true');
  await expect(page.getByRole('button', { name: 'Save changes' })).toHaveCount(0);

  await page.getByRole('tab', { name: 'History' }).click();
  await page.getByRole('button', { name: 'Restore v1', exact: true }).click();
  await page.getByRole('button', { name: 'Confirm restore v1', exact: true }).click();
  expect(restore).toEqual({ source_revision: 1, expected_revision: 12 });
  await expect(page.getByRole('button', { name: 'v13 Current' })).toBeVisible();
});

test('Config editor protects dirty source and keeps it after an actionable Revision conflict', async ({ page }) => {
  await mockIdentity(page);
  let current = { environment_key: 'production', config_key: 'payment', config_name: 'Payment service', format: 'yaml', content: 'port: 6379\n', revision: 2 };
  const revisions = [{ revision: 2, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T01:00:00Z' }];
  let saves = 0;
  await page.route('**/v1/environments/production/configs/payment**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname.endsWith('/revisions')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: revisions }) });
    if (request.method() === 'PUT') {
      saves += 1;
      if (saves === 1) return route.fulfill({ status: 409, contentType: 'application/json', body: '{"error":{"code":"revision_conflict","request_id":"request-abc123"}}' });
      current = { ...current, content: request.postDataJSON().content, revision: 3 };
      revisions.unshift({ revision: 3, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T02:00:00Z' });
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":3}' });
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(current) });
  });
  await page.goto('/ui/#/overview');
  await page.evaluate(() => { window.location.hash = '#/configs/payment/production'; });

  const save = page.getByRole('button', { name: 'Save changes' });
  await expect(save).toBeDisabled();
  const editor = page.getByLabel('Configuration source');
  await editor.fill('port: 6380\n');
  await expect(save).toBeEnabled();

  const back = page.goBack();
  const backDialog = await page.waitForEvent('dialog');
  expect(backDialog.message()).toBe('You have unsaved changes. Discard them?');
  await backDialog.dismiss();
  await back;
  await expect(page).toHaveURL(/#\/configs\/payment\/production$/);
  await expectEditorText(editor, 'port: 6380\n');

  const click = page.locator('.config-detail .eyebrow').getByRole('link', { name: 'Configs' }).click();
  const dialog = await page.waitForEvent('dialog');
  expect(dialog.message()).toBe('You have unsaved changes. Discard them?');
  await dialog.dismiss();
  await click;
  await expect(page).toHaveURL(/#\/configs\/payment\/production$/);
  await expectEditorText(editor, 'port: 6380\n');

  await save.click();
  await expect(page.getByRole('alert')).toContainText('This resource changed after you opened it. Review the latest Revision and try again.');
  await expect(page.getByRole('button', { name: 'Copy Request ID request-abc123' })).toBeVisible();
  await expectEditorText(editor, 'port: 6380\n');

  await save.click();
  await expect(page.getByRole('status')).toContainText('Saved as v3.');
  await expect(page.getByRole('button', { name: 'v3 Current' })).toBeVisible();
  await expect(save).toBeDisabled();
});

test('administrator compares, restores, and clones an immutable Config Revision', async ({ page }) => {
  await mockIdentity(page);
  let current = { environment_key: 'production', config_key: 'payment', config_name: 'Payment service', format: 'yaml', content: 'port: 6380\n', revision: 3 };
  const revisions = [
    { revision: 3, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T03:00:00Z' },
    { revision: 2, format: 'yaml', actor_id: 'alex@example.com', created_at: '2026-08-27T02:00:00Z' },
  ];
  let restore;
  let clone;
  await page.route('**/v1/environments/production/configs/payment**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname.endsWith('/revisions/2')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ...current, content: 'port: 6379\n', revision: 2 }) });
      return;
    }
    if (pathname.endsWith('/revisions')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: revisions }) });
      return;
    }
    if (pathname.endsWith('/restore')) {
      restore = { body: request.postDataJSON(), operation: request.headers()['idempotency-key'] };
      current = { ...current, content: 'port: 6379\n', revision: 4 };
      revisions.unshift({ revision: 4, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T04:00:00Z' });
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":4}' });
      return;
    }
    if (pathname.endsWith('/clone')) {
      clone = { body: request.postDataJSON(), operation: request.headers()['idempotency-key'] };
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":1}' });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(current) });
  });
  await page.goto('/ui/#/configs/payment/production');

  const v2 = page.getByLabel('Revision history').getByRole('button', { name: /v2/ });
  await v2.click();
  await expect(v2).toHaveAttribute('aria-pressed', 'true');
  await expectEditorText(page.getByLabel('Revision v2'), 'port: 6379\n');
  await expect(page.getByLabel('Revision v2')).toHaveAttribute('aria-readonly', 'true');
  await expect(page.locator('.compare-picker')).toHaveCount(0);
  const v3 = page.getByLabel('Revision history').getByRole('button', { name: 'v3 Current' });
  await v3.click();
  await expect(v3).toHaveAttribute('aria-pressed', 'true');
  await expectEditorText(page.getByLabel('Configuration source'), 'port: 6380\n');
  await expect(page.getByLabel('Configuration source')).toHaveAttribute('aria-readonly', 'false');
  await page.getByRole('button', { name: 'Compare', exact: true }).click();
  await expect(page.getByLabel('Revision history')).toHaveCount(0);
  await page.getByTestId('revision-production-2').first().dragTo(page.getByTestId('compare-source-slot'));
  await page.getByTestId('revision-production-3').first().dragTo(page.getByTestId('compare-target-slot'));
  await expectEditorText(page.getByLabel('production/payment@v2'), 'port: 6379\n');
  await expectEditorText(page.getByLabel('production/payment@v3'), 'port: 6380\n');
  await page.getByRole('button', { name: 'Use dark theme' }).click();
  await expect(page.locator('.config-diff-editor .cm-line span').first()).toHaveCSS('color', 'rgb(130, 183, 255)');
  await page.getByRole('button', { name: 'Cancel' }).click();
  await page.getByRole('tab', { name: 'History' }).click();
  await page.getByRole('button', { name: 'Restore v2' }).click();
  await page.getByRole('button', { name: 'Confirm restore v2' }).click();
  expect(restore.body).toEqual({ source_revision: 2, expected_revision: 3 });
  expect(restore.operation).toMatch(/^[0-9a-f-]{36}$/);

  await page.locator('summary[aria-label="More actions"]').click();
  await page.getByRole('button', { name: 'Clone', exact: true }).click();
  await page.getByLabel('Target environment').fill('recovery');
  await page.getByLabel('Target config').fill('payment_recovery');
  await page.getByLabel('Target name').fill('Recovery payment');
  await page.getByRole('button', { name: 'Clone config' }).click();
  expect(clone.body).toEqual({ target_environment: 'recovery', target_config: 'payment_recovery', target_name: 'Recovery payment' });
  expect(clone.operation).toMatch(/^[0-9a-f-]{36}$/);
});

test('viewer compares arbitrary Config revisions across peer Environments', async ({ page }) => {
  await mockIdentity(page, 'viewer');
  await page.route('**/v1/configs*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ key: 'payment', display_name: 'Payment', environments: [{ key: 'production', revision: 3 }, { key: 'recovery', revision: 7 }] }] }),
  }));
  await page.route('**/v1/environments', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/v1/vault-items', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/v1/environments/**/configs/**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.endsWith('/revisions')) {
      const recovery = pathname.includes('/recovery/');
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ revision: recovery ? 7 : 3, format: 'yaml', actor_id: 'admin', created_at: '2026-08-27T03:00:00Z' }] }) });
      return;
    }
    if (pathname === '/v1/environments/production/configs/payment') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ environment_key: 'production', config_key: 'payment', config_name: 'Payment', format: 'yaml', content: 'port: 6380\n', revision: 3 }) });
      return;
    }
    const content = pathname.includes('/recovery/') ? 'port: 6379\n' : 'port: 6380\n';
    const revision = Number(pathname.split('/').at(-1));
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ format: 'yaml', content, revision }) });
  });
  await page.goto('/ui/#/configs/payment/production');
  await page.getByRole('tab', { name: 'History' }).click();
  await expect(page.locator('.compare-picker')).toHaveCount(0);
  await page.getByRole('button', { name: 'Compare', exact: true }).click();

  await expect(page.getByTestId('compare-source-slot')).toContainText('Drop a revision here');
  await expect(page.getByTestId('compare-target-slot')).toContainText('Drop a revision here');
  await page.getByTestId('revision-recovery-7').dragTo(page.getByTestId('compare-source-slot'));
  await page.getByTestId('revision-production-3').dragTo(page.getByTestId('compare-target-slot'));

  await expectEditorText(page.getByLabel('recovery/payment@v7'), 'port: 6379\n');
  await expectEditorText(page.getByLabel('production/payment@v3'), 'port: 6380\n');
  await expect(page.locator('.config-diff-editor .cm-changedLine').first()).toBeVisible();
});

test('administrator sees a format mismatch for Merge and can Replace across formats', async ({ page }) => {
  await mockIdentity(page);
  const yaml = 'database:\n  host: source\n';
  const json = '{\n  "database": {"host": "target"}\n}\n';
  const previewModes = [];
  await page.route('**/v1/configs*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ key: 'payment', display_name: 'Payment', environments: [{ key: 'production', revision: 2 }, { key: 'recovery', revision: 1 }] }] }),
  }));
  await page.route('**/v1/environments', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/v1/vault-items', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/v1/environments/**/configs/payment**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    const recovery = pathname.includes('/recovery/');
    if (pathname.endsWith('/revisions')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ revision: recovery ? 1 : 2, format: recovery ? 'json' : 'yaml', actor_id: 'admin', created_at: '2026-08-27T03:00:00Z' }] }) });
      return;
    }
    if (pathname.endsWith('/transfer-preview')) {
      const body = route.request().postDataJSON();
      previewModes.push(body.mode);
      await route.fulfill(body.mode === 'merge'
        ? { status: 422, contentType: 'application/json', body: '{"error":{"code":"format_mismatch","request_id":"test"}}' }
        : { status: 200, contentType: 'application/json', body: JSON.stringify({ mode: 'replace', format: 'yaml', content: yaml, source_revision: 2, target_revision: 1, expected_target_revision: 1 }) });
      return;
    }
    if (pathname === '/v1/environments/production/configs/payment') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ environment_key: 'production', config_key: 'payment', config_name: 'Payment', format: 'yaml', content: yaml, revision: 2 }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ format: recovery ? 'json' : 'yaml', content: recovery ? json : yaml, revision: recovery ? 1 : 2 }) });
  });
  await page.goto('/ui/#/configs/payment/production');

  await page.getByRole('button', { name: 'Merge', exact: true }).click();
  await page.getByTestId('revision-production-2').dragTo(page.getByTestId('compare-source-slot'));
  await page.getByTestId('revision-recovery-1').dragTo(page.getByTestId('compare-target-slot'));
  await expect(page.getByText('Cannot merge: Source is YAML, while Target is JSON. Convert them to the same format first.')).toBeVisible();

  await page.getByRole('button', { name: 'Replace', exact: true }).click();
  await page.getByTestId('revision-production-2').dragTo(page.getByTestId('compare-source-slot'));
  await page.getByTestId('revision-recovery-1').dragTo(page.getByTestId('compare-target-slot'));
  await expectEditorText(page.getByLabel('Result · after'), yaml);
  await expect(page.locator('.transfer-result .cm-merge-a .cm-changedLine').first()).toBeVisible();
  await expect(page.locator('.transfer-result .cm-merge-b .cm-changedLine').first()).toBeVisible();
  expect(previewModes).toEqual(['merge', 'replace']);
});

test('administrator previews and commits one directional Config merge', async ({ page }) => {
  await mockIdentity(page);
  let current = {
    environment_key: 'b', config_key: 'payment', config_name: 'Payment service',
    format: 'yaml', content: 'database:\n  host: target\n  target_only: keep\n', revision: 5,
  };
  const targetRevisions = [
    { revision: 5, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T01:00:00Z' },
    { revision: 4, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-26T01:00:00Z' },
  ];
  let previewRequest;
  let mergeRequest;
  await page.route('**/v1/configs*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ key: 'payment', display_name: 'Payment service', environments: [{ key: 'a', revision: 3 }, { key: 'b', revision: 5 }] }] }),
  }));
  await page.route('**/v1/environments', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/v1/vault-items', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/v1/environments/**/configs/payment**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/revisions')) {
      const items = url.pathname.includes('/a/')
        ? [{ revision: 3, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T02:00:00Z' }]
        : targetRevisions;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items }) });
      return;
    }
    if (url.pathname.endsWith('/transfer-preview')) {
      previewRequest = route.request().postDataJSON();
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
        mode: 'merge', format: 'yaml', content: 'database:\n  host: source\n  historical_only: keep\n', source_revision: 3, target_revision: 4, expected_target_revision: 5,
      }) });
      return;
    }
    if (url.pathname.endsWith('/merge')) {
      mergeRequest = { body: route.request().postDataJSON(), operation: route.request().headers()['idempotency-key'] };
      current = { ...current, content: 'database:\n  host: source\n  historical_only: keep\n', revision: 6 };
      targetRevisions.unshift({ revision: 6, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T03:00:00Z' });
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":6}' });
      return;
    }
    if (url.pathname.endsWith('/revisions/3') && url.pathname.includes('/a/')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ format: 'yaml', content: 'database:\n  host: source\n', revision: 3 }) });
      return;
    }
    if (url.pathname.endsWith('/revisions/5')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ format: 'yaml', content: 'database:\n  host: target\n  target_only: keep\n', revision: 5 }) });
      return;
    }
    if (url.pathname.endsWith('/revisions/4')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ format: 'yaml', content: 'database:\n  host: historical\n  historical_only: keep\n', revision: 4 }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(current) });
  });
  await page.goto('/ui/#/configs/payment/b');

  await expect(page.getByRole('tab', { name: 'Transfer' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Replace', exact: true }).click();
  await expect(page.getByLabel('Revision history')).toHaveCount(0);
  await expect(page.getByTestId('compare-source-slot')).toContainText('Drop a revision here');
  await expect(page.getByTestId('compare-target-slot')).toContainText('Drop a revision here');
  await expect(page.getByRole('button', { name: 'Use b@v4 as Target' })).toBeEnabled();
  await page.getByRole('button', { name: 'Merge', exact: true }).click();
  await page.getByTestId('revision-a-3').dragTo(page.getByTestId('compare-source-slot'));
  await page.getByTestId('revision-b-4').dragTo(page.getByTestId('compare-target-slot'));

  await expectEditorText(page.getByLabel('Result · after'), 'database:\n  host: source\n  historical_only: keep\n');
  await expect(page.locator('.transfer-result .cm-merge-a .cm-changedLine').first()).toBeVisible();
  await expect(page.locator('.transfer-result .cm-merge-b .cm-changedLine').first()).toBeVisible();
  expect(previewRequest).toEqual({ mode: 'merge', source_environment: 'a', source_config: 'payment', source_revision: 3, target_revision: 4 });
  await expect(page.locator('.transfer-direction')).toContainText('a/payment@v3→b/payment@v4');
  await expect(page.locator('.transfer-conflict-hint')).toContainText('current at v5');
  await page.getByRole('button', { name: 'Merge into b' }).click();

  expect(mergeRequest.body).toEqual({ source_environment: 'a', source_config: 'payment', source_revision: 3, target_revision: 4, expected_target_revision: 5 });
  expect(mergeRequest.operation).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByRole('button', { name: 'v6 Current' })).toBeVisible();
});

test('administrator explicitly reveals the resolved Config and its exact Vault revisions', async ({ page }) => {
  await mockIdentity(page);
  let resolvedReads = 0;
  const current = {
    environment_key: 'production', config_key: 'payment', config_name: 'Payment service', format: 'yaml',
    content: "password: '{vault.platform.mysql.password}'\n", revision: 7,
  };
  await page.route('**/v1/environments/production/configs/payment**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/revisions')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ revision: 7, format: 'yaml', actor_id: 'admin@example.com', created_at: '2026-08-27T01:00:00Z' }] }) });
      return;
    }
    if (url.pathname.endsWith('/resolved-preview')) {
      resolvedReads += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ format: 'yaml', content: 'password: resolved-secret\n', config_revision: 7, vault_revisions: { 'platform.mysql': 4 } }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(current) });
  });
  await page.goto('/ui/#/configs/payment/production');

  await page.getByRole('tab', { name: 'Resolved preview' }).click();
  expect(resolvedReads).toBe(0);
  await page.getByRole('button', { name: 'Reveal resolved values' }).click();

  expect(resolvedReads).toBe(1);
  await expectEditorText(page.getByLabel('Resolved configuration'), 'password: resolved-secret\n');
  await expect(page.getByText('platform.mysql @ v4')).toBeVisible();
});

test('viewer browses versioned Vault Item metadata without value controls', async ({ page }) => {
  await mockIdentity(page, 'viewer');
  await page.route('**/v1/vault-items*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [
      { namespace_key: 'platform', key: 'redis', display_name: 'Redis connection', revision: 4, archived: false, updated_at: '2026-08-27T01:00:00Z' },
      { namespace_key: 'legacy', key: 'redis', display_name: 'Legacy key', revision: 2, archived: true, updated_at: '2026-08-26T01:00:00Z' },
    ] }),
  }));
  await page.goto('/ui/#/vault');

  await expect(page.getByRole('heading', { name: 'Vault' })).toBeVisible();
  await expect(page.getByRole('link', { name: /Redis connection.*v4/ })).toHaveAttribute('href', '#/vault/platform/redis');
  await expect(page.getByRole('row', { name: /platform.*redis.*Redis connection/ })).toBeVisible();
  await expect(page.getByRole('row', { name: /legacy.*redis.*Legacy key/ })).toBeVisible();
  await expect(page.getByText('Archived', { exact: true })).toBeVisible();
  await page.getByLabel('Filter by namespace').selectOption('platform');
  await expect(page.getByRole('row', { name: /legacy.*redis.*Legacy key/ })).toHaveCount(0);
  await expect(page.getByRole('button', { name: /new vault/i })).toHaveCount(0);
});

test('archived Vault Item keeps metadata and history but has no value or mutation controls', async ({ page }) => {
  await mockIdentity(page);
  await page.route('**/v1/vault-items/platform/redis**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.endsWith('/revisions')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
        { revision: 2, actor_id: 'admin@example.com', created_at: '2026-08-27T02:00:00Z' },
        { revision: 1, actor_id: 'admin@example.com', created_at: '2026-08-27T01:00:00Z' },
      ] }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
      namespace_key: 'platform', key: 'redis', display_name: 'Redis', revision: 2, archived: true,
      snapshot: { fields: [{ key: 'password', name: 'Password', type: 'secret' }], variants: [{ id: '01', environments: ['production'] }] },
    }) });
  });
  await page.goto('/ui/#/vault/platform/redis');

  await expect(page.getByText('Archived', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Edit item' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Reveal item values' })).toHaveCount(0);
  await page.getByRole('tab', { name: 'History' }).click();
  await expect(page.getByRole('button', { name: /Restore/ })).toHaveCount(0);
});

test('viewer opens a Vault Revision beyond the rail from History and returns to current', async ({ page }) => {
  await mockIdentity(page, 'viewer');
  const metadata = {
    namespace_key: 'platform', key: 'redis', display_name: 'Redis current', revision: 12, archived: false,
    snapshot: { fields: [{ key: 'host', name: 'Current host', type: 'text' }], variants: [{ id: '01', environments: ['production'] }] },
  };
  const historical = {
    ...metadata, display_name: 'Redis v1', revision: 1,
    snapshot: { fields: [{ key: 'host', name: 'Historical host', type: 'text' }], variants: [{ id: '01', environments: ['production'] }] },
  };
  const revisions = Array.from({ length: 12 }, (_, index) => ({ revision: 12 - index, actor_id: 'admin@example.com', created_at: `2026-08-${String(27 - index).padStart(2, '0')}T01:00:00Z` }));
  await page.route('**/v1/vault-items/platform/redis**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.endsWith('/revisions/1')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(historical) });
    if (pathname.endsWith('/revisions')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: revisions }) });
    if (pathname.endsWith('/usages')) return route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' });
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(metadata) });
  });
  await page.goto('/ui/#/vault/platform/redis');

  await expect(page.getByLabel('Revision history').getByText('v1', { exact: true })).toHaveCount(0);
  await page.getByRole('tab', { name: 'History' }).click();
  await page.getByRole('button', { name: 'View v1', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Redis v1' })).toBeVisible();
  await expect(page.getByText('Read-only', { exact: true })).toBeVisible();
  await expect(page.getByText('Historical host', { exact: true })).toBeVisible();

  await page.getByLabel('Revision history').getByRole('button', { name: /v12/ }).click();
  await expect(page.getByRole('heading', { name: 'Redis current' })).toBeVisible();
  await expect(page.getByLabel('Revision history').getByRole('button', { name: /v12/ })).toHaveAttribute('aria-pressed', 'true');
});

test('administrator creates and controls the soft lifecycle of a Vault Item snapshot', async ({ page }) => {
  await mockIdentity(page);
  await page.route('**/v1/environments', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
    { key: 'production', display_name: 'Production', archived: false },
    { key: 'staging', display_name: 'Staging', archived: false },
  ] }) }));
  const items = [];
  const lifecycle = [];
  let commit;
  await page.route('**/v1/vault-items**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === 'PUT') {
      commit = { body: request.postDataJSON(), operation: request.headers()['idempotency-key'] };
      items.push({ namespace_key: 'platform', key: 'redis', display_name: commit.body.display_name, revision: 1, archived: false, updated_at: '2026-08-27T01:00:00Z' });
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":1}' });
      return;
    }
    if (request.method() === 'POST') {
      const action = pathname.endsWith('/unarchive') ? 'unarchive' : 'archive';
      lifecycle.push({ action, operation: request.headers()['idempotency-key'] });
      items[0].archived = action === 'archive';
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', key: 'redis', archived: items[0].archived }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items }) });
  });
  await page.goto('/ui/#/vault');

  await page.getByRole('button', { name: 'New Vault item' }).click();
  await expect(page.getByLabel('Snapshot JSON')).toHaveCount(0);
  if (process.env.CONFIGRA_VAULT_CREATE_SCREENSHOT) await page.screenshot({ path: process.env.CONFIGRA_VAULT_CREATE_SCREENSHOT, fullPage: true });
  await page.getByLabel('Namespace').fill('platform');
  await page.getByLabel('Resource key').fill('redis');
  await page.getByLabel('Display name').fill('Redis connection');
  await page.getByLabel('Field key 1').fill('password');
  await page.getByLabel('Field name 1').fill('Password');
  await page.getByLabel('Field type 1').selectOption('secret');
  await page.getByLabel('Variant 1 Password value').fill('secret-value');
  await page.getByRole('button', { name: 'Add field' }).click();
  await page.getByLabel('Field key 2').fill('password');
  await page.getByLabel('Field name 2').fill('CA bundle');
  await page.getByLabel('Field type 2').selectOption('file');
  await page.getByLabel('Variant 1 CA bundle value').setInputFiles({ name: 'mysql-ca.pem', mimeType: 'application/x-pem-file', buffer: Buffer.from('certificate-bytes') });
  await page.getByRole('button', { name: 'Create Vault item' }).click();
  await expect(page.getByText('Complete every Field value and use a unique valid Field key.')).toBeVisible();
  expect(commit).toBeUndefined();
  await page.getByLabel('Field key 2').fill('ca');
  await page.getByLabel('Variant 1 Staging staging').check();
  await page.getByRole('button', { name: 'Add Variant' }).click();
  await page.getByLabel('Variant 2 Staging staging').check();
  await page.getByLabel('Variant 2 Password value').fill('staging-secret');
  await page.getByLabel('Variant 2 CA bundle value').setInputFiles({ name: 'staging-ca.pem', mimeType: 'application/x-pem-file', buffer: Buffer.from('staging-certificate') });
  await page.getByRole('button', { name: 'Create Vault item' }).click();

  expect(commit.body.display_name).toBe('Redis connection');
  expect(commit.body.expected_revision).toBe(0);
  expect(commit.body.snapshot.fields).toEqual([
    { key: 'password', name: 'Password', type: 'secret' },
    { key: 'ca', name: 'CA bundle', type: 'file' },
  ]);
  expect(commit.body.snapshot.variants).toHaveLength(2);
  expect(commit.body.snapshot.variants[0].id).toMatch(/^[0-9a-f]{32}$/);
  expect(commit.body.snapshot.variants[0].environments).toEqual(['production']);
  expect(commit.body.snapshot.variants[0].values).toEqual({
    password: { text: 'secret-value' },
    ca: { file: { filename: 'mysql-ca.pem', content_type: 'application/x-pem-file', bytes: 'Y2VydGlmaWNhdGUtYnl0ZXM=' } },
  });
  expect(commit.body.snapshot.variants[1].environments).toEqual(['staging']);
  expect(commit.body.snapshot.variants[1].values.password.text).toBe('staging-secret');
  expect(commit.operation).toMatch(/^[0-9a-f-]{36}$/);
  await page.getByRole('button', { name: 'Archive Redis connection' }).click();
  await page.getByRole('button', { name: 'Confirm archive Redis connection' }).click();
  await page.getByRole('button', { name: 'Unarchive Redis connection' }).click();
  expect(lifecycle.map(item => item.action)).toEqual(['archive', 'unarchive']);
});

test('administrator selects a Vault Variant, copies references, and explicitly reveals values', async ({ page, context }) => {
  if (process.env.CONFIGRA_UI_MOBILE === '1') await page.setViewportSize({ width: 390, height: 844 });
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: 'http://127.0.0.1:4173' });
  await mockIdentity(page);
  const metadata = {
    namespace_key: 'platform', key: 'redis', display_name: 'Redis connection', revision: 4, archived: false,
    snapshot: {
      fields: [
        { key: 'username', name: 'Username', type: 'text' },
        { key: 'password', name: 'Password', type: 'secret' },
        { key: 'tls_cert', name: 'TLS certificate', type: 'file' },
      ],
      variants: [
        { id: '01010101010101010101010101010101', environments: ['a', 'b'] },
        { id: '02020202020202020202020202020202', environments: ['recovery'] },
      ],
    },
  };
  const valued = structuredClone(metadata);
  valued.snapshot.variants[0].values = {
    username: { text: 'redis-user' },
    password: { text: 'vault-secret-sentinel' },
    tls_cert: { file: { filename: 'server.pem', content_type: 'application/x-pem-file', bytes: 'Y2VydA==' } },
  };
  valued.snapshot.variants[1].values = {
    username: { text: 'recovery-user' }, password: { text: 'recovery-secret' },
    tls_cert: { file: { filename: 'recovery.pem', content_type: 'application/x-pem-file', bytes: 'Y2VydA==' } },
  };
  const historical = structuredClone(metadata);
  historical.display_name = 'Redis connection · historical';
  historical.revision = 3;
  historical.snapshot.fields = historical.snapshot.fields.slice(0, 2);
  const historicalValued = structuredClone(historical);
  historicalValued.snapshot.variants[0].values = { username: { text: 'redis-user-v3' }, password: { text: 'vault-secret-v3' } };
  historicalValued.snapshot.variants[1].values = { username: { text: 'recovery-user-v3' }, password: { text: 'recovery-secret-v3' } };
  let valueReads = 0;
  await page.route('**/v1/vault-items/platform/redis**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.endsWith('/revisions')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
        { revision: 4, actor_id: 'alex@example.com', created_at: '2026-08-27T01:00:00Z' },
        { revision: 3, actor_id: 'bob@example.com', created_at: '2026-08-26T01:00:00Z' },
      ] }) });
      return;
    }
    if (pathname.endsWith('/revisions/3/values')) {
      valueReads += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(historicalValued) });
      return;
    }
    if (pathname.endsWith('/revisions/3')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(historical) });
      return;
    }
    if (pathname.endsWith('/values')) {
      valueReads += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(valued) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(metadata) });
  });
  await page.goto('/ui/#/vault/platform/redis');

  await expect(page.getByRole('heading', { name: 'Redis connection' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Variant 1 · a, b' })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByText('{vault.platform.redis.username}', { exact: true })).toBeVisible();
  await expect(page.getByText('{vault.platform.redis.password}', { exact: true })).toBeVisible();
  await expect(page.getByText('Reference key', { exact: true })).toHaveCount(2);
  await expect(page.getByText('/v1/environments/<environment>/vault-items/platform/redis/fields/tls_cert/content', { exact: true })).toBeVisible();
  expect(valueReads).toBe(0);
  if (process.env.CONFIGRA_VAULT_SCREENSHOT) await page.screenshot({ path: process.env.CONFIGRA_VAULT_SCREENSHOT, fullPage: true });

  await page.getByRole('button', { name: 'Copy reference {vault.platform.redis.username}' }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('{vault.platform.redis.username}');
  await page.getByRole('button', { name: 'Reveal item values' }).click();
  expect(valueReads).toBe(1);
  await expect(page.getByText('redis-user', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Password', { exact: true })).toHaveAttribute('type', 'password');
  await page.getByRole('button', { name: 'Reveal Password' }).click();
  await expect(page.getByRole('button', { name: 'Hide Password' }).locator('.icon')).toBeVisible();
  await expect(page.getByLabel('Password', { exact: true })).toHaveAttribute('type', 'text');
  await expect(page.getByLabel('Password', { exact: true })).toHaveValue('vault-secret-sentinel');
  await expect(page.getByText('server.pem')).toBeVisible();
  const downloadStarted = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download TLS certificate' }).click();
  expect((await downloadStarted).suggestedFilename()).toBe('server.pem');

  await page.getByRole('tab', { name: 'History' }).click();
  await expect(page.getByRole('row', { name: /v3.*bob@example.com/ })).toBeVisible();
  await page.locator('.vault-revision-track button').filter({ hasText: 'v3' }).click();
  await expect(page.getByRole('heading', { name: 'Redis connection · historical' })).toBeVisible();
  await expect(page.getByText('Read-only', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Edit item' })).toHaveCount(0);
  await expect(page.getByText('TLS certificate', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Reveal item values' }).click();
  await expect(page.getByText('redis-user-v3', { exact: true })).toBeVisible();
  expect(valueReads).toBe(2);
});

test('Vault current references are value-free and destructive edits require impact confirmation', async ({ page }) => {
  await mockIdentity(page);
  const metadata = {
    namespace_key: 'platform', key: 'redis', display_name: 'Redis connection', revision: 4, archived: false,
    snapshot: {
      fields: [{ key: 'password', name: 'Password', type: 'secret' }, { key: 'host', name: 'Host', type: 'text' }],
      variants: [{ id: '01010101010101010101010101010101', environments: ['production'] }],
    },
  };
  const valued = structuredClone(metadata);
  valued.snapshot.variants[0].values = { password: { text: 'secret-never-rendered' }, host: { text: 'redis.internal' } };
  let commit;
  await page.route('**/v1/environments*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ key: 'production', display_name: 'Production', archived: false }] }) }));
  await page.route('**/v1/vault-items/platform/redis**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname.endsWith('/revisions')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ revision: 4, actor_id: 'admin@example.com', created_at: '2026-08-27T01:00:00Z' }] }) });
    if (pathname.endsWith('/usages')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ field_key: 'password', environment_key: 'production', config_key: 'payment', config_name: 'Payment service', config_revision: 7 }] }) });
    if (pathname.endsWith('/values')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(valued) });
    if (request.method() === 'PUT') {
      commit = request.postDataJSON();
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":5}' });
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(metadata) });
  });
  await page.goto('/ui/#/vault/platform/redis');

  await expect(page.getByRole('heading', { name: 'Current references' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Payment service production v7' })).toHaveAttribute('href', '#/configs/payment/production');
  await expect(page.getByText('secret-never-rendered')).toHaveCount(0);

  await page.getByRole('button', { name: 'Edit item' }).click();
  await page.getByRole('button', { name: 'Remove field Password' }).click();
  await expect(page.getByRole('alert')).toContainText('Payment service');
  await page.getByRole('button', { name: 'Save item' }).click();
  expect(commit).toBeUndefined();
  await page.getByRole('button', { name: 'Confirm save with impact' }).click();
  expect(commit.snapshot.fields.map(field => field.key)).toEqual(['host']);
});

test('administrator edits and restores a complete Vault Item snapshot', async ({ page }) => {
  await mockIdentity(page);
  let metadata = {
    namespace_key: 'platform', key: 'redis', display_name: 'Redis connection', revision: 4, archived: false,
    snapshot: { fields: [{ key: 'password', name: 'Password', type: 'secret' }], variants: [{ id: '01010101010101010101010101010101', environments: ['production'] }] },
  };
  const withValues = () => ({ ...metadata, snapshot: { ...metadata.snapshot, variants: [{ ...metadata.snapshot.variants[0], values: { password: { text: 'current-secret' } } }] } });
  const revisions = [
    { revision: 4, actor_id: 'admin@example.com', created_at: '2026-08-27T04:00:00Z' },
    { revision: 3, actor_id: 'alex@example.com', created_at: '2026-08-27T03:00:00Z' },
  ];
  let valueReads = 0;
  let commit;
  let restore;
  await page.route('**/v1/vault-items/platform/redis**', async route => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname.endsWith('/revisions')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: revisions }) });
      return;
    }
    if (pathname.endsWith('/values')) {
      valueReads += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(withValues()) });
      return;
    }
    if (pathname.endsWith('/restore')) {
      restore = { body: request.postDataJSON(), operation: request.headers()['idempotency-key'] };
      metadata = { ...metadata, revision: 6 };
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":6}' });
      return;
    }
    if (request.method() === 'PUT') {
      commit = { body: request.postDataJSON(), operation: request.headers()['idempotency-key'] };
      metadata = { ...metadata, display_name: commit.body.display_name, revision: 5 };
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","revision":5}' });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(metadata) });
  });
  await page.goto('/ui/#/vault/platform/redis');

  await page.getByRole('button', { name: 'Edit item' }).click();
  expect(valueReads).toBe(1);
  await expect(page.getByLabel('Snapshot JSON')).toHaveCount(0);
  await page.getByLabel('Display name').fill('Temporary name');
  const cancel = page.getByRole('button', { name: 'Cancel' });
  const cancelledClick = cancel.click();
  const cancelledDialog = await page.waitForEvent('dialog');
  expect(cancelledDialog.message()).toBe('You have unsaved changes. Discard them?');
  await cancelledDialog.dismiss();
  await cancelledClick;
  await expect(page.getByLabel('Display name')).toHaveValue('Temporary name');
  const confirmedClick = cancel.click();
  const confirmedDialog = await page.waitForEvent('dialog');
  await confirmedDialog.accept();
  await confirmedClick;
  await expect(page.getByRole('button', { name: 'Edit item' })).toBeVisible();

  await page.getByRole('button', { name: 'Edit item' }).click();
  await page.getByLabel('Display name').fill('Primary Redis');
  await page.getByLabel('Variant 1 Password value').fill('rotated-secret');
  await page.getByRole('button', { name: 'Save item' }).click();

  expect(commit.body.display_name).toBe('Primary Redis');
  expect(commit.body.expected_revision).toBe(4);
  expect(commit.body.snapshot.variants[0].values.password.text).toBe('rotated-secret');
  expect(commit.operation).toMatch(/^[0-9a-f-]{36}$/);
  await page.getByRole('tab', { name: 'History' }).click();
  await page.getByRole('button', { name: 'Restore v3' }).click();
  await page.getByRole('button', { name: 'Confirm restore v3' }).click();
  expect(restore.body).toEqual({ source_revision: 3, expected_revision: 5 });
  expect(restore.operation).toMatch(/^[0-9a-f-]{36}$/);
});
