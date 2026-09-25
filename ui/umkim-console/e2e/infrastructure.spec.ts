import { test, expect } from './fixtures';
import { fleetApiUrl } from './helpers/env';

test.describe('Infrastructure', () => {
  test('hub infrastructure page and API', async ({ page, apiRequest }) => {
    const res = await apiRequest.get(`${fleetApiUrl()}/infrastructure/hub`);
    expect(res.ok()).toBeTruthy();
    const hub = await res.json();
    expect(hub.host_name || hub.management_site_id).toBeTruthy();

    await page.goto('./infrastructure');
    await expect(page.getByRole('heading', { name: 'Infrastructure', level: 1 })).toBeVisible();
    await expect(page.getByRole('alert', { name: 'Request failed' })).toHaveCount(0);

    await expect(page.getByText('Host', { exact: true })).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText('Management site', { exact: true })).toBeVisible();

    const mgmtSite = hub.management_site_id as string | undefined;
    if (mgmtSite) {
      await expect(page.getByRole('link', { name: mgmtSite, exact: true })).toBeVisible();
    }

    const servicesOrNodes =
      (hub.services as unknown[] | undefined)?.length ||
      (hub.nodes as unknown[] | undefined)?.length;
    if (servicesOrNodes) {
      const section = (hub.nodes as unknown[] | undefined)?.length ? 'Hub nodes' : 'Services';
      await expect(page.getByRole('heading', { name: section })).toBeVisible();
    }
  });
});
