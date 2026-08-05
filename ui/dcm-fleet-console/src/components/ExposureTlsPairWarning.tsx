import { Alert, AlertVariant } from '@patternfly/react-core';
import type { ExposureTlsPairIssue } from '../edge/exposureTlsPairs';
import { EXPOSURE_TLS_DOCS_GITHUB_URL, EXPOSURE_TLS_DOCS_URL } from '../edge/exposureTlsPairs';

type Props = {
  issue: ExposureTlsPairIssue | null;
  /** Optional label when warning about a named domain zone */
  domainLabel?: string;
};

/** Inline warning when exposure + TLS is not a supported working pair. */
export function ExposureTlsPairWarning({ issue, domainLabel }: Props) {
  if (!issue) return null;
  const title = domainLabel
    ? `Unusual exposure + TLS pair (${domainLabel})`
    : 'Unusual exposure + TLS pair';
  return (
    <Alert
      variant={AlertVariant.warning}
      title={title}
      isInline
      style={{ marginTop: '0.75rem' }}
    >
      <p>{issue.message}</p>
      <p style={{ marginTop: '0.5rem', marginBottom: 0 }}>
        Recommended: <code>{issue.effectiveExposure}</code> + <code>{issue.recommendedTls}</code>
        {' · '}
        <a href={EXPOSURE_TLS_DOCS_URL} target="_blank" rel="noreferrer">
          Docs: working pairs
        </a>
        {' · '}
        <a href={EXPOSURE_TLS_DOCS_GITHUB_URL} target="_blank" rel="noreferrer">
          Source
        </a>
      </p>
    </Alert>
  );
}
