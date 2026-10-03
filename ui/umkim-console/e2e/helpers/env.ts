import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const e2eDir = path.dirname(fileURLToPath(import.meta.url));
export const authDir = path.join(e2eDir, '..', '.auth');
export const storageStatePath = path.join(authDir, 'storageState.json');
export const authEnvPath = path.join(authDir, 'env.json');

export function fleetConsoleUrl(): string {
  const raw = process.env.FLEET_CONSOLE_URL || 'http://localhost:5173';
  return raw.replace(/\/$/, '').replace(/\/fleet\/?$/, '');
}

export function fleetConsoleBasePath(): string {
  return `${fleetConsoleUrl()}/fleet/`;
}

export function fleetApiUrl(): string {
  return process.env.FLEET_API_URL || `${fleetConsoleUrl()}/fleet/api/v1alpha1`;
}

export function authCredentials(): { user: string; pass: string } {
  return {
    user: process.env.FLEET_AUTH_USER || 'localadmin',
    pass: process.env.FLEET_AUTH_PASSWORD || 'CHANGE_ME',
  };
}

export function authCookieJar(): string | undefined {
  if (process.env.FLEET_AUTH_COOKIE_JAR) {
    return process.env.FLEET_AUTH_COOKIE_JAR;
  }
  try {
    const data = JSON.parse(fs.readFileSync(authEnvPath, 'utf8')) as { cookieJar?: string };
    return data.cookieJar;
  } catch {
    return undefined;
  }
}

export function verifyScriptEnv(): NodeJS.ProcessEnv {
  const jar = authCookieJar();
  return {
    ...process.env,
    FLEET_API_URL: fleetApiUrl(),
    ...(jar ? { FLEET_AUTH_COOKIE_JAR: jar } : {}),
  };
}
