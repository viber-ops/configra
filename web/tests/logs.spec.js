import { expect, test } from '@playwright/test';

test('viewer distinguishes observed Access records from durable Audit attempts', async ({ page }) => {
  const auditRows = [
    { id: '00112233445566778899aabbccddeeff', time: '2026-08-27T02:00:00Z', event_type: 'config.updated', operation_id: 'operation-1', actor_type: 'user', actor_id: 'admin@example.com', action: 'config.commit', outcome: 'success', environment: 'production', resource_type: 'config', resource: 'payment', revision: 8, delivery_attempt: 1 },
    { id: 'ffeeddccbbaa99887766554433221100', time: '2026-08-27T02:01:00Z', event_type: 'vault.validation_failed', operation_id: 'operation-2', actor_type: 'user', actor_id: 'admin@example.com', action: 'vault.commit', outcome: 'validation_failed', environment: '', namespace: 'platform', resource_type: 'vault_item', resource: 'redis', revision: 0, delivery_attempt: 2 },
  ];
  const auditRequests = [];
  await page.route('**/v1/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ subject: 'viewer-1', email: 'viewer@example.com', role: 'viewer' }) }));
  await page.route('**/v1/access*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
    { time: '2026-08-27T01:00:00Z', principal: 'token-public-id', authentication: 'mtls', environment: 'production', resource_type: 'config', resource: 'payment', config_revision: 7, vault_revisions: { 'platform.mysql': 4 } },
    { time: '2026-08-27T01:01:00Z', principal: 'admin@example.com', authentication: 'oidc', environment: '', namespace: 'platform', resource_type: 'vault_item', resource: 'redis', config_revision: 0, vault_revisions: { 'platform.redis': 8 } },
  ] }) }));
  await page.route('**/v1/audit*', route => {
    const url = new URL(route.request().url());
    auditRequests.push(url.searchParams);
    const search = url.searchParams.get('q');
    const offset = Number(url.searchParams.get('offset'));
    const items = search ? auditRows.filter(row => row.operation_id === search) : offset ? [auditRows[1]] : auditRows;
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items, has_more: !search && offset === 0 }) });
  });
  await page.goto('/ui/#/access');

  await expect(page.getByRole('heading', { name: 'Access' })).toBeVisible();
  await expect(page.getByText('Observed responses', { exact: true })).toBeVisible();
  await expect(page.getByText('304 checks and dropped events are not included.')).toBeVisible();
  await expect(page.getByRole('row', { name: /token-public-id.*mtls.*production.*payment.*v7.*platform.mysql @ v4/ })).toBeVisible();
  await expect(page.getByRole('row', { name: /admin@example.com.*oidc.*platform\/redis.*platform.redis @ v8/ })).toBeVisible();

  await page.getByRole('link', { name: 'Audit' }).click();
  await expect(page.getByRole('heading', { name: 'Audit' })).toBeVisible();
  await expect(page.getByText('At-least-once delivery may produce duplicate rows.')).toBeVisible();
  await expect(page.getByRole('row', { name: /operation-1.*admin@example.com.*config.commit.*success.*payment.*v8/ })).toBeVisible();
  await expect(page.getByRole('row', { name: /operation-2.*validation_failed.*platform\/redis/ })).toBeVisible();
  expect(auditRequests[0].get('limit')).toBe('25');
  expect(auditRequests[0].get('offset')).toBe('0');
  await page.getByRole('button', { name: 'Next' }).click();
  await expect(page.getByText('Page 2')).toBeVisible();
  expect(auditRequests.at(-1).get('offset')).toBe('25');
  await page.getByPlaceholder('Operation or Request ID, actor, action, outcome, or resource').fill('operation-1');
  await page.getByRole('button', { name: 'Search', exact: true }).click();
  await expect(page.getByRole('row', { name: /operation-1.*payment/ })).toBeVisible();
  await expect(page.getByRole('row', { name: /operation-2/ })).toHaveCount(0);
  expect(auditRequests.at(-1).get('q')).toBe('operation-1');
  expect(auditRequests.at(-1).get('offset')).toBe('0');
  await expect(page.getByText('vault-secret-sentinel')).toHaveCount(0);
});

test('rejected requests show a searchable Request ID and reason without an OperationID', async ({ page }) => {
  const requestID = '0123456789abcdef01234567';
  await page.route('**/v1/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ subject: 'viewer-1', role: 'viewer' }) }));
  await page.route('**/v1/audit*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{
    id: '1234567890abcdef1234567890abcdef', time: '2026-09-12T00:00:00Z',
    operation_id: '', request_id: requestID, error_code: 'forbidden', actor_id: 'viewer-1',
    action: 'environment.create', outcome: 'validation_failed', resource_type: 'environment', resource: '', revision: 0, delivery_attempt: 1,
  }], has_more: false }) }));
  await page.goto('/ui/#/audit');
  await expect(page.getByRole('cell', { name: requestID, exact: true })).toBeVisible();
  await expect(page.getByText('forbidden', { exact: true })).toBeVisible();
  await page.getByRole('searchbox').fill(requestID);
  const searched = page.waitForRequest(request => request.url().includes('/v1/audit?') && new URL(request.url()).searchParams.get('q') === requestID);
  await page.getByRole('button', { name: 'Search', exact: true }).click();
  await searched;
});
