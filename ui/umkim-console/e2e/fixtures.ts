import { test as base, expect } from '@playwright/test';
import type { APIRequestContext } from '@playwright/test';
import { fleetConsoleUrl, storageStatePath } from './helpers/env';

export const test = base.extend<{
  apiRequest: APIRequestContext;
}>({
  apiRequest: async ({ playwright }, use) => {
    const ctx = await playwright.request.newContext({
      storageState: storageStatePath,
      baseURL: fleetConsoleUrl(),
    });
    await use(ctx);
    await ctx.dispose();
  },
});

export { expect };
