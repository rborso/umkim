import { test, expect } from '../fixtures';

test.describe('Cluster detail read-only', () => {
  test('opens first cluster detail and shows providers panel', async ({ page, apiRequest }) => {
    const res = await apiRequest.get('/fleet/api/v1alpha1/clusters');
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    const clusters = (body.clusters || []) as { name: string }[];
    test.skip(clusters.length === 0, 'no clusters registered');
    const name = clusters[0].name;
    await page.goto(`./clusters/${encodeURIComponent(name)}`);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText('Providers', { exact: true }).first()).toBeVisible();
  });
});
