import {
  Alert,
  AlertVariant,
  Chip,
  ChipGroup,
  Spinner,
} from '@patternfly/react-core';
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table';
import type { PlacementPreviewResponse } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';

type Props = {
  preview: PlacementPreviewResponse | null;
  loading?: boolean;
  error?: unknown;
};

function formatUtil(v: number | null | undefined): string {
  if (v === null || v === undefined) return '—';
  return `${v}%`;
}

export function PlacementImpactPanel({ preview, loading, error }: Props) {
  if (loading) {
    return <Spinner size="md" aria-label="Loading placement preview" />;
  }
  if (error) {
    return <ErrorAlert error={error} />;
  }
  if (!preview) {
    return (
      <p style={{ fontSize: '0.875rem', color: 'var(--pf-v5-global--Color--200)' }}>
        Placement preview will appear when template and policy fields are set.
      </p>
    );
  }

  const clusters = preview.eligible_clusters || [];
  const names = clusters.map((c) => c.name).filter(Boolean);
  const count = preview.eligible_count ?? names.length;
  const effective = preview.effective_placement_count ?? count;
  const policyName =
    preview.winning_policy?.display_name || preview.winning_policy?.id || '—';
  const showUtilization = clusters.some((c) => c.metrics_ok || c.cpu_utilization != null);
  const showFit = clusters.some((c) => c.workload_fit);

  return (
    <div data-testid="placement-impact-panel">
      {preview.deployable === false ? (
        <Alert
          variant={AlertVariant.warning}
          title="No deploy targets"
          isInline
          style={{ marginBottom: '0.75rem' }}
        >
          {(preview.blocking_reasons || []).join(' ') ||
            preview.warnings?.join(' ') ||
            `${count} eligible cluster(s) — deployment will not place work.`}
        </Alert>
      ) : (
        <Alert
          variant={AlertVariant.success}
          title={`Will deploy to ${effective} cluster(s)`}
          isInline
          style={{ marginBottom: '0.75rem' }}
        >
          {names.length > 0 ? names.join(', ') : '—'}
          {preview.winning_policy ? ` (policy: ${policyName})` : null}
        </Alert>
      )}
      <p style={{ fontSize: '0.875rem', margin: '0 0 0.5rem' }}>
        <strong>Eligible clusters:</strong> {count}
        {preview.service_type ? ` · service_type=${preview.service_type}` : null}
      </p>
      {showUtilization || showFit ? (
        <Table variant="compact" aria-label="Cluster placement impact">
          <Thead>
            <Tr>
              <Th>Cluster</Th>
              <Th>CPU</Th>
              <Th>Memory</Th>
              {showFit ? <Th>Workload fit</Th> : null}
            </Tr>
          </Thead>
          <Tbody>
            {clusters.map((c) => (
              <Tr key={c.name || c.provider}>
                <Td>{c.name || '—'}</Td>
                <Td>{c.metrics_ok ? formatUtil(c.cpu_utilization) : '—'}</Td>
                <Td>{c.metrics_ok ? formatUtil(c.memory_utilization) : '—'}</Td>
                {showFit ? (
                  <Td>
                    {c.workload_fit || '—'}
                    {c.limits_over_capacity ? ' (limits risk)' : ''}
                  </Td>
                ) : null}
              </Tr>
            ))}
          </Tbody>
        </Table>
      ) : names.length > 0 ? (
        <ChipGroup>
          {names.map((n) => (
            <Chip key={n} isReadOnly>
              {n}
            </Chip>
          ))}
        </ChipGroup>
      ) : null}
      {(preview.warnings || []).length > 0 ? (
        <Alert variant={AlertVariant.warning} title="Warnings" isPlain isInline>
          <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
            {preview.warnings!.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        </Alert>
      ) : null}
    </div>
  );
}
