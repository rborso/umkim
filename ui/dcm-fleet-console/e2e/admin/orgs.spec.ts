import { test, expect } from '../fixtures';

test.describe('Admin orgs', () => {
  test('lists orgs page for dc-admin', async ({ page }) => {
    await page.goto('./admin/orgs');
    await expect(page.getByRole('heading', { name: 'Organizations', level: 1 })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByRole('button', { name: 'Create organization' })).toBeVisible();
  });
});
