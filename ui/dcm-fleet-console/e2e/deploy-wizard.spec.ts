import { test, expect } from './fixtures';
import { requireCatalogItem } from './helpers/catalog';
import { dcmApiUrl } from './helpers/env';

const DEMO_CATALOG_ID = 'demo-webapp';

test.describe('Deploy wizard', () => {
  test.beforeEach(async ({ apiRequest }) => {
    const ok = await requireCatalogItem(apiRequest, DEMO_CATALOG_ID);
    if (!ok) {
      test.skip(true, `Catalog item missing: ${DEMO_CATALOG_ID}`);
    }
  });

  test('shows named steps and configure fields from catalog', async ({ page }) => {
    await page.goto(`./instances?catalogItem=${DEMO_CATALOG_ID}&deploy=1`);

    const stepper = page.getByTestId('deploy-wizard-stepper');
    await expect(stepper).toBeVisible();
    await expect(stepper).toContainText('Template');
    await expect(stepper).toContainText('Configure');
    await expect(stepper).toContainText('Placement');
    await expect(stepper).toContainText('Review');

    await expect(page.getByTestId('deploy-display-name')).toBeVisible();
    await expect(page.getByTestId('deploy-tier-required')).toBeVisible();
    await expect(page.getByTestId('deploy-tier-common')).toBeVisible();
    await expect(page.getByText('Container image')).toBeVisible();

    await page.getByTestId('deploy-wizard-next').click();
    await expect(page.getByTestId('deploy-label-selector')).toBeVisible();
    await expect(page.getByTestId('deploy-site-select')).toHaveCount(0);
    await expect(page.getByTestId('placement-impact-panel')).toBeVisible();

    await page.getByTestId('deploy-wizard-next').click();
    await expect(page.getByTestId('deploy-review-values')).toBeVisible();
    await expect(page.getByTestId('instance-deploy-submit')).toBeVisible();
  });

  test('blocks next when required catalog field is cleared', async ({ page }) => {
    await page.goto(`./instances?catalogItem=${DEMO_CATALOG_ID}&deploy=1`);

    await expect(page.getByTestId('deploy-display-name')).toBeVisible();

    const nameInput = page.locator('#dep-required-metadata-name');
    await nameInput.fill('');

    await page.getByTestId('deploy-wizard-next').click();
    await expect(page.getByTestId('deploy-label-selector')).not.toBeVisible();
    await expect(page.getByTestId('deploy-tier-required')).toBeVisible();
  });

  test('toolbar deploy opens wizard at template step', async ({ page }) => {
    await page.goto('./instances');
    await page.getByTestId('instances-deploy-toolbar').click();

    const stepper = page.getByTestId('deploy-wizard-stepper');
    await expect(stepper).toBeVisible();
    await expect(stepper.getByText('Template', { exact: true })).toBeVisible();
    await expect(page.getByTestId('deploy-catalog-select')).toBeVisible();
  });

  test('catalog API lists demo-webapp', async ({ apiRequest }) => {
    const res = await apiRequest.get(`${dcmApiUrl()}/catalog-items/${DEMO_CATALOG_ID}`);
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    const path = (body.path || body.id || body.catalog_item_id || '') as string;
    expect(path).toContain('demo-webapp');
  });

  test('advanced CPU fields support millicore unit selector', async ({ page }) => {
    await page.goto(`./instances?catalogItem=${DEMO_CATALOG_ID}&deploy=1`);
    await expect(page.getByTestId('deploy-display-name')).toBeVisible();

    const advanced = page.getByTestId('deploy-tier-advanced');
    if (await advanced.isVisible()) {
      await advanced.click();
    }

    const cpuMinUnit = page.locator('[id$="resources-cpu-min-unit"]');
    const hasCpuWidget = (await cpuMinUnit.count()) > 0;
    if (!hasCpuWidget) {
      test.skip(true, 'Catalog CPU fields not yet seeded with cpu-quantity widget');
      return;
    }

    await expect(cpuMinUnit).toBeVisible();
    await cpuMinUnit.selectOption('millicores');
    const cpuInput = page.locator('[name$="resources-cpu-min"]');
    await cpuInput.fill('250');
    await expect(cpuInput).toHaveValue('250');

    await page.getByTestId('deploy-wizard-next').click();
    await expect(page.getByTestId('deploy-label-selector')).toBeVisible();
  });
});
