import { expect, test } from '@playwright/test';

test('anonymous user can switch the complete sign-in experience between English and Chinese', async ({ page }) => {
  await page.route('**/v1/me', route => route.fulfill({ status: 401, contentType: 'application/json', body: '{"error":{"code":"unauthenticated"}}' }));
  await page.goto('/ui/');

  await expect(page.getByRole('heading', { name: 'Exact configuration, for every environment.' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Continue with SSO' })).toHaveAttribute('href', '/auth/login?return_to=/');
  await expect(page.getByText('Configra never receives your password.')).toBeVisible();
  if (process.env.CONFIGRA_UI_SCREENSHOT) await page.screenshot({ path: process.env.CONFIGRA_UI_SCREENSHOT, fullPage: true });
  await page.getByRole('button', { name: /中文/ }).click();

  await expect(page.getByRole('heading', { name: '让每个 Environment 都拿到准确配置。' })).toBeVisible();
  await expect(page.getByRole('link', { name: '使用 SSO 继续' })).toBeVisible();
  await expect(page.getByText('Configra 不会接收账号密码。')).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN');
  await page.reload();
  await expect(page.getByRole('heading', { name: '让每个 Environment 都拿到准确配置。' })).toBeVisible();
});

test('theme follows the user, persists, and keeps dark surfaces legible', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await page.route('**/v1/me', route => route.fulfill({ status: 401, contentType: 'application/json', body: '{"error":{"code":"unauthenticated"}}' }));
  await page.goto('/ui/');

  await page.getByRole('button', { name: 'Use dark theme' }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect.poll(() => page.evaluate(() => localStorage.getItem('configra-theme'))).toBe('dark');

  const ratios = await page.evaluate(() => {
    const rgb = value => value.match(/[\d.]+/g).slice(0, 3).map(Number);
    const luminance = value => rgb(value).map(channel => {
      const normalized = channel / 255;
      return normalized <= .04045 ? normalized / 12.92 : ((normalized + .055) / 1.055) ** 2.4;
    }).reduce((sum, channel, index) => sum + channel * [.2126, .7152, .0722][index], 0);
    const ratio = (foreground, background) => {
      const values = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
      return (values[0] + .05) / (values[1] + .05);
    };
    const pair = (foreground, background) => ratio(getComputedStyle(document.querySelector(foreground)).color, getComputedStyle(document.querySelector(background)).backgroundColor);
    return [pair('body', 'body'), pair('.login-copy > p:not(.eyebrow)', '.login-layout'), pair('.login-code', '.login-source'), pair('.primary-action', '.primary-action')];
  });
  expect(Math.min(...ratios)).toBeGreaterThanOrEqual(4.5);

  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(page.getByRole('button', { name: 'Use light theme' })).toBeVisible();
});

test('authenticated overview shows observed health, real changes, and configuration gaps', async ({ page }) => {
  const mobile = process.env.CONFIGRA_UI_MOBILE === '1';
  if (mobile) await page.setViewportSize({ width: 390, height: 844 });
  await page.route('**/v1/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ subject: 'admin-1', email: 'alex@example.com', role: 'admin' }) }));
  await page.route('**/v1/environments', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ key: 'production', display_name: 'Production' }] }) }));
  await page.route('**/v1/configs', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
    { key: 'payment', display_name: 'Payment', updated_at: '2026-08-27T01:00:00Z', environments: [{ key: 'production', revision: 17 }] },
    { key: 'orphan', display_name: 'Orphan', updated_at: '2026-08-27T00:30:00Z', environments: [{ key: 'retired', revision: 2, archived: true }] },
  ] }) }));
  await page.route('**/v1/vault-items', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ namespace_key: 'platform', key: 'database', display_name: 'Database', revision: 8, updated_at: '2026-08-27T01:01:00Z' }] }) }));
  await page.route('**/v1/audit?limit=8', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
    { id: 'vault-change', time: '2026-08-27T01:01:00Z', action: 'vault.commit', outcome: 'success', environment: '', namespace: 'platform', resource_type: 'vault_item', resource: 'database', revision: 8 },
    { id: 'config-change', time: '2026-08-27T01:00:00Z', action: 'config.commit', outcome: 'success', environment: 'production', resource_type: 'config', resource: 'payment', revision: 17 },
    { id: 'config-change', time: '2026-08-27T01:00:00Z', action: 'config.commit', outcome: 'success', environment: 'production', resource_type: 'config', resource: 'payment', revision: 17 },
  ] }) }));
  await page.route('**/v1/access?limit=1', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [
    { time: '2026-08-27T01:02:00Z', environment: 'production', resource_type: 'config', resource: 'payment' },
  ] }) }));
  await page.route('**/health/ready', route => route.fulfill({ status: 204 }));
  await page.goto('/ui/');

  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
  await expect(page.getByText('Configra / V1')).toHaveCount(0);
  await expect(page.getByText('Latest revision rail')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'System status' })).toBeVisible();
  await expect(page.getByText('Latest client read')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Recent changes' })).toBeVisible();
  const recentChanges = page.getByLabel('Recent changes');
  await expect(recentChanges.getByText('production/payment', { exact: true })).toBeVisible();
  await expect(recentChanges.getByText('platform.database', { exact: true })).toBeVisible();
  await expect(recentChanges.getByRole('row', { name: /production\/payment/ })).toHaveCount(1);
  await expect(recentChanges.getByText('v17')).toBeVisible();
  await expect(recentChanges.getByText('v8')).toBeVisible();
  await expect(page.getByText('orphan', { exact: true })).toBeVisible();
  await expect(page.getByText('No active Environment Revision')).toBeVisible();
  if (mobile) {
    await expect(page.getByText('alex@example.com')).toBeHidden();
    const layout = await page.evaluate(() => ({ documentWidth: document.documentElement.scrollWidth, viewportWidth: window.innerWidth }));
    expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth + 1);
  }
  else await expect(page.getByText('alex@example.com')).toBeVisible();
  const logout = page.locator('form[action="/auth/logout"]');
  await expect(logout).toHaveAttribute('method', 'post');
  await expect(logout.getByRole('button', { name: 'Sign out' })).toBeVisible();
  await expect(page.locator('.topbar-actions > a.icon-action')).toHaveCount(0);
  await expect(page.locator('.topbar-actions .theme-button')).toBeVisible();
  if (process.env.CONFIGRA_UI_SCREENSHOT) {
    await page.screenshot({ path: process.env.CONFIGRA_UI_SCREENSHOT, fullPage: true });
  }
});

test('authenticated workspace follows the selected Chinese locale', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('configra-language', 'zh'));
  await page.route('**/v1/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ subject: 'viewer-1', email: 'viewer@example.com', role: 'viewer' }) }));
  await page.route('**/v1/environments', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/v1/configs', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/v1/vault-items', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":[]}' }));
  await page.goto('/ui/');

  await expect(page.getByRole('heading', { name: '概览' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '系统状态' })).toBeVisible();
  await expect(page.getByText('MySQL', { exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: '最近变更' })).toBeVisible();
  await expect(page.getByText('CONFIGRA / V1')).toHaveCount(0);
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN');
});

test('service failure gives a specific retry action without exposing response details', async ({ page }) => {
  await page.route('**/v1/me', route => route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":{"code":"session-secret-sentinel"}}' }));
  await page.goto('/ui/');

  await expect(page.getByRole('heading', { name: 'Configra could not load this workspace.' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
  await expect(page.getByText('session-secret-sentinel')).toHaveCount(0);
});

test('viewer navigation and direct routes never request administrator-only resources', async ({ page }) => {
  let privilegedRequests = 0;
  await page.route('**/v1/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ subject: 'viewer-1', email: 'viewer@example.com', role: 'viewer' }) }));
  await page.route(/\/v1\/(api-tokens|client-certificates|notification-destinations)/, route => {
    privilegedRequests += 1;
    return route.fulfill({ status: 403, contentType: 'application/json', body: '{"error":{"code":"forbidden"}}' });
  });
  await page.goto('/ui/#/administration');

  await expect(page.getByRole('link', { name: 'Administration' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Notifications' })).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Administrator access required' })).toBeVisible();
  expect(privilegedRequests).toBe(0);

  await page.goto('/ui/#/notifications');
  await expect(page.getByRole('heading', { name: 'Administrator access required' })).toBeVisible();
  expect(privilegedRequests).toBe(0);
});
