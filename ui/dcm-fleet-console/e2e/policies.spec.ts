import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test, expect } from './fixtures';
import { verifyScriptEnv } from './helpers/env';

const RUN_ID = `${Date.now()}`;
const DISPLAY_NAME = `E2E Policy ${RUN_ID}`;
const UPDATED_DESCRIPTION = `E2E updated ${RUN_ID}`;

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const verifyScript = path.join(scriptDir, '..', 'scripts', 'verify-policy-api.sh');

function verifyApi(...args: string[]) {
  execFileSync('bash', [verifyScript, ...args], {
    env: {
      ...verifyScriptEnv(),
      E2E_POLICY_PREFIX: 'E2E Policy',
    },
    stdio: 'inherit',
  });
}

const VALID_REGO = `package dcm.fleet.e2e

main := {
  "rejected": false
}
`;

test.describe('Policies CRUD', () => {
  let policyId: string;

  test.afterAll(() => {
    if (!policyId) return;
    try {
      verifyApi('delete', '--id', policyId);
    } catch {
      // already deleted or unreachable API
    }
  });

  test('validate form and placement strategy dropdown', async ({ page }) => {
    await page.goto('./policies');
    await page.getByTestId('policies-create-btn').click();

    await expect(page.getByTestId('policy-placement-strategy')).toBeVisible();
    const strategyTag = await page.getByTestId('policy-placement-strategy').evaluate(
      (el) => el.tagName.toLowerCase(),
    );
    expect(strategyTag).toBe('select');

    await page.getByTestId('policy-display-name').fill(`E2E validate ${RUN_ID}`);
    await page.getByTestId('policy-validate-form').click();
    await expect(page.getByTestId('policy-validate-form-success')).toBeVisible();
  });

  test('create, view, modify, delete with API verification', async ({ page }) => {
    await page.goto('./policies');

    const createResponse = page.waitForResponse(
      (r) =>
        r.url().includes('/api/v1alpha1/policies') &&
        r.request().method() === 'POST',
    );

    await page.getByTestId('policies-create-btn').click();
    await page.getByTestId('policy-display-name').fill(DISPLAY_NAME);
    await page.getByTestId('policy-description').fill('E2E initial description');
    await page.getByTestId('policy-priority').fill('50');
    await page.getByTestId('policy-rego-code').fill(VALID_REGO);
    await page.getByTestId('policy-save-btn').click();

    const res = await createResponse;
    expect(res.ok()).toBeTruthy();
    const created = await res.json();
    policyId = created.id as string;
    expect(policyId).toBeTruthy();

    verifyApi('assert-exists', '--id', policyId);
    verifyApi(
      'assert-field',
      '--id',
      policyId,
      '--jq',
      `.display_name == "${DISPLAY_NAME}"`,
    );

    await expect(page.getByTestId(`policy-row-${policyId}`)).toBeVisible();

    await page.getByTestId(`policy-view-${policyId}`).click();
    await expect(page.getByTestId('policy-view-rego')).toContainText('rejected');
    await page.getByTestId('policy-view-close').click();

    const patchResponse = page.waitForResponse(
      (r) =>
        r.url().includes(`/policies/${policyId}`) &&
        r.request().method() === 'PATCH',
    );

    await page.getByTestId(`policy-edit-${policyId}`).click();
    await page.getByTestId('policy-description').fill(UPDATED_DESCRIPTION);
    await page.getByTestId('policy-priority').fill('10');
    await page.getByTestId('policy-cluster-count').fill('2');
    await page.getByTestId('policy-save-btn').click();
    await patchResponse;

    verifyApi(
      'assert-field',
      '--id',
      policyId,
      '--jq',
      `.description == "${UPDATED_DESCRIPTION}"`,
    );
    verifyApi('assert-field', '--id', policyId, '--jq', '.priority == 10');
    verifyApi('assert-field', '--id', policyId, '--jq', '.placement.cluster_count == 2');

    const deleteResponse = page.waitForResponse(
      (r) =>
        r.url().includes(`/policies/${policyId}`) &&
        r.request().method() === 'DELETE',
    );

    await page.getByTestId(`policy-delete-${policyId}`).click();
    await page.getByTestId('policy-delete-confirm').click();
    await deleteResponse;

    await expect(page.getByTestId(`policy-row-${policyId}`)).toHaveCount(0);
    verifyApi('assert-deleted', '--id', policyId);
    policyId = '';
  });
});
