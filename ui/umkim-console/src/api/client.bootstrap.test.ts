import { afterEach, describe, expect, it, vi } from 'vitest';
import { downloadFleetSiteBootstrapScript } from './client';

describe('downloadFleetSiteBootstrapScript', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends the session cookie', async () => {
    const fetchMock = vi.fn(async () => new Response('#!/bin/sh\n', { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await downloadFleetSiteBootstrapScript('homelab');
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/sites/homelab/bootstrap-script'),
      expect.objectContaining({ credentials: 'include' }),
    );
  });
});
