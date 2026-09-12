import { chromium } from '@playwright/test';
import { mkdir, readFile } from 'node:fs/promises';
import path from 'node:path';

const baseURL = process.env.CONFIGRA_URL || 'https://localhost:18088';
const screenshotDirectory = process.env.CONFIGRA_SCREENSHOT_DIR || '.cache/ui-screenshots';
const command = process.argv[2] || 'all';

async function signIn(page, username = 'admin', password = 'configra-admin') {
  await page.goto(baseURL, { waitUntil: 'domcontentloaded' });
  if (await page.locator('#input').count() === 0) {
    await page.getByRole('link', { name: /SSO/i }).click();
    await page.locator('#input').waitFor();
  }
  await page.locator('#input').fill(username);
  await page.locator('#normal_login_password').fill(password);
  await page.getByRole('button', { name: 'Sign In' }).click();
  await page.waitForURL(url => url.origin === new URL(baseURL).origin);
  await page.locator('.app-shell').waitFor();
}

async function api(page, pathname, method = 'GET', body) {
  const response = await page.evaluate(async input => {
    const options = { method: input.method, headers: {} };
    if (input.body !== undefined) {
      options.headers['Content-Type'] = 'application/json';
      options.body = JSON.stringify(input.body);
    }
    if (input.method !== 'GET' && input.method !== 'HEAD') options.headers['Idempotency-Key'] = crypto.randomUUID();
    const result = await fetch(input.pathname, options);
    const text = await result.text();
    let payload = null;
    try { payload = text ? JSON.parse(text) : null; } catch { payload = text; }
    return { ok: result.ok, status: result.status, payload };
  }, { pathname, method, body });
  if (!response.ok) throw new Error(`${method} ${pathname}: HTTP ${response.status} ${JSON.stringify(response.payload)}`);
  return response.payload;
}

async function seed() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();
  await signIn(page);

  const environmentSeeds = [
    ['production', 'Production · Singapore'],
    ['staging', 'Staging / Integration'],
    ['development', 'Development · Local and shared'],
    ['recovery', 'Disaster recovery · Tokyo'],
    ['eu-west', 'Europe West · Frankfurt'],
    ['canary', 'Canary rollout · 5% traffic and automated rollback observation window'],
    ['qa', 'Quality assurance · Browser, API and migration verification'],
    ['legacy', 'Legacy datacenter · archived read-only context'],
  ];
  let environments = (await api(page, '/v1/environments?include_archived=true')).items;
  for (const [key, display_name] of environmentSeeds) {
    if (!environments.some(item => item.key === key)) await api(page, '/v1/environments', 'POST', { key, display_name });
  }
  environments = (await api(page, '/v1/environments?include_archived=true')).items;
  const legacy = environments.find(item => item.key === 'legacy');
  if (legacy && !legacy.archived) await api(page, '/v1/environments/legacy/archive', 'POST');

  const activeEnvironments = ['production', 'staging', 'development', 'recovery', 'eu-west', 'canary', 'qa'];
  const variants = (valuesFor, revision) => [
    { id: '11111111111111111111111111111111', environments: ['production', 'recovery', 'eu-west'], values: valuesFor('primary', revision) },
    { id: '22222222222222222222222222222222', environments: ['staging', 'development', 'canary', 'qa'], values: valuesFor('nonprod', revision) },
  ];
  const fileValue = (name, text) => ({ file: { filename: name, content_type: 'application/x-pem-file', bytes: Buffer.from(text).toString('base64') } });
  const vaultSeeds = [
    {
      namespace: 'platform', key: 'mysql', name: 'MySQL application credentials · primary regional clusters', revisions: 5,
      fields: [
        { key: 'username', name: 'Application username', type: 'text' },
        { key: 'password', name: 'Application password', type: 'secret' },
        { key: 'readonly_password', name: 'Read-only password', type: 'secret' },
        { key: 'host', name: 'Cluster endpoint', type: 'text' },
        { key: 'port', name: 'TCP port', type: 'text' },
        { key: 'tls_ca', name: 'Regional CA bundle', type: 'file' },
      ],
      values: (tier, revision) => ({
        username: { text: tier === 'primary' ? 'payment_runtime' : 'payment_nonprod' },
        password: { text: `${tier}-mysql-password-r${revision}-local-only` },
        readonly_password: { text: `${tier}-readonly-password-r${revision}-local-only` },
        host: { text: `mysql-${tier}.internal.example` }, port: { text: '3306' },
        tls_ca: fileValue(`mysql-${tier}-ca.pem`, `-----BEGIN CERTIFICATE-----\nLOCAL-${tier.toUpperCase()}-REV-${revision}\n-----END CERTIFICATE-----\n`),
      }),
    },
    {
      namespace: 'platform', key: 'redis', name: 'Redis cluster credentials', revisions: 3,
      fields: [{ key: 'host', name: 'Redis endpoint', type: 'text' }, { key: 'password', name: 'Redis password', type: 'secret' }, { key: 'database', name: 'Database index', type: 'text' }],
      values: (tier, revision) => ({ host: { text: `redis-${tier}.internal.example` }, password: { text: `${tier}-redis-r${revision}-local-only` }, database: { text: tier === 'primary' ? '0' : '4' } }),
    },
    {
      namespace: 'security', key: 'oidc', name: 'OIDC relying-party credentials', revisions: 2,
      fields: [{ key: 'issuer', name: 'Issuer URL', type: 'text' }, { key: 'client_id', name: 'Client ID', type: 'text' }, { key: 'client_secret', name: 'Client secret', type: 'secret' }],
      values: (tier, revision) => ({ issuer: { text: `https://identity-${tier}.example.com` }, client_id: { text: `configra-${tier}` }, client_secret: { text: `${tier}-oidc-r${revision}-local-only` } }),
    },
  ];
  for (const [index, item] of ['kafka', 'nats', 'clickhouse', 'object-storage', 'email-provider', 'payment-gateway', 'feature-flags', 'monitoring', 'pager', 'artifact-registry', 'dns-provider', 'cdn-edge'].entries()) {
    vaultSeeds.push({
      namespace: index < 3 ? 'platform' : index < 7 ? 'cloud' : 'operations', key: item,
      name: `${item.replaceAll('-', ' ')} integration credentials${index === 11 ? ' · deliberately long display name for truncation and responsive layout verification across production tables' : ''}`,
      revisions: 1,
      fields: [{ key: 'endpoint', name: 'Service endpoint', type: 'text' }, { key: 'token', name: 'Access token', type: 'secret' }],
      values: (tier, revision) => ({ endpoint: { text: `https://${item}-${tier}.services.example.com/api/v${revision}` }, token: { text: `${item}-${tier}-token-local-only` } }),
    });
  }
  vaultSeeds.push({
    namespace: 'legacy', key: 'ftp', name: 'Retired FTP credentials', revisions: 1, archive: true,
    fields: [{ key: 'username', name: 'Username', type: 'text' }, { key: 'password', name: 'Password', type: 'secret' }],
    values: tier => ({ username: { text: `retired-${tier}` }, password: { text: 'retired-local-only' } }),
    unbound: true,
  });

  let vaultItems = (await api(page, '/v1/vault-items?include_archived=true')).items;
  for (const item of vaultSeeds) {
    const existing = vaultItems.find(candidate => candidate.namespace_key === item.namespace && candidate.key === item.key);
    let revision = existing?.revision || 0;
    while (revision < item.revisions) {
      const snapshot = {
        fields: item.fields,
        variants: item.unbound
          ? [{ id: '11111111111111111111111111111111', environments: [], values: item.values('retired', revision + 1) }]
          : variants(item.values, revision + 1),
      };
      const result = await api(page, `/v1/vault-items/${item.namespace}/${item.key}`, 'PUT', { display_name: item.name, expected_revision: revision, snapshot });
      revision = result.revision;
    }
  }
  vaultItems = (await api(page, '/v1/vault-items?include_archived=true')).items;
  const retiredVault = vaultItems.find(item => item.namespace_key === 'legacy' && item.key === 'ftp');
  if (retiredVault && !retiredVault.archived) await api(page, '/v1/vault-items/legacy/ftp/archive', 'POST');

  const yamlContent = (service, environment, revision) => `service:\n  name: ${service}\n  environment: ${environment}\n  release: r${revision}\n  request_timeout: ${20 + revision}s\nserver:\n  address: 0.0.0.0\n  port: ${8000 + revision}\ndatabase:\n  host: "{vault.platform.mysql.host}"\n  port: "{vault.platform.mysql.port}"\n  username: "{vault.platform.mysql.username}"\n  password: "{vault.platform.mysql.password}"\ncache:\n  host: "{vault.platform.redis.host}"\n  password: "{vault.platform.redis.password}"\nfeatures:\n  checkout_v2: ${revision > 2}\n  metrics: true\n`;
  const jsonContent = (service, environment, revision) => JSON.stringify({ service: { name: service, environment, release: `r${revision}` }, oidc: { issuer: '{vault.security.oidc.issuer}', client_id: '{vault.security.oidc.client_id}', client_secret: '{vault.security.oidc.client_secret}' }, limits: { concurrent_requests: 100 + revision, queue_depth: 4000 } }, null, 2) + '\n';
  const configSeeds = [
    ['payment', 'Payment service · checkout, ledger reconciliation and settlement', 'yaml', { production: 7, staging: 5, development: 2, recovery: 4, 'eu-west': 2, canary: 2 }],
    ['worker', 'Asynchronous settlement worker', 'yaml', { production: 5, staging: 2, development: 2 }],
    ['identity-gateway-routing-policy-for-enterprise-west-coast', 'Identity gateway routing policy · enterprise customers · west-coast failover and compliance boundary', 'json', { production: 4, staging: 2, recovery: 2 }],
    ['public-api', 'Public API gateway', 'yaml', { production: 4, staging: 2, canary: 2 }],
    ['billing-policy', 'Billing rules and country-specific tax policy', 'json', { production: 2, staging: 2 }],
    ['shared-edge', 'Shared edge routing configuration across every active Environment', 'yaml', Object.fromEntries(activeEnvironments.map(environment => [environment, environment === 'production' ? 3 : 1]))],
  ];
  for (let index = 1; index <= 72; index += 1) {
    const key = `service-${String(index).padStart(3, '0')}`;
    const environment = activeEnvironments[index % activeEnvironments.length];
    const longSuffix = index % 17 === 0 ? ' · cross-region data-processing pipeline with a deliberately long operational display name used to verify ellipsis and maximum-width behavior' : '';
    configSeeds.push([key, `Service ${String(index).padStart(3, '0')} runtime configuration${longSuffix}`, index % 5 === 0 ? 'json' : 'yaml', { [environment]: 1 }]);
  }
  configSeeds.push(['obsolete-batch-worker', 'Obsolete batch worker · archived after migration', 'yaml', { production: 1 }]);

  const existingConfigs = (await api(page, '/v1/configs?include_archived=true')).items;
  for (const [key, name, format, contexts] of configSeeds) {
    if (existingConfigs.find(item => item.key === key)?.archived) continue;
    for (const [environment, desiredRevision] of Object.entries(contexts)) {
      let current = 0;
      try { current = (await api(page, `/v1/environments/${environment}/configs/${key}`)).revision; } catch (error) { if (!error.message.includes('HTTP 404')) throw error; }
      while (current < desiredRevision) {
        const next = current + 1;
        const content = format === 'json' ? jsonContent(key, environment, next) : yamlContent(key, environment, next);
        const result = await api(page, `/v1/environments/${environment}/configs/${key}`, 'PUT', { name, expected_revision: current, format, content });
        current = result.revision;
      }
    }
  }
  let configs = (await api(page, '/v1/configs?include_archived=true')).items;
  const obsolete = configs.find(item => item.key === 'obsolete-batch-worker');
  if (obsolete && !obsolete.archived) await api(page, '/v1/configs/obsolete-batch-worker/archive', 'POST');

  const notificationSeeds = [
    ['platform-events', 'Platform configuration changes · production webhook', 'generic_webhook', 'https://example.com/configra/hooks/platform-events-2026', 'local-signing-secret', true, ['config.created', 'config.updated', 'vault.created', 'vault.updated']],
    ['security-review', 'Security review archive stream', 'generic_webhook', 'https://example.org/automation/security-review', 'local-security-secret', false, ['vault.updated', 'client_certificate.registered', 'token.created']],
    ['feishu-incident-room', 'Feishu incident room · disabled until production handoff', 'feishu_bot', 'https://open.feishu.cn/open-apis/bot/v2/hook/local-development-token', 'local-feishu-secret', false, ['config.updated']],
    ['retired-webhook', 'Retired deployment webhook', 'generic_webhook', 'https://example.net/retired/configra', '', false, ['config.updated']],
  ];
  let destinations = (await api(page, '/v1/notification-destinations?include_archived=true')).items;
  for (const [key, display_name, provider, url, secret, enabled, event_types] of notificationSeeds) {
    if (!destinations.some(item => item.key === key)) await api(page, `/v1/notification-destinations/${key}`, 'PUT', { display_name, provider, url, secret, enabled, event_types });
  }
  destinations = (await api(page, '/v1/notification-destinations?include_archived=true')).items;
  const retiredDestination = destinations.find(item => item.key === 'retired-webhook');
  if (retiredDestination && !retiredDestination.archived) await api(page, '/v1/notification-destinations/retired-webhook/archive', 'POST');

  let tokens = (await api(page, '/v1/api-tokens?include_revoked=true')).items;
  const tokenSeeds = [
    ['Production edge readers · mTLS required', ['production', 'recovery', 'eu-west'], false, true],
    ['Developer laptops · token-only local access', ['development', 'staging'], true, true],
    ['Canary deployment automation with a deliberately long token display name', ['canary', 'production'], false, false],
    ['Retired QA smoke test token', ['qa'], true, true],
  ];
  for (const [display_name, environment_keys, allow_without_mtls, never_expires] of tokenSeeds) {
    if (!tokens.some(item => item.display_name === display_name)) {
      await api(page, '/v1/api-tokens', 'POST', { display_name, environment_keys, allow_without_mtls, never_expires, expires_at: never_expires ? null : new Date(Date.now() + 45 * 86400000).toISOString() });
    }
  }
  tokens = (await api(page, '/v1/api-tokens?include_revoked=true')).items;
  const retiredToken = tokens.find(item => item.display_name === 'Retired QA smoke test token');
  if (retiredToken && !retiredToken.revoked) await api(page, `/v1/api-tokens/${retiredToken.public_id}/revoke`, 'POST');

  const certificateSeeds = [
    ['Edge reader 01 · Singapore', '.cache/local-dev/client-edge.crt', false],
    ['Retired reader 02 · revoked example', '.cache/local-dev/client-revoked.crt', true],
  ];
  let certificates = (await api(page, '/v1/client-certificates?include_revoked=true')).items;
  for (const [display_name, filename] of certificateSeeds) {
    if (!certificates.some(item => item.display_name === display_name)) {
      const certificate_pem = await readFile(filename, 'utf8');
      await api(page, '/v1/client-certificates', 'POST', { display_name, certificate_pem });
    }
  }
  certificates = (await api(page, '/v1/client-certificates?include_revoked=true')).items;
  const revokedCertificate = certificates.find(item => item.display_name === certificateSeeds[1][0]);
  if (revokedCertificate && !revokedCertificate.revoked) await api(page, `/v1/client-certificates/${revokedCertificate.fingerprint_sha256}/revoke`, 'POST');

  for (let index = 0; index < 5; index += 1) await api(page, '/v1/vault-items/platform/mysql/values');
  for (const environment of ['production', 'staging', 'recovery']) await api(page, `/v1/environments/${environment}/configs/payment/resolved-preview`);
  await page.waitForTimeout(1800);

  const counts = {
    environments: (await api(page, '/v1/environments?include_archived=true')).items.length,
    configs: (await api(page, '/v1/configs?include_archived=true')).items.length,
    vaultItems: (await api(page, '/v1/vault-items?include_archived=true')).items.length,
    tokens: (await api(page, '/v1/api-tokens?include_revoked=true')).items.length,
    certificates: (await api(page, '/v1/client-certificates?include_revoked=true')).items.length,
    destinations: (await api(page, '/v1/notification-destinations?include_archived=true')).items.length,
    access: (await api(page, '/v1/access?limit=100')).items.length,
    audit: (await api(page, '/v1/audit?limit=100')).items.length,
  };
  console.log(JSON.stringify(counts, null, 2));
  await browser.close();
}

async function settle(page, selector = '.page') {
  await page.locator(selector).first().waitFor({ state: 'visible' });
  await page.waitForFunction(() => !document.querySelector('.collection-state .loading-line'));
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(180);
}

async function assertNoDocumentOverflow(page, name) {
  const { documentWidth, viewportWidth } = await page.evaluate(() => ({
    documentWidth: document.documentElement.scrollWidth,
    viewportWidth: window.innerWidth,
  }));
  if (documentWidth > viewportWidth + 1) throw new Error(`${name}: document width ${documentWidth}px exceeds viewport ${viewportWidth}px`);
}

async function screenshots() {
  await mkdir(screenshotDirectory, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const anonymous = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1600, height: 1000 }, deviceScaleFactor: 1 });
  const loginPage = await anonymous.newPage();
  await loginPage.addInitScript(() => localStorage.setItem('configra-language', 'zh'));
  await loginPage.goto(baseURL);
  await loginPage.locator('.login-layout').waitFor();
  await assertNoDocumentOverflow(loginPage, '01-login.png');
  await loginPage.screenshot({ path: path.join(screenshotDirectory, '01-login.png'), fullPage: true });
  console.log('01-login.png');
  await anonymous.close();

  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1600, height: 1000 }, deviceScaleFactor: 1 });
  await context.addInitScript(() => localStorage.setItem('configra-language', 'zh'));
  const page = await context.newPage();
  await signIn(page);
  const shot = async (name, hash, action) => {
    await page.goto(`${baseURL}/${hash ? `#/${hash}` : ''}`);
    await settle(page);
    if (action) await action(page);
    await page.evaluate(() => document.fonts.ready);
    await page.waitForTimeout(180);
    await assertNoDocumentOverflow(page, name);
    await page.screenshot({ path: path.join(screenshotDirectory, name), fullPage: true });
    console.log(name);
  };
  await shot('02-overview.png', 'overview');
  await shot('03-environments.png', 'environments');
  await shot('40-environment-detail.png', 'environments/production');
  await shot('04-configs-index.png', 'configs');
  await shot('05-config-home.png', 'configs/payment');
  await shot('06-config-current.png', 'configs/payment/production');
  await shot('07-config-history.png', 'configs/payment/production', async current => { await current.locator('.tabbar [role="tab"]').nth(2).click(); await current.locator('.history-table').waitFor(); });
  await shot('08-config-revision-v1.png', 'configs/payment/production', async current => { await current.locator('.revision-track button', { hasText: 'v1' }).click(); await current.locator('.config-code-editor.read-only').waitFor(); });
  await shot('09-config-resolved.png', 'configs/payment/production', async current => { await current.locator('.tabbar [role="tab"]').nth(1).click(); await current.locator('.reveal-panel button').click(); await current.locator('.resolved-result').waitFor(); });
  await shot('10-config-compare-empty.png', 'configs/payment/production', async current => { await current.locator('.config-operation-switch button').nth(0).click(); await current.locator('.compare-picker').waitFor(); });
  await shot('11-config-compare-result.png', 'configs/payment/production', async current => {
    await current.locator('.config-operation-switch button').nth(0).click();
    const cards = current.locator('.compare-revision-lane').first().locator('.compare-revision-card');
    await cards.nth(0).locator('button').nth(0).click();
    await cards.nth(1).locator('button').nth(1).click();
    await current.locator('.config-diff-panel').waitFor();
  });
  for (const [name, button] of [['12-config-merge-preview.png', 1], ['13-config-replace-preview.png', 2]]) {
    await shot(name, 'configs/payment/production', async current => {
      await current.locator('.config-operation-switch button').nth(button).click();
      await current.locator('.compare-revision-lane').first().locator('.compare-revision-card').first().locator('button').nth(0).click();
      await current.locator('.compare-revision-lane').nth(1).locator('.compare-revision-card').first().locator('button').nth(1).click();
      await current.locator('.transfer-result').waitFor();
    });
  }
  await shot('14-config-clone.png', 'configs/payment/production', async current => { await current.locator('.detail-more-actions summary').click(); await current.locator('.detail-more-actions button').click(); await current.locator('.clone-form').waitFor(); });
  await shot('15-vault-index.png', 'vault');
  await shot('16-vault-detail.png', 'vault/platform/mysql');
  await shot('17-vault-values.png', 'vault/platform/mysql', async current => { await current.locator('.value-gate button').click(); await current.locator('.field-current input').first().waitFor(); });
  await shot('18-vault-history.png', 'vault/platform/mysql', async current => { await current.locator('.tabbar [role="tab"]').nth(1).click(); await current.locator('.history-table').waitFor(); });
  await shot('19-vault-edit.png', 'vault/platform/mysql', async current => { await current.locator('.config-detail-heading .secondary-action').click(); await current.locator('.vault-detail-editor').waitFor(); });
  await shot('20-notifications.png', 'notifications');
  await shot('21-notification-deliveries.png', 'notifications', async current => {
    const button = current.locator('tbody tr', { hasText: 'platform-events' }).getByRole('button', { name: /查看投递|View deliveries/ });
    if (await button.count()) { await button.click(); await current.locator('.delivery-section').waitFor(); }
  });
  await shot('22-access.png', 'access');
  await shot('23-audit.png', 'audit');
  await shot('24-administration-tokens.png', 'administration');
  await shot('25-administration-certificates.png', 'administration', async current => { await current.locator('.administration-page > .tabbar [role="tab"]').nth(1).click(); await current.locator('.admin-panel').waitFor(); });
  await shot('26-administration-notifications.png', 'administration', async current => { await current.locator('.administration-page > .tabbar [role="tab"]').nth(2).click(); await current.locator('.embedded-notifications').waitFor(); });
  await shot('27-administration-deployment.png', 'administration', async current => { await current.locator('.administration-page > .tabbar [role="tab"]').nth(3).click(); await current.locator('.deployment-panel').waitFor(); });
  await shot('29-environment-create.png', 'environments', async current => { await current.locator('.resource-heading .primary-action').click(); await current.locator('.create-panel').waitFor(); });
  await shot('30-config-create.png', 'configs', async current => { await current.locator('.resource-heading .primary-action').click(); await current.locator('.config-create-form').waitFor(); });
  await shot('31-vault-create.png', 'vault', async current => { await current.locator('.resource-heading .primary-action').click(); await current.locator('.vault-structured-form').waitFor(); });
  await shot('32-notification-create.png', 'notifications', async current => { await current.locator('.resource-heading .primary-action').click(); await current.locator('.destination-form').waitFor(); });
  await shot('33-administration-token-create.png', 'administration', async current => { await current.locator('.tokens-panel .panel-actions .primary-action').click(); await current.locator('.token-form').waitFor(); });
  await shot('34-administration-token-grants.png', 'administration', async current => { await current.locator('.tokens-panel tbody tr', { hasText: 'Production edge readers' }).locator('.row-actions button').first().click(); await current.locator('.grant-form').waitFor(); });
  await shot('35-administration-certificate-import.png', 'administration', async current => { await current.locator('.administration-page > .tabbar [role="tab"]').nth(1).click(); await current.locator('.certificates-panel .panel-actions .primary-action').click(); await current.locator('.certificate-form').waitFor(); });
  await shot('36-not-found.png', 'configs/not-a-real-config');
  await page.getByRole('button', { name: /使用暗色主题|Use dark theme/ }).click();
  await shot('37-dark-overview.png', 'overview');
  await shot('38-dark-config-diff.png', 'configs/payment/production', async current => {
    await current.locator('.config-operation-switch button').nth(0).click();
    const cards = current.locator('.compare-revision-lane').first().locator('.compare-revision-card');
    await cards.nth(0).locator('button').nth(0).click();
    await cards.nth(1).locator('button').nth(1).click();
    await current.locator('.config-diff-panel').waitFor();
  });
  await shot('39-dark-vault-values.png', 'vault/platform/mysql', async current => { await current.locator('.value-gate button').click(); await current.locator('.field-current input').first().waitFor(); });
  await context.close();

  const viewer = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1600, height: 1000 }, deviceScaleFactor: 1 });
  await viewer.addInitScript(() => localStorage.setItem('configra-language', 'zh'));
  const viewerPage = await viewer.newPage();
  await signIn(viewerPage, 'viewer', 'configra-viewer');
  await viewerPage.goto(`${baseURL}/#/administration`);
  await settle(viewerPage);
  await assertNoDocumentOverflow(viewerPage, '28-viewer-forbidden.png');
  await viewerPage.screenshot({ path: path.join(screenshotDirectory, '28-viewer-forbidden.png'), fullPage: true });
  console.log('28-viewer-forbidden.png');
  await viewer.close();
  await browser.close();
}

if (!['seed', 'screenshots', 'all'].includes(command)) throw new Error('usage: node web/scripts/local-ui.mjs [seed|screenshots|all]');
if (command === 'seed' || command === 'all') await seed();
if (command === 'screenshots' || command === 'all') await screenshots();
