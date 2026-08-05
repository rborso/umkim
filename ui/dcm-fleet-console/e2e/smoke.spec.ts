import { test, expect } from './fixtures';
import { dcmApiUrl } from './helpers/env';

const navRoutes = [
  { path: './', heading: 'Overview' },
  { path: './sites', heading: 'Sites' },
  { path: './infrastructure', heading: 'Infrastructure' },
  { path: './clusters', heading: 'Clusters' },
  { path: './providers', heading: 'Service providers' },
  { path: './catalog', heading: 'Catalog' },
  { path: './instances', heading: 'Catalog instances' },
  { path: './policies', heading: 'Policies' },
] as const;

test.describe('Smoke navigation', () => {
  for (const route of navRoutes) {
    test(`loads ${route.heading}`, async ({ page }) => {
      await page.goto(route.path);
      await expect(page.getByRole('heading', { name: route.heading, level: 1 })).toBeVisible({
        timeout: 30_000,
      });
      await expect(page.getByRole('alert', { name: 'Request failed' })).toHaveCount(0);
    });
  }

  test('overview shows health cards', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByText('Providers', { exact: true }).first()).toBeVisible();
    await expect(page.getByText('Catalog', { exact: true }).first()).toBeVisible();
    await expect(page.getByText('Policies', { exact: true }).first()).toBeVisible();
  });

  test('DCM health endpoints respond', async ({ apiRequest }) => {
    for (const path of ['providers', 'catalog', 'policies', 'placement']) {
      const res = await apiRequest.get(`${dcmApiUrl()}/health/${path}`);
      expect(res.ok(), `health/${path} should return 200`).toBeTruthy();
    }
  });
});
