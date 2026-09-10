import { test, expect } from './fixtures';
import { fleetApiUrl } from './helpers/env';

function managementClusterCandidates(): string[] {
  const fromEnv = process.env.E2E_MANAGEMENT_CLUSTER;
  const defaults = ['umkim-hub', 'umkim-hub'];
  return [...new Set([fromEnv, ...defaults].filter((v): v is string => !!v))];
}

test.describe('Clusters', () => {
  test('list page loads with at least one cluster', async ({ page, apiRequest }) => {
    const res = await apiRequest.get(`${fleetApiUrl()}/clusters`);
    expect(res.ok()).toBeTruthy();
    const body = (await res.json()) as {
      clusters?: Array<{ name?: string; metadata?: { name?: string } }>;
    };
    const clusters = body.clusters ?? [];
    expect(clusters.length).toBeGreaterThanOrEqual(1);

    await page.goto('./clusters');
    await expect(page.getByRole('heading', { name: 'Clusters', level: 1 })).toBeVisible();
    await expect(page.getByRole('alert', { name: 'Request failed' })).toHaveCount(0);

    const names = clusters
      .map((c) => c.name || c.metadata?.name)
      .filter((n): n is string => !!n);
    const visible =
      managementClusterCandidates().find((name) => names.includes(name)) ?? names[0];
    await expect(page.getByRole('link', { name: visible })).toBeVisible();
  });
});
