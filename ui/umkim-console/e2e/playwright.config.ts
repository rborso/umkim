import { defineConfig, devices } from '@playwright/test';
import { fleetConsoleBasePath, storageStatePath } from './helpers/env';

export default defineConfig({
  testDir: '.',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: 'list',
  timeout: 60_000,
  globalSetup: './global-setup.ts',
  use: {
    baseURL: fleetConsoleBasePath(),
    storageState: storageStatePath,
    trace: 'on-first-retry',
    ...devices['Desktop Chrome'],
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
