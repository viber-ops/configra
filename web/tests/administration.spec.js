import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

async function mockAdmin(page) {
  await page.route('**/v1/me', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ subject: 'admin-1', email: 'admin@example.com', role: 'admin' }),
  }));
}

test('Notification destinations and their subscriptions are independently paged', async ({ page }) => {
  await mockAdmin(page);
  const events = Array.from({ length: 105 }, (_, index) => `event.${String(index).padStart(3, '0')}`);
  const items = Array.from({ length: 105 }, (_, index) => ({ key: `target_${String(index).padStart(3, '0')}`, display_name: `Target ${index}`, provider: 'generic_webhook', safe_host: 'hooks.example.test', masked_suffix: '••••test', enabled: true, archived: false, event_types: events }));
  await page.route('**/v1/notification-destinations**', route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/event-types')) {
      const offset = Number(url.searchParams.get('offset') || 0), limit = Number(url.searchParams.get('limit') || 50);
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: events.slice(offset, offset + limit), total: events.length }) });
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, items) });
  });
  await page.goto('/ui/#/notifications');
  await expect(page.locator('.notifications-page tbody tr')).toHaveCount(50);
  await page.locator('.notifications-page > nav').getByRole('button', { name: 'Next' }).click();
  await expect(page.getByRole('row', { name: /Target 50 / })).toBeVisible();
  await page.locator('.notifications-page').getByRole('searchbox').fill('target_104');
  await expect(page.locator('.notifications-page tbody tr')).toHaveCount(1);
  await page.getByRole('button', { name: 'Event types Target 104' }).click();
  const subscriptions = page.locator('.delivery-section');
  await expect(subscriptions.getByText('event.049', { exact: true })).toBeVisible();
  await expect(subscriptions.getByText('event.050', { exact: true })).toHaveCount(0);
  await subscriptions.getByRole('button', { name: 'Next' }).click();
  await subscriptions.getByRole('button', { name: 'Next' }).click();
  await expect(subscriptions.getByText('event.104', { exact: true })).toBeVisible();
  await expect(subscriptions.getByRole('button', { name: 'Next' })).toBeDisabled();
});

test('administrator creates an Environment-scoped API Token and receives its plaintext once', async ({ page }) => {
  await mockAdmin(page);
  const tokens = [{
    public_id: '0123456789abcdef', display_name: 'Datacenter A', display_prefix: 'cfg_0123456789abcdef',
    environment_keys: ['production'], allow_without_mtls: false, revoked: false, created_at: '2026-08-27T01:00:00Z',
  }];
  let mutation;
  await page.route('**/v1/environments*', route => route.fulfill({
    status: 200, contentType: 'application/json', body: inventoryPage(route, [
      { key: 'production', display_name: 'Production', archived: false },
      { key: 'recovery', display_name: 'Recovery', archived: false },
    ]),
  }));
  await page.route('**/v1/api-tokens*', async route => {
    if (route.request().method() === 'POST') {
      mutation = { body: route.request().postDataJSON(), operation: route.request().headers()['idempotency-key'] };
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
        outcome: 'success', public_id: 'fedcba9876543210', display_prefix: 'cfg_fedcba9876543210',
        token: 'cfg_fedcba9876543210_one-time-secret', environment_keys: mutation.body.environment_keys,
        allow_without_mtls: mutation.body.allow_without_mtls,
      }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, tokens) });
  });
  await page.goto('/ui/#/administration');

  await expect(page.getByRole('heading', { name: 'Administration' })).toBeVisible();
  await expect(page.getByRole('row', { name: /Datacenter A.*production.*mTLS required/ })).toBeVisible();
  await page.getByRole('button', { name: 'New API token' }).click();
  await page.getByLabel('Token name').fill('Recovery reader');
  await page.getByRole('checkbox', { name: /Recovery/ }).check();
  await page.getByLabel('Allow this token without mTLS').check();
  await page.getByRole('checkbox', { name: 'Never expires' }).check();
  await page.getByRole('button', { name: 'Create API token' }).click();

  expect(mutation.body).toEqual({
    display_name: 'Recovery reader', environment_keys: ['recovery'], allow_without_mtls: true, never_expires: true,
  });
  expect(mutation.operation).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByText('cfg_fedcba9876543210_one-time-secret', { exact: true })).toBeVisible();
  await expect(page.getByText('This token will not be shown again.')).toBeVisible();
});

test('administrator imports a public client certificate and can permanently revoke it', async ({ page }) => {
  await mockAdmin(page);
  await page.route('**/v1/environments*', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[],"total":0}' }));
  await page.route('**/v1/api-tokens*', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[],"total":0}' }));
  const certificates = [{
    fingerprint_sha256: '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
    display_name: 'Datacenter A', subject: 'CN=datacenter-a', serial_hex: '01',
    not_before: '2026-01-01T00:00:00Z', not_after: '2027-01-01T00:00:00Z', revoked: false,
  }];
  let imported;
  let revoked;
  await page.route('**/v1/client-certificates**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.endsWith('/revoke')) {
      revoked = { fingerprint: pathname.split('/').at(-2), operation: route.request().headers()['idempotency-key'] };
      certificates[0].revoked = true;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', ...certificates[0] }) });
      return;
    }
    if (route.request().method() === 'POST') {
      imported = { body: route.request().postDataJSON(), operation: route.request().headers()['idempotency-key'] };
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', ...certificates[0], display_name: imported.body.display_name }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, certificates) });
  });
  await page.goto('/ui/#/administration');
  await page.getByRole('tab', { name: 'Client certificates' }).click();

  await expect(page.getByRole('row', { name: /Datacenter A.*CN=datacenter-a.*Active/ })).toBeVisible();
  await page.getByRole('button', { name: 'Import client certificate' }).click();
  await expect(page.getByText('Public certificate only. Private keys are never accepted.')).toBeVisible();
  await page.getByLabel('Certificate name').fill('Datacenter B');
  await page.getByLabel('Public certificate PEM').fill('-----BEGIN CERTIFICATE-----\npublic-only\n-----END CERTIFICATE-----');
  await page.getByRole('button', { name: 'Register certificate' }).click();

  expect(imported.body).toEqual({ display_name: 'Datacenter B', certificate_pem: '-----BEGIN CERTIFICATE-----\npublic-only\n-----END CERTIFICATE-----' });
  expect(imported.operation).toMatch(/^[0-9a-f-]{36}$/);
  await page.getByRole('button', { name: 'Revoke Datacenter A' }).click();
  await page.getByRole('button', { name: 'Confirm revoke Datacenter A' }).click();
  expect(revoked.fingerprint).toBe(certificates[0].fingerprint_sha256);
  expect(revoked.operation).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByText('Revoked', { exact: true })).toBeVisible();
});

test('administrator patches a Token Environment grant set and permanently revokes the Token', async ({ page }) => {
  await mockAdmin(page);
  const token = {
    public_id: '0123456789abcdef', display_name: 'Datacenter A', display_prefix: 'cfg_0123456789abcdef',
    environment_keys: ['production'], allow_without_mtls: false, revoked: false, created_at: '2026-08-27T01:00:00Z',
  };
  await page.route('**/v1/environments*', route => route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, [
    { key: 'production', display_name: 'Production', archived: false, granted: token.environment_keys.includes('production') }, { key: 'recovery', display_name: 'Recovery', archived: false, granted: token.environment_keys.includes('recovery') },
  ]) }));
  let grantChange;
  let revocation;
  await page.route('**/v1/api-tokens**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.endsWith('/environments')) {
      grantChange = { body: route.request().postDataJSON(), operation: route.request().headers()['idempotency-key'] };
      expect(route.request().method()).toBe('PATCH');
      token.environment_keys = [...token.environment_keys.filter(key => !grantChange.body.remove.includes(key)), ...grantChange.body.add];
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', public_id: token.public_id, environment_keys: token.environment_keys }) });
      return;
    }
    if (pathname.endsWith('/revoke')) {
      revocation = route.request().headers()['idempotency-key'];
      token.revoked = true;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', public_id: token.public_id, revoked: true }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, [token]) });
  });
  await page.goto('/ui/#/administration');

  await page.getByRole('button', { name: 'Edit environments Datacenter A' }).click();
  await page.getByRole('checkbox', { name: /Production/ }).uncheck();
  await page.getByRole('checkbox', { name: /Recovery/ }).check();
  await page.getByRole('button', { name: 'Save environment grants' }).click();
  expect(grantChange.body).toEqual({ add: ['recovery'], remove: ['production'] });
  expect(grantChange.operation).toMatch(/^[0-9a-f-]{36}$/);

  await page.getByRole('button', { name: 'Revoke Datacenter A' }).click();
  await page.getByRole('button', { name: 'Confirm revoke Datacenter A' }).click();
  expect(revocation).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByText('Revoked', { exact: true })).toBeVisible();
});

test('grant editing retains unseen permissions and pending changes across searches and pages', async ({ page }) => {
  await mockAdmin(page);
  const environments = Array.from({ length: 105 }, (_, index) => ({ key: `env_${String(index).padStart(3, '0')}`, display_name: `Environment ${index}`, granted: index % 2 === 0 }));
  const token = { public_id: '0123456789abcdef', display_name: 'Paged reader', display_prefix: 'cfg_0123456789abcdef', environment_keys: environments.filter(item => item.granted).map(item => item.key) };
  const reads = [];
  let mutation;
  await page.route('**/v1/environments*', route => {
    reads.push(new URL(route.request().url()).searchParams);
    return route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, environments) });
  });
  await page.route('**/v1/api-tokens**', route => {
    if (route.request().method() === 'PATCH') {
      mutation = route.request().postDataJSON();
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","public_id":"0123456789abcdef"}' });
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, [token]) });
  });
  await page.goto('/ui/#/administration');
  await page.getByRole('button', { name: 'Edit environments Paged reader' }).click();
  const form = page.locator('.grant-form');
  await expect(form.getByRole('checkbox')).toHaveCount(50);
  await expect(form.getByRole('checkbox', { name: /env_000$/ })).toBeChecked();
  await form.getByRole('checkbox', { name: /env_000$/ }).uncheck();
  await form.getByRole('button', { name: 'Next', exact: true }).click();
  await form.getByRole('checkbox', { name: /env_051$/ }).check();
  await form.getByRole('button', { name: 'Previous', exact: true }).click();
  await expect(form.getByRole('checkbox', { name: /env_000$/ })).not.toBeChecked();
  await form.getByRole('searchbox').fill('env_104');
  await expect(form.getByRole('checkbox')).toHaveCount(1);
  await expect(form.getByRole('checkbox', { name: /env_104$/ })).toBeChecked();
  await form.getByRole('button', { name: 'Save environment grants' }).click();
  expect(mutation).toEqual({ add: ['env_051'], remove: ['env_000'] });
  expect(reads.some(query => query.get('offset') === '50' && query.get('token') === token.public_id)).toBe(true);
  expect(reads.some(query => query.get('q') === 'env_104' && query.get('offset') === '0')).toBe(true);
  expect(reads.every(query => Number(query.get('limit')) <= 50)).toBe(true);
});

test('grant drafts distinguish inherited property names from explicit edits', async ({ page }) => {
  await mockAdmin(page);
  const token = { public_id: '0123456789abcdef', display_name: 'Constructor reader', environment_keys: ['constructor'] };
  let mutation;
  await page.route('**/v1/environments*', route => route.fulfill({
    contentType: 'application/json', body: inventoryPage(route, [
      { key: 'constructor', display_name: 'Constructor', granted: true },
      { key: 'production', display_name: 'Production', granted: false },
    ]),
  }));
  await page.route('**/v1/api-tokens**', route => {
    if (route.request().method() === 'PATCH') {
      mutation = route.request().postDataJSON();
      return route.fulfill({ contentType: 'application/json', body: '{"outcome":"success"}' });
    }
    return route.fulfill({ contentType: 'application/json', body: inventoryPage(route, [token]) });
  });
  await page.goto('/ui/#/administration');
  await page.getByRole('button', { name: 'Edit environments Constructor reader' }).click();
  const form = page.locator('.grant-form');
  const constructor = form.getByRole('checkbox', { name: /constructor$/ });
  await expect(constructor).toBeChecked();
  await expect(form.getByRole('button', { name: 'Save environment grants' })).toBeDisabled();
  await constructor.uncheck();
  await form.getByRole('searchbox').fill('production');
  await form.getByRole('checkbox', { name: /production$/ }).check();
  await form.getByRole('searchbox').fill('constructor');
  await expect(constructor).not.toBeChecked();
  await form.getByRole('button', { name: 'Save environment grants' }).click();
  expect(mutation).toEqual({ add: ['production'], remove: ['constructor'] });
});

test('administrator creates a Notification Destination without redisplaying URL credentials', async ({ page }) => {
  await mockAdmin(page);
  const items = [{
    key: 'operations', display_name: 'Operations', provider: 'generic_webhook', safe_host: 'hooks.example.com',
    masked_suffix: '••••inel', enabled: true, event_types: ['config.created'], archived: false,
    updated_at: '2026-08-27T01:00:00Z',
  }];
  let mutation;
  await page.route('**/v1/notification-destinations**', async route => {
    if (route.request().method() === 'PUT') {
      mutation = { body: route.request().postDataJSON(), operation: route.request().headers()['idempotency-key'] };
      items.push({ key: 'pager', display_name: mutation.body.display_name, provider: mutation.body.provider, safe_host: 'notify.example.com', masked_suffix: '••••cret', enabled: mutation.body.enabled, event_types: mutation.body.event_types, archived: false });
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', ...items.at(-1) }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, items) });
  });
  await page.goto('/ui/#/notifications');

  await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
  await expect(page.getByRole('row', { name: /Operations.*hooks.example.com.*••••inel.*Enabled/ })).toBeVisible();
  await page.getByRole('button', { name: 'New destination' }).click();
  await page.getByLabel('Destination key').fill('pager');
  await page.getByLabel('Destination name').fill('Pager');
  await page.getByLabel('Provider').selectOption('generic_webhook');
  await page.getByLabel('Webhook URL').fill('https://notify.example.com/hooks/url-token-sentinel');
  await page.getByLabel('Signing secret').fill('notification-secret-sentinel');
  await page.getByLabel('Event types').fill('config.created, vault.updated');
  await page.getByRole('button', { name: 'Save destination' }).click();

  expect(mutation.body).toEqual({
    display_name: 'Pager', provider: 'generic_webhook', url: 'https://notify.example.com/hooks/url-token-sentinel',
    secret: 'notification-secret-sentinel', enabled: true, event_types: ['config.created', 'vault.updated'],
  });
  expect(mutation.operation).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByRole('row', { name: /Pager.*notify.example.com.*••••cret/ })).toBeVisible();
  await expect(page.getByText('url-token-sentinel')).toHaveCount(0);
  await expect(page.getByText('notification-secret-sentinel')).toHaveCount(0);
});

test('administrator tests, inspects, redelivers, and archives a Notification Destination', async ({ page }) => {
  await mockAdmin(page);
  const destination = {
    key: 'operations', display_name: 'Operations', provider: 'generic_webhook', safe_host: 'hooks.example.com',
    masked_suffix: '••••inel', enabled: true, event_types: ['config.created'], archived: false,
  };
  let testOperation;
  let redeliveryOperation;
  let archiveOperation;
  let unarchiveOperation;
  await page.route('**/v1/notification-destinations**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (route.request().method() === 'GET' && pathname.endsWith('/deliveries')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
        { id: '00112233445566778899aabbccddeeff', event_type: 'config.created', attempt: 1, status: 'succeeded', http_status: 204, latency_ms: 32, created_at: '2026-08-27T01:00:00Z' },
        { id: 'ffeeddccbbaa99887766554433221100', event_type: 'vault.updated', attempt: 8, status: 'dead', error_code: 'attempt_limit_reached', latency_ms: 0, created_at: '2026-08-27T02:00:00Z' },
      ] }) });
      return;
    }
    if (pathname.endsWith('/test')) {
      testOperation = route.request().headers()['idempotency-key'];
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success"}' });
      return;
    }
    if (pathname.endsWith('/redeliver')) {
      redeliveryOperation = route.request().headers()['idempotency-key'];
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success"}' });
      return;
    }
    if (pathname.endsWith('/archive')) {
      archiveOperation = route.request().headers()['idempotency-key'];
      destination.archived = true;
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","archived":true}' });
      return;
    }
    if (pathname.endsWith('/unarchive')) {
      unarchiveOperation = route.request().headers()['idempotency-key'];
      destination.archived = false;
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"outcome":"success","archived":false}' });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, [destination]) });
  });
  await page.goto('/ui/#/notifications');

  await page.getByRole('button', { name: 'Send test Operations' }).click();
  expect(testOperation).toMatch(/^[0-9a-f-]{36}$/);
  await page.getByRole('button', { name: 'View deliveries Operations' }).click();
  await expect(page.getByRole('row', { name: /config.created.*succeeded.*204.*32/ })).toBeVisible();
  await expect(page.getByRole('row', { name: /vault.updated.*dead.*attempt_limit_reached/ })).toBeVisible();
  await page.getByRole('button', { name: 'Redeliver vault.updated' }).click();
  expect(redeliveryOperation).toMatch(/^[0-9a-f-]{36}$/);

  await page.getByRole('button', { name: 'Archive Operations' }).click();
  await page.getByRole('button', { name: 'Confirm archive Operations' }).click();
  expect(archiveOperation).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByText('Archived', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Unarchive Operations' }).click();
  expect(unarchiveOperation).toMatch(/^[0-9a-f-]{36}$/);
});

test('deployment status distinguishes observed readiness from configured architecture', async ({ page }) => {
  await mockAdmin(page);
  await page.route('**/v1/environments*', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[],"total":0}' }));
  await page.route('**/v1/api-tokens*', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[],"total":0}' }));
  await page.route('**/health/ready', route => route.fulfill({ status: 204 }));
  await page.goto('/ui/#/administration');
  await page.getByRole('tab', { name: 'Deployment status' }).click();

  await expect(page.getByText('Management ready', { exact: true })).toBeVisible();
  await expect(page.getByText('MySQL 8.0.22', { exact: true })).toBeVisible();
  await expect(page.getByText('ClickHouse', { exact: true })).toBeVisible();
  await expect(page.getByText('NATS', { exact: true })).toBeVisible();
  await expect(page.getByText('Cold-start YAML', { exact: true })).toBeVisible();
  await expect(page.getByText('API scales independently', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: /edit|save|change/i })).toHaveCount(0);
});
