import { test, expect } from '@playwright/test';
import { fleetConsoleBasePath } from './helpers/env';

const customerUser = process.env.FLEET_CUSTOMER_USER;
const customerPass = process.env.FLEET_CUSTOMER_PASSWORD;

test.describe('Customer multi-tenancy', () => {
  test.skip(!customerUser || !customerPass, 'Set FLEET_CUSTOMER_USER and FLEET_CUSTOMER_PASSWORD');

  test('customer nav hides policies and catalog admin', async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    await page.goto(`${fleetConsoleBasePath()}login`);
    await page.locator('#username').fill(customerUser!);
    await page.locator('#password').fill(customerPass!);
    await page.getByRole('button', { name: 'Log in' }).click();
    await page.waitForURL((url) => !url.pathname.endsWith('/login'), { timeout: 30_000 });

    await expect(page.getByRole('link', { name: 'Policies' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Create catalog item' })).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Catalog instances' })).toBeVisible();

    await context.close();
  });
});
