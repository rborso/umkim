import { test, expect } from '@playwright/test';
import { fleetConsoleBasePath } from '../helpers/env';

test.describe('Viewer role', () => {
  test.use({
    storageState: { cookies: [], origins: [] },
  });

  test('can list clusters but cannot create', async ({ page, request }) => {
    const user = process.env.FLEET_VIEWER_USER;
    const pass = process.env.FLEET_VIEWER_PASSWORD;
    test.skip(!user || !pass, 'requires FLEET_VIEWER_USER and FLEET_VIEWER_PASSWORD');

    const base = fleetConsoleBasePath().replace(/\/$/, '');
    const loginRes = await request.post(`${base}/auth/v1alpha1/login`, {
      data: { username: user, password: pass },
    });
    expect(loginRes.ok()).toBeTruthy();

    await page.goto('./clusters');
    await expect(page.getByRole('heading', { name: 'Clusters', level: 1 })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByRole('button', { name: /Create cluster/i })).toBeDisabled();
  });
});
