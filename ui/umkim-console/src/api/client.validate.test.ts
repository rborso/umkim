import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, validateFleetClusterNodeOperation } from './client';

describe('validateFleetClusterNodeOperation', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('sends a 401 toward the login page', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('denied', { status: 401 })));
    const hrefs: string[] = [];
    vi.stubGlobal('location', {
      pathname: '/fleet/clusters/lab',
      set href(value: string) {
        hrefs.push(value);
      },
      get href() {
        return hrefs.at(-1) ?? 'http://localhost/fleet/clusters/lab';
      },
    });

    await expect(validateFleetClusterNodeOperation('lab', { type: 'add' })).rejects.toBeInstanceOf(ApiError);
    expect(window.location.href).toBe('/fleet/login');
  });

  it('returns the validation body on 422', async () => {
    const body = { ok: false, phase: 'pre', checks: [{ name: 'ssh', ok: false, message: 'missing' }] };
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify(body), { status: 422, headers: { 'Content-Type': 'application/json' } })),
    );
    const result = await validateFleetClusterNodeOperation('lab', { type: 'add' });
    expect(result.ok).toBe(false);
    expect(result.checks[0]?.message).toBe('missing');
  });
});
