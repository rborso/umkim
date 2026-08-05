import { test, expect } from '../fixtures';

test.describe('Admin LLM API keys', () => {
  test('loads LLM API keys page', async ({ page }) => {
    test.skip(!process.env.FLEET_ORG_ADMIN_USER, 'requires org-admin session (not default global setup)');
    await page.goto('./admin/llm-api-keys');
    await expect(page.getByRole('heading', { name: /LLM API keys/i })).toBeVisible({
      timeout: 30_000,
    });
  });
});
