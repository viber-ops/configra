import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

const authority = {
  id: '00112233445566778899aabbccddeeff', display_name: 'Production clients',
  fingerprint_sha256: '0123456789abcdef'.repeat(4), certificate_pem: '-----BEGIN CERTIFICATE-----\npublic-test-material\n-----END CERTIFICATE-----',
  not_before: '2026-01-01T00:00:00Z', not_after: '2031-09-11T00:00:00Z', created_at: '2026-09-11T00:00:00Z', revoked: false, client_certificate_count: 0,
};
const archive = 'UEsFBgAAAAAAAAAAAAAAAAAAAAAAAA==';

test('certificate pages retain issuer names and issuance excludes expired history with retryable failures', async ({ page }) => {
  await mockWorkspace(page);
  const authorities = Array.from({ length: 105 }, (_, index) => ({ ...authority, id: index.toString(16).padStart(32, '0'), display_name: `Authority ${String(index).padStart(3, '0')}`, not_after: index < 104 ? '2020-01-01T00:00:00Z' : authority.not_after }));
  const certificates = authorities.map((item, index) => ({ display_name: `Client ${String(index).padStart(3, '0')}`, fingerprint_sha256: index.toString(16).padStart(64, '0'), authority_id: authorities[0].id, authority_name: authorities[0].display_name, subject: `CN=client-${index}`, serial_hex: String(index), not_before: authority.not_before, not_after: authority.not_after, revoked: false }));
  let unavailable = true;
  await page.route('**/v1/certificate-authorities**', route => {
    const query = new URL(route.request().url()).searchParams;
    if (query.get('usable') === 'true' && unavailable) return route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":{"code":"service_unavailable","request_id":"issuer-retry-123"}}' });
    return route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, authorities) });
  });
  await page.route('**/v1/client-certificates**', route => route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, certificates) }));
  await page.goto('/ui/#/administration');
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  const caPanel = page.locator('.authority-panel');
  await expect(caPanel.locator('.authority-card')).toHaveCount(50);
  await caPanel.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(caPanel.getByRole('heading', { name: 'Authority 050' })).toBeVisible();
  await caPanel.getByRole('searchbox').fill('Authority 104');
  await expect(caPanel.locator('.authority-card')).toHaveCount(1);
  await page.getByRole('tab', { name: 'Client certificates' }).click();
  const panel = page.locator('.certificates-panel');
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  await expect(panel.locator('tbody tr').first()).toContainText('Authority 000');
  await panel.getByRole('searchbox').fill('Client 104');
  await expect(panel.locator('tbody tr')).toHaveCount(1);
  await expect(panel.locator('tbody tr')).toContainText('Authority 000');
  await panel.getByRole('searchbox').fill('missing');
  await expect(panel.getByText('No matching records.')).toBeVisible();
  await panel.getByRole('button', { name: 'Issue certificate', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('issuer-retry-123');
  await expect(panel.getByText('Create a certificate authority to issue client certificates here.')).toHaveCount(0);
  unavailable = false;
  await panel.getByRole('button', { name: 'Try again', exact: true }).click();
  await expect(panel.getByRole('combobox', { name: 'Authority', exact: true })).toHaveValue(authorities[104].id);
  await expect(panel.locator('select[name="authority_id"] option')).toHaveCount(1);
});

async function mockWorkspace(page) {
  await page.route('**/v1/**', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(new URL(route.request().url()).pathname === '/v1/me'
    ? { subject: 'admin', email: 'admin@example.com', role: 'admin' } : { items: [], total: 0 }) }));
}

test('administrator creates an Authority, saves its first export, issues and revokes client credentials', async ({ page }) => {
  await mockWorkspace(page);
  const authorities = [];
  const certificates = [];
  let createBody, issueBody, revoked = false;
  await page.route('**/v1/certificate-authorities**', async route => {
    const path = new URL(route.request().url()).pathname;
    let body = JSON.parse(inventoryPage(route, authorities));
    if (path.endsWith('/revoke')) {
      revoked = true; authorities[0].revoked = true; certificates.forEach(certificate => { certificate.revoked = true; });
      body = { outcome: 'success', authority: authorities[0] };
    } else if (route.request().method() === 'POST') {
      createBody = route.request().postDataJSON();
      authorities.push(structuredClone(authority));
      body = { outcome: 'success', authority: authorities[0], export_bundle: archive };
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
  await page.route('**/v1/client-certificates**', async route => {
    let body = JSON.parse(inventoryPage(route, certificates));
    if (new URL(route.request().url()).pathname.endsWith('/issue')) {
      issueBody = route.request().postDataJSON();
      const certificate = { display_name: issueBody.display_name, authority_id: authority.id, authority_name: authority.display_name, subject: `CN=${issueBody.display_name}`, fingerprint_sha256: 'abcdef0123456789'.repeat(4), serial_hex: 'AB12', not_before: '2026-01-01T00:00:00Z', not_after: '2027-01-01T00:00:00Z', revoked: false };
      certificates.push(certificate); authorities[0].client_certificate_count++;
      body = { outcome: 'success', certificate, export_bundle: archive };
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
  await page.goto('/ui/#/administration');
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  await page.getByRole('button', { name: 'Create CA', exact: true }).click();
  await page.getByLabel('Authority name', { exact: true }).fill('Production clients');
  await page.getByRole('button', { name: 'Generate certificate authority' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(createBody).toEqual({ display_name: 'Production clients', valid_days: 1825 });
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download credential bundle' }).click();
  expect((await download).suggestedFilename()).toBe('configra-ca-00112233.zip');
  await page.getByRole('button', { name: 'Saved, close' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  if (process.env.CONFIGRA_CA_SCREENSHOT) await page.screenshot({ path: process.env.CONFIGRA_CA_SCREENSHOT, fullPage: true });
  await page.reload();
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  await expect(page.getByRole('heading', { name: 'Production clients' })).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('tab', { name: 'Client certificates' }).click();
  await page.getByRole('button', { name: 'Issue certificate', exact: true }).click();
  await page.getByLabel('Certificate name', { exact: true }).fill('payments-api');
  await page.getByRole('button', { name: 'Issue client certificate', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(issueBody).toEqual({ display_name: 'payments-api', authority_id: authority.id, valid_days: 90 });
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('row', { name: /payments-api.*Production clients.*Active/ })).toBeVisible();
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  await page.getByRole('button', { name: 'Revoke CA', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('Every client certificate');
  await page.getByRole('button', { name: 'Confirm revoke', exact: true }).click();
  await expect.poll(() => revoked).toBe(true);
  await page.getByRole('tab', { name: 'Client certificates' }).click();
  await expect(page.getByRole('row', { name: /payments-api.*Revoked/ })).toBeVisible();
});

test('retry after a lost Authority response keeps the operation key and does not promise private-key recovery', async ({ page }) => {
  await mockWorkspace(page);
  const keys = [];
  await page.route('**/v1/certificate-authorities**', async route => {
    if (route.request().method() !== 'POST') return route.fulfill({ status: 200, contentType: 'application/json', body: inventoryPage(route, keys.length ? [authority] : []) });
    keys.push(route.request().headers()['idempotency-key']);
    if (keys.length === 1) return route.abort('failed');
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ outcome: 'success', authority }) });
  });
  await page.goto('/ui/#/administration');
  await page.getByRole('tab', { name: 'Certificate authorities' }).click();
  await page.getByRole('button', { name: 'Create CA', exact: true }).click();
  await page.getByLabel('Authority name', { exact: true }).fill('Production clients');
  await page.getByRole('button', { name: 'Generate certificate authority' }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await page.getByRole('button', { name: 'Generate certificate authority' }).click();
  await expect(page.getByRole('alert')).toContainText('already completed');
  expect(keys).toHaveLength(2);
  expect(keys[0]).toBe(keys[1]);
  await expect(page.getByRole('dialog')).toHaveCount(0);
});
