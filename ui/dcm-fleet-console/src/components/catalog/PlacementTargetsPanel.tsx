import { useCallback, useState } from 'react';
import {
  Badge,
  Spinner,
  Title,
} from '@patternfly/react-core';
import {
  Table,
  Thead,
  Tr,
  Th,
  Tbody,
  Td,
} from '@patternfly/react-table';
import { getInstanceHubRoute, getInstancePlacementTargets } from '../../api/client';
import type { InstanceHubRouteResponse, PlacementTargetStatus, PlacementTargetsResponse } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import { usePoll } from '../../hooks/usePoll';

type Props = {
  instanceId: string;
  poll?: boolean;
};

export function hubRouteApplicable(route: InstanceHubRouteResponse | null | undefined): boolean {
  if (!route) return false;
  if (route.hub_route_applicable === false) return false;
  if (route.hub_route_applicable === true) return true;
  const mode = route.effective_mode;
  // Console no longer offers dual; still treat API dual as hub-applicable for status display.
  return mode === 'hub' || mode === 'dual';
}

function hubRouteBadge(route: InstanceHubRouteResponse | null | undefined) {
  if (!route) return null;
  if (!hubRouteApplicable(route)) {
    if (route.effective_mode === 'site') {
      return <Badge isRead color="green">Site route</Badge>;
    }
    return null;
  }
  const phase = route.failover_phase || 'Idle';
  if (phase === 'Relocating') {
    return <Badge isRead color="blue">Hub route: relocating</Badge>;
  }
  if (phase === 'Degraded' || route.hub_route_healthy === false) {
    return <Badge isRead color="red">Hub route: degraded</Badge>;
  }
  if (route.hub_route_healthy) {
    return <Badge isRead color="green">Hub route: healthy</Badge>;
  }
  return <Badge isRead>Hub route: unknown</Badge>;
}

export function PlacementTargetsPanel({ instanceId, poll = true }: Props) {
  const [data, setData] = useState<PlacementTargetsResponse | null>(null);
  const [hubRoute, setHubRoute] = useState<InstanceHubRouteResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async () => {
    if (!instanceId) return;
    setError(null);
    try {
      const [targets, route] = await Promise.all([
        getInstancePlacementTargets(instanceId),
        getInstanceHubRoute(instanceId),
      ]);
      setData(targets);
      setHubRoute(route);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [instanceId]);

  usePoll(load, 15000, poll && !!instanceId);

  if (!instanceId) return null;

  const targetCount = data?.target_count ?? 0;
  const healthyCount = data?.healthy_count ?? 0;
  const showReplication = targetCount > 1;

  const summaryBadge = () => {
    if (!data || targetCount === 0) return hubRouteBadge(hubRoute);
    if (!showReplication) {
      const target = data.targets?.[0];
      return (
        <>
          {hubRouteBadge(hubRoute)}
          {target ? (
            <Badge isRead color={targetStatusColor(target)}>
              {targetStatusLabel(target)}
            </Badge>
          ) : (
            <Badge isRead>—</Badge>
          )}
        </>
      );
    }
    const allOk = healthyCount === targetCount;
    return (
      <>
        {hubRouteBadge(hubRoute)}
        <Badge isRead color={allOk ? 'green' : healthyCount > 0 ? 'orange' : 'red'}>
          {healthyCount}/{targetCount} healthy
        </Badge>
      </>
    );
  };

  return (
    <div style={{ marginTop: '1rem', marginBottom: '1rem' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.5rem', flexWrap: 'wrap' }}>
        <Title headingLevel="h3" size="md">
          {showReplication ? 'Cluster replication' : 'Placement status'}
        </Title>
        {loading && !data ? <Spinner size="sm" /> : summaryBadge()}
      </div>
      {hubRoute?.active_cluster ? (
        <p style={{ fontSize: '0.8rem', opacity: 0.85, marginBottom: '0.5rem' }}>
          Active cluster: <code>{hubRoute.active_cluster}</code>
          {hubRoute.standby_cluster ? (
            <> · Standby: <code>{hubRoute.standby_cluster}</code></>
          ) : null}
          {hubRoute.last_failover_at ? (
            <> · Last failover: {new Date(hubRoute.last_failover_at).toLocaleString()}</>
          ) : null}
        </p>
      ) : null}
      <ErrorAlert error={error} />
      {data && data.targets && data.targets.length > 0 ? (
        <Table aria-label="Placement targets" variant="compact">
          <Thead>
            <Tr>
              <Th>Role</Th>
              <Th>Provider</Th>
              <Th>SPRM ID</Th>
              <Th>Status</Th>
              <Th>Message</Th>
            </Tr>
          </Thead>
          <Tbody>
            {data.targets.map((t) => (
              <Tr key={t.sprm_id}>
                <Td>{t.role || '—'}</Td>
                <Td>{t.provider || '—'}</Td>
                <Td>
                  <code style={{ fontSize: '0.75rem' }}>{t.sprm_id}</code>
                </Td>
                <Td>
                  <Badge isRead color={targetStatusColor(t)}>
                    {targetStatusLabel(t)}
                  </Badge>
                </Td>
                <Td style={{ fontSize: '0.85rem' }}>{t.status_message || '—'}</Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      ) : !loading && !error ? (
        <p style={{ fontSize: '0.875rem', opacity: 0.85 }}>No placement targets.</p>
      ) : null}
      {data?.approval_status ? (
        <p style={{ fontSize: '0.75rem', marginTop: '0.5rem', opacity: 0.75 }}>
          Policy approval: {data.approval_status}
        </p>
      ) : null}
    </div>
  );
}

/** Compact replication summary for instance list rows. */
export function replicationSummary(data: PlacementTargetsResponse | null | undefined): string {
  if (!data) return '—';
  const count = data.target_count ?? 0;
  if (count <= 1) {
    const t = data.targets?.[0];
    if (!t) return '—';
    return targetStatusLabel(t);
  }
  return `${data.healthy_count ?? 0}/${count}`;
}

function targetStatusLabel(target: PlacementTargetStatus): string {
  if (target.healthy) return 'Healthy';
  const status = (target.status || '').toLowerCase();
  if (status === 'active' || status === 'deployed') return 'Deployed';
  if (status === 'pending' || status === 'installing') return 'Pending';
  if (status === 'failed') return 'Failed';
  return target.status || '—';
}

function targetStatusColor(target: PlacementTargetStatus): 'green' | 'orange' | 'red' | undefined {
  if (target.healthy) return 'green';
  const status = (target.status || '').toLowerCase();
  if (status === 'pending' || status === 'installing') return 'orange';
  if (status === 'failed' || status === 'unreachable') return 'red';
  return undefined;
}

export function hubRouteSummary(route: InstanceHubRouteResponse | null | undefined): string {
  if (!route) return '—';
  if (!hubRouteApplicable(route)) {
    if (route.effective_mode === 'site') return 'site only';
    return 'n/a';
  }
  if (route.failover_phase === 'Relocating') return 'relocating';
  if (route.failover_phase === 'Degraded' || route.hub_route_healthy === false) return 'degraded';
  if (route.hub_route_healthy) return 'healthy';
  return '—';
}
