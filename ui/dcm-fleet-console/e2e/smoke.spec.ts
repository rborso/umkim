import { test, expect } from './fixtures';

const navRoutes = [
  { path: './', heading: 'Overview' },
  { path: './sites', heading: 'Sites' },
  { path: './infrastructure', heading: 'Infrastructure' },
  { path: './clusters', heading: 'Clusters' },
  { path: './apps', heading: 'Apps' },
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

  test('overview shows site and cluster health', async ({ page }) => {
    await page.goto('./');
    await expect(page.getByText('Sites', { exact: true }).first()).toBeVisible();
    await expect(page.getByText('Clusters', { exact: true }).first()).toBeVisible();
  });
});
