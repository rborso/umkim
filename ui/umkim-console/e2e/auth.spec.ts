import { test, expect } from './fixtures';
import { authCredentials, fleetConsoleBasePath } from './helpers/env';

test.describe('Authentication', () => {
  test('shows authenticated user in masthead', async ({ page }) => {
    const { user } = authCredentials();
    await page.goto('./');
    await expect(page.getByText('UMKIM fleet console')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Log out' })).toBeVisible();
    await expect(page.getByText(user, { exact: false })).toBeVisible();
  });

  test('login form redirects to overview', async ({ browser }) => {
    const { user, pass } = authCredentials();
    const context = await browser.newContext();
    const page = await context.newPage();

    await page.goto(`${fleetConsoleBasePath()}login`);
    await page.locator('#username').fill(user);
    await page.locator('#password').fill(pass);
    await page.getByRole('button', { name: 'Log in' }).click();
    await page.waitForURL((url) => !url.pathname.endsWith('/login'), { timeout: 30_000 });
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();

    await context.close();
  });

  test('logout returns to login', async ({ page }) => {
    await page.goto('./');
    await page.getByRole('button', { name: 'Log out' }).click();
    await page.waitForURL((url) => url.pathname.endsWith('/login'), { timeout: 15_000 });
    await expect(page.getByRole('button', { name: 'Log in' })).toBeVisible();
  });
});
