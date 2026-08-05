import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test, expect } from './fixtures';
import { verifyScriptEnv } from './helpers/env';

const RUN_ID = `${Date.now()}`;
const SITE_NAME = `e2e-site-${RUN_ID}`;
const DISPLAY_NAME = `E2E Site ${RUN_ID}`;

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const verifyScript = path.join(scriptDir, '..', 'scripts', 'verify-fleet-api.sh');

function verifyFleetApi(...args: string[]) {
  execFileSync('bash', [verifyScript, ...args], {
    env: {
      ...verifyScriptEnv(),
      E2E_SITE_PREFIX: 'E2E Site',
    },
    stdio: 'inherit',
  });
}

test.describe('Sites CRUD', () => {
  test.afterAll(() => {
    try {
      verifyFleetApi('delete', '--name', SITE_NAME);
    } catch {
      // cleanup best-effort
    }
  });

  test('create registry site and delete with API verification', async ({ page }) => {
    await page.goto('./sites');
    await page.getByRole('button', { name: 'Create site' }).click();

    await page.locator('#siteName').fill(SITE_NAME);
    await page.locator('#displayName').fill(DISPLAY_NAME);
    await page.locator('#edgeDomain').fill(`e2e-${RUN_ID}.example.com`);
    await page.locator('#wildcard').fill(`*.e2e-${RUN_ID}.example.com`);
    await page.locator('#dnsNote').fill('Playwright E2E');

    const createResponse = page.waitForResponse(
      (r) => r.url().includes('/fleet/api/v1alpha1/sites') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: 'Create', exact: true }).click();
    const res = await createResponse;
    expect(res.ok()).toBeTruthy();

    verifyFleetApi('assert-exists', '--name', SITE_NAME);
    verifyFleetApi('assert-exists', '--display-name', DISPLAY_NAME);

    await expect(page.getByRole('link', { name: DISPLAY_NAME })).toBeVisible();
    await expect(page.getByTestId(`site-delete-${SITE_NAME}`)).toBeVisible();

    await page.getByTestId(`site-delete-${SITE_NAME}`).click();
    await page.getByTestId('site-delete-confirm-name').fill(SITE_NAME);
    const deleteResponse = page.waitForResponse(
      (r) =>
        r.url().includes(`/fleet/api/v1alpha1/sites/${SITE_NAME}`) &&
        r.request().method() === 'DELETE',
    );
    await page.getByTestId('site-delete-confirm').click();
    await deleteResponse;

    await expect(page.getByTestId(`site-delete-${SITE_NAME}`)).toHaveCount(0);
    verifyFleetApi('assert-deleted', '--name', SITE_NAME);
  });
});
