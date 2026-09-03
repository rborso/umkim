import { test, expect } from '../fixtures';

test.describe('Cluster detail read-only', () => {
  test('opens first cluster detail with tabbed layout', async ({ page, apiRequest }) => {
    const res = await apiRequest.get('/fleet/api/v1alpha1/clusters');
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    const clusters = (body.clusters || []) as { name: string }[];
    test.skip(clusters.length === 0, 'no clusters registered');
    const name = clusters[0].name;
    await page.goto(`./clusters/${encodeURIComponent(name)}`);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({ timeout: 30_000 });
    await expect(page.getByTestId('cluster-detail-tabs')).toBeVisible();
    await expect(page.getByTestId('cluster-tab-overview')).toBeVisible();
    await expect(page.getByText('Providers', { exact: true }).first()).toBeVisible();

    await page.getByTestId('cluster-tab-edge').click();
    await expect(page.getByRole('heading', { name: 'Site and topology' })).toBeVisible();

    await page.getByTestId('cluster-tab-storage').click();
    await expect(page.getByText('Storage', { exact: true }).first()).toBeVisible();

    await page.getByTestId('cluster-tab-nodes').click();
    await expect(page.getByRole('columnheader', { name: 'Node' })).toBeVisible();
  });
});
