import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test, expect } from './fixtures';
import { dcmApiUrl, verifyScriptEnv } from './helpers/env';

const RUN_ID = `${Date.now()}`;
const DISPLAY_NAME = `E2E Operator ${RUN_ID}`;

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const verifyScript = path.join(scriptDir, '..', 'scripts', 'verify-operator-instance-api.sh');

function verifyApi(...args: string[]) {
  execFileSync('bash', [verifyScript, ...args], {
    env: verifyScriptEnv(),
    stdio: 'inherit',
  });
}

test.describe('Catalog instances (operators)', () => {
  let instanceId: string;

  test.afterAll(() => {
    if (!instanceId) return;
    try {
      verifyApi('delete', '--id', instanceId);
    } catch {
      // cleanup best-effort
    }
  });

  test('navigate to Catalog instances', async ({ page }) => {
    await page.goto('./instances');
    await expect(page.getByRole('heading', { name: 'Catalog instances' })).toBeVisible();
    await expect(page.getByTestId('instances-deploy-toolbar')).toBeVisible();
  });

  test('create operator instance via API and delete from UI when catalog exists', async ({
    page,
    apiRequest,
  }) => {
    const catalogItem = process.env.E2E_OPERATOR_CATALOG || 'cloudnative-pg-operator';

    const catalogRes = await apiRequest.get(`${dcmApiUrl()}/catalog-items/${catalogItem}`);
    if (!catalogRes.ok()) {
      test.skip(true, `Catalog item missing: ${catalogItem}`);
      return;
    }

    const createRes = await apiRequest.post(`${dcmApiUrl()}/catalog-item-instances`, {
      data: {
        api_version: 'v1alpha1',
        display_name: DISPLAY_NAME,
        spec: {
          catalog_item_id: catalogItem,
          user_values: [
            { path: 'metadata.name', value: `e2e-cnpg-${RUN_ID}` },
            { path: 'operatorGroup.targetNamespace', value: 'operators' },
          ],
        },
      },
    });

    if (!createRes.ok()) {
      test.skip(true, `DCM API unavailable or catalog missing: HTTP ${createRes.status()}`);
      return;
    }

    const created = await createRes.json();
    instanceId = (created.id || created.resource_id) as string;
    expect(instanceId).toBeTruthy();
    verifyApi('assert-exists', '--id', instanceId);

    await page.goto('./instances');
    await expect(page.getByText(DISPLAY_NAME)).toBeVisible({ timeout: 15000 });

    const row = page.getByRole('row', { name: new RegExp(DISPLAY_NAME) });
    await row.getByRole('button', { name: 'Delete instance' }).click();
    await page.getByRole('button', { name: 'Delete' }).click();

    await expect(page.getByText(DISPLAY_NAME)).toHaveCount(0, { timeout: 30000 });
    verifyApi('assert-deleted', '--id', instanceId);
    instanceId = '';
  });
});
