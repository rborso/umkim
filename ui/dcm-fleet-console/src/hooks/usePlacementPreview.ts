import { useCallback, useEffect, useRef, useState } from 'react';
import { previewPlacement } from '../api/client';
import type { PlacementPreviewRequest, PlacementPreviewResponse } from '../api/types';

export function usePlacementPreview(
  request: PlacementPreviewRequest | null,
  debounceMs = 300,
  options?: { autoFetch?: boolean },
): {
  preview: PlacementPreviewResponse | null;
  loading: boolean;
  error: unknown;
  refetch: () => Promise<void>;
} {
  const autoFetch = options?.autoFetch !== false;
  const [preview, setPreview] = useState<PlacementPreviewResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const requestRef = useRef(request);
  requestRef.current = request;

  const runFetch = useCallback(async (req: PlacementPreviewRequest | null) => {
    if (!req) {
      setPreview(null);
      setError(null);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await previewPlacement(req);
      setPreview(result);
    } catch (e) {
      setError(e);
      setPreview(null);
    } finally {
      setLoading(false);
    }
  }, []);

  const refetch = useCallback(async () => {
    await runFetch(requestRef.current);
  }, [runFetch]);

  useEffect(() => {
    if (!autoFetch) return;
    if (!request) {
      setPreview(null);
      setError(null);
      return;
    }

    let cancelled = false;
    const timer = window.setTimeout(() => {
      void (async () => {
        setLoading(true);
        setError(null);
        try {
          const result = await previewPlacement(request);
          if (!cancelled) setPreview(result);
        } catch (e) {
          if (!cancelled) {
            setError(e);
            setPreview(null);
          }
        } finally {
          if (!cancelled) setLoading(false);
        }
      })();
    }, debounceMs);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [request, debounceMs, autoFetch]);

  return { preview, loading, error, refetch };
}
