import { test, expect } from '../fixtures';
import { requireCatalogItem } from '../helpers/catalog';

const WHOAMI = 'whoami-helm';
const RUN_ID = process.env.FV_RUN_ID || `${Date.now()}`;
const RELEASE = `verify-synth-${RUN_ID}-whoami`;

test.describe('@parity Deploy submit', () => {
  test.beforeEach(async ({ apiRequest }) => {
    const ok = await requireCatalogItem(apiRequest, WHOAMI);
    if (!ok) test.skip(true, `missing catalog ${WHOAMI}`);
  });

  test('submits whoami-helm deploy wizard', async ({ page, apiRequest }) => {
    await page.goto(`./instances?catalogItem=${WHOAMI}&deploy=1`);
    await expect(page.getByTestId('deploy-display-name')).toBeVisible();
    await page.getByTestId('deploy-display-name').fill(RELEASE);

    await page.getByTestId('deploy-wizard-next').click();
    await page.getByTestId('deploy-wizard-next').click();
    await page.getByTestId('deploy-wizard-next').click();

    const createRes = page.waitForResponse(
      (r) => r.url().includes('/catalog-item-instances') && r.request().method() === 'POST',
    );
    await page.getByTestId('instance-deploy-submit').click();
    expect((await createRes).ok()).toBeTruthy();

    const list = await apiRequest.get('/api/v1alpha1/catalog-item-instances');
    expect(list.ok()).toBeTruthy();
    const body = await list.json();
    const found = (body.results || []).some(
      (r: { display_name?: string }) => r.display_name === RELEASE,
    );
    expect(found).toBeTruthy();

    const inst = (body.results || []).find(
      (r: { display_name?: string; id?: string; uid?: string }) => r.display_name === RELEASE,
    );
    const id = inst?.id || inst?.uid;
    if (id) {
      await apiRequest.delete(`/api/v1alpha1/catalog-item-instances/${id}`);
    }
  });
});
