import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import { request, type FullConfig } from '@playwright/test';
import {
  authCredentials,
  authDir,
  authEnvPath,
  fleetConsoleUrl,
  storageStatePath,
} from './helpers/env';

export default async function globalSetup(_config: FullConfig) {
  const base = fleetConsoleUrl();
  const { user, pass } = authCredentials();
  const cookieJar = `${authDir}/cookies.txt`;

  fs.mkdirSync(authDir, { recursive: true });
  fs.rmSync(cookieJar, { force: true });

  const loginBody = JSON.stringify({ username: user, password: pass });
  execFileSync(
    'curl',
    [
      '-s',
      '-c',
      cookieJar,
      '-X',
      'POST',
      `${base}/fleet/auth/v1alpha1/login`,
      '-H',
      'Content-Type: application/json',
      '-d',
      loginBody,
    ],
    { stdio: 'pipe' },
  );

  const meCode = execFileSync(
    'curl',
    ['-s', '-o', '/dev/null', '-w', '%{http_code}', '-b', cookieJar, `${base}/fleet/auth/v1alpha1/me`],
    { encoding: 'utf8' },
  ).trim();
  if (meCode !== '200') {
    throw new Error(`Auth global setup failed: GET /me returned HTTP ${meCode}`);
  }

  const apiContext = await request.newContext({ baseURL: base });
  const loginRes = await apiContext.post('/fleet/auth/v1alpha1/login', {
    data: { username: user, password: pass },
  });
  if (!loginRes.ok()) {
    throw new Error(`Playwright login failed: HTTP ${loginRes.status()} ${await loginRes.text()}`);
  }
  const meRes = await apiContext.get('/fleet/auth/v1alpha1/me');
  if (!meRes.ok()) {
    throw new Error(`Playwright GET /me failed: HTTP ${meRes.status()}`);
  }
  await apiContext.storageState({ path: storageStatePath });
  await apiContext.dispose();

  fs.writeFileSync(
    authEnvPath,
    JSON.stringify({ cookieJar, user, baseUrl: base }, null, 2),
  );
}
