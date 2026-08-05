import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test, expect } from '../fixtures';
import { verifyScriptEnv } from '../helpers/env';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const verifyScript = path.join(scriptDir, '..', '..', 'scripts', 'verify-policy-api.sh');

test.describe('@parity Policy CRUD', () => {
  test('policy form validate and CRUD via console', async ({ page }) => {
    await page.goto('./policies');
    await page.getByRole('button', { name: 'Create policy' }).click();
    await page.getByTestId('policy-display-name').fill('');
    await page.getByTestId('policy-create-submit').click();
    await expect(page.getByText('Display name is required')).toBeVisible();

    const name = `E2E Parity Policy ${Date.now()}`;
    await page.getByTestId('policy-display-name').fill(name);
    await page.getByTestId('policy-create-submit').click();
    await expect(page.getByRole('cell', { name })).toBeVisible({ timeout: 30_000 });

    execFileSync('bash', [verifyScript, 'assert-exists', '--display-name', name], {
      env: verifyScriptEnv(),
      stdio: 'inherit',
    });
  });
});
