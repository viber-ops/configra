import { expect, test } from '@playwright/test';
import { inventoryPage } from './inventory.js';

const principal = { issuer: 'https://id.example.com', subject: 'admin-1', email: 'admin@example.com', role: 'admin' };

async function workspace(page, { me = principal, onMe } = {}) {
  const state = { meCalls: 0, signedIn: true, identity: me };
  await page.route('**/v1/me', route => {
    state.meCalls += 1;
    onMe?.(state);
    return state.signedIn
      ? route.fulfill({ json: state.identity })
      : route.fulfill({ status: 401, json: { error: { code: 'unauthenticated' } } });
  });
  await page.route(/\/v1\/(configs|vault-items)(\?|$)/, route => route.fulfill({ body: inventoryPage(route, []) }));
  await page.route(/\/v1\/environments\?/, route => route.fulfill({ body: inventoryPage(route, [{ key: 'production', display_name: 'Production' }]) }));
  await page.route('**/v1/audit*', route => route.fulfill({ json: { items: [] } }));
  await page.route('**/v1/api-tokens*', route => route.fulfill({ body: inventoryPage(route, []) }));
  await page.route('**/health/ready', route => route.fulfill({ status: 204 }));
  return state;
}

test('an ended session keeps the page and recovers after signing in from another tab', async ({ page, context }) => {
  const state = await workspace(page);
  let saves = 0;
  await page.route('**/v1/environments/production/configs/payment', async route => {
    if (route.request().method() !== 'PUT') return route.fallback();
    saves += 1;
    if (!state.signedIn) return route.fulfill({ status: 401, json: { error: { code: 'unauthenticated', request_id: 'req-expired' } } });
    return route.fulfill({ json: { outcome: 'success', revision: 1 } });
  });
  await page.goto('/ui/#/configs');
  await page.getByRole('button', { name: 'New config' }).click();
  await page.getByRole('combobox', { name: 'Environment', exact: true }).selectOption('production');
  await page.getByLabel('Resource key').fill('payment');
  await page.getByLabel('Display name').fill('Payment service');
  await page.getByLabel('Configuration source').fill('port: 6379\n');

  state.signedIn = false;
  await page.getByRole('button', { name: 'Create config' }).click();
  const banner = page.locator('.session-banner');
  await expect(banner).toContainText('Your session has ended');
  await expect(page.locator('.config-create-form [role=alert]')).toContainText('Sign in again, then retry; nothing was saved.');
  await expect(page.getByLabel('Resource key')).toHaveValue('payment');
  // The banner stays in view while the user is further down the page.
  await page.getByRole('button', { name: 'Create config' }).scrollIntoViewIfNeeded();
  await expect(banner).toBeInViewport();

  const popup = page.waitForEvent('popup');
  await banner.getByRole('button', { name: 'Sign in in a new tab' }).click();
  const signIn = await popup;
  expect(new URL(signIn.url()).pathname).toBe('/auth/login');
  expect(new URL(signIn.url()).searchParams.get('return_to')).toBe('/ui/?signed-in=1');
  await signIn.close();

  // The sign-in tab lands on /ui/?signed-in=1, which tells this tab to re-check its identity.
  state.signedIn = true;
  const returned = await context.newPage();
  await returned.goto('/ui/?signed-in=1');
  await expect(returned.getByText('You are signed in. Return to your Configra tab')).toBeVisible();
  await expect(banner).toHaveCount(0);
  await page.getByRole('button', { name: 'Create config' }).click();
  await expect(page.locator('.config-create-form')).toHaveCount(0);
  expect(saves).toBe(2);
});

test('a different identity after signing in again reloads the page', async ({ page, context }) => {
  const state = await workspace(page);
  await page.goto('/ui/#/configs');
  await expect(page.getByRole('heading', { name: 'Configs' })).toBeVisible();
  await page.evaluate(() => window.dispatchEvent(new Event('configra:session-ended')));
  await expect(page.locator('.session-banner')).toBeVisible();
  state.identity = { ...principal, subject: 'viewer-9', role: 'viewer' };
  const reloaded = page.waitForEvent('load');
  const returned = await context.newPage();
  await returned.goto('/ui/?signed-in=1');
  await reloaded;
  await expect(page.locator('.session-banner')).toHaveCount(0);
});

test('the idle timeout is announced and can be extended deliberately', async ({ page }) => {
  await page.clock.install({ time: new Date('2026-11-05T09:00:00Z') });
  const state = await workspace(page, { me: { ...principal, session_idle_timeout_seconds: 300, session_lifetime_seconds: 28800, session_expires_at: '2026-11-05T17:00:00Z' } });
  await page.goto('/ui/#/configs');
  // Let the page's own requests finish so they cannot land during the clock jump.
  await expect(page.getByText('No managed resources yet.')).toBeVisible();
  await page.waitForLoadState('networkidle');
  await expect(page.locator('.session-banner')).toHaveCount(0);
  await page.clock.fastForward('03:30');
  await expect(page.locator('.session-banner')).toContainText('signed out in about two minutes');
  const before = state.meCalls;
  await page.getByRole('button', { name: 'Stay signed in' }).click();
  await expect(page.locator('.session-banner')).toHaveCount(0);
  expect(state.meCalls).toBe(before + 1);
  await page.clock.fastForward('05:30');
  await expect(page.locator('.session-banner')).toContainText('Your session has ended');
});

test('the end of the session lifetime is announced in advance', async ({ page }) => {
  await page.clock.install({ time: new Date('2026-11-05T16:55:00Z') });
  await workspace(page, { me: { ...principal, session_idle_timeout_seconds: 1800, session_lifetime_seconds: 28800, session_expires_at: '2026-11-05T17:00:00Z' } });
  await page.goto('/ui/#/configs');
  await expect(page.locator('.session-banner')).toContainText('Your session ends at');
  await expect(page.locator('.session-banner').getByRole('button', { name: 'Sign in in a new tab' })).toBeVisible();
});

test('signing in returns to the page that asked for it', async ({ page }) => {
  const state = await workspace(page);
  state.signedIn = false;
  await page.goto('/ui/#/configs?q=payment');
  await page.route(/\/auth\/login/, route => route.fulfill({ status: 200, contentType: 'text/html', body: '<p>identity provider</p>' }));
  await page.getByRole('link', { name: /Continue with SSO/ }).click();
  await expect(page.getByText('identity provider')).toBeVisible();
  state.signedIn = true;
  await page.goto('/ui/');
  await expect.poll(() => page.evaluate(() => window.location.hash)).toBe('#/configs?q=payment');
});

test('Deployment status shows the configured session limits', async ({ page }) => {
  await workspace(page, { me: { ...principal, session_idle_timeout_seconds: 1800, session_lifetime_seconds: 259200, session_expires_at: '2026-11-08T09:00:00Z' } });
  await page.goto('/ui/#/administration');
  await page.getByRole('tab', { name: 'Deployment status' }).click();
  await expect(page.getByText('30 min idle · 72 h maximum')).toBeVisible();
});
