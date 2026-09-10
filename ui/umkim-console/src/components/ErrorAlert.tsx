import { Alert } from '@patternfly/react-core';
import { ApiError } from '../api/client';

function formatErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.body) {
      try {
        const parsed = JSON.parse(error.body) as { message?: string; error?: string };
        const detail = parsed.message || parsed.error;
        if (detail) return `${error.message}: ${detail}`;
      } catch {
        const trimmed = error.body.trim().slice(0, 500);
        if (trimmed) return `${error.message}: ${trimmed}`;
      }
    }
    return error.message;
  }
  if (error instanceof Error) return error.message;
  return String(error);
}

export function ErrorAlert({ error }: { error: unknown }) {
  if (!error) return null;
  const message = formatErrorMessage(error);
  return (
    <Alert variant="danger" title="Request failed" isInline>
      {message}
    </Alert>
  );
}
