import { test, expect } from '../fixtures';

const RUN_ID = process.env.FV_RUN_ID || `${Date.now()}`;
const SITE_NAME = `verify-synth-${RUN_ID}-parity-site`;

test.describe('@parity Site create', () => {
  test.afterAll(async ({ apiRequest }) => {
    await apiRequest.delete(`/fleet/api/v1alpha1/sites/${SITE_NAME}`).catch(() => undefined);
  });

  test('creates registry site via console', async ({ page }) => {
    await page.goto('./sites');
    await page.getByRole('button', { name: 'Create site' }).click();
    await page.locator('#siteName').fill(SITE_NAME);
    await page.locator('#displayName').fill(`Parity ${RUN_ID}`);
    const createResponse = page.waitForResponse(
      (r) => r.url().includes('/fleet/api/v1alpha1/sites') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: 'Create', exact: true }).click();
    expect((await createResponse).ok()).toBeTruthy();
    await expect(page.getByRole('link', { name: `Parity ${RUN_ID}` })).toBeVisible({ timeout: 30_000 });
  });
});
