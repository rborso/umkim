import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Label, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { listFleetClusterVMs } from '../../api/client';
import type { FleetSite } from '../../api/types';
import { useFleetClustersQuery } from '../../hooks/useFleetQueries';
import { ErrorAlert } from '../ErrorAlert';
import { combineSiteVmResults, siteClusterNames } from './siteVms';
import type { SiteVmRow } from './siteVms';

function phaseColor(phase?: string): 'green' | 'red' | 'blue' | 'grey' | 'orange' {
  switch (phase) {
    case 'Running':
      return 'green';
    case 'Failed':
      return 'red';
    case 'Starting':
    case 'Scheduling':
    case 'Provisioning':
    case 'Pending':
    case 'WaitingForVolumeBinding':
      return 'orange';
    case 'Stopping':
    case 'Terminating':
      return 'blue';
    default:
      return 'grey';
  }
}

/**
 * Read-only aggregate of KubeVirt VirtualMachines across every cluster managed
 * in the site. Console/power controls stay on each cluster's Virtual machines
 * page.
 */
export function SiteKubeVirtPanel({ site }: { site: FleetSite }) {
  const { data: clusters = [] } = useFleetClustersQuery(30_000);
  const clusterNames = useMemo(() => siteClusterNames(site, clusters), [site, clusters]);
  const [rows, setRows] = useState<SiteVmRow[]>([]);
  const [failedClusters, setFailedClusters] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async () => {
    setError(null);
    if (clusterNames.length === 0) {
      setRows([]);
      setFailedClusters([]);
      setLoading(false);
      return;
    }
    try {
      const results = await Promise.allSettled(
        clusterNames.map((name) => listFleetClusterVMs(name)),
      );
      const combined = combineSiteVmResults(clusterNames, results);
      setRows(combined.rows);
      setFailedClusters(combined.failedClusters);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [clusterNames]);

  useEffect(() => {
    void load();
    const t = window.setInterval(() => void load(), 10_000);
    return () => window.clearInterval(t);
  }, [load]);

  return (
    <>
      <Title headingLevel="h3" style={{ marginTop: '1rem' }}>
        KubeVirt machines
      </Title>
      <p style={{ marginTop: '0.5rem', maxWidth: '48rem', fontSize: '0.9rem' }}>
        VirtualMachines from clusters managed in this site. Serial/VNC console and power actions are on
        each cluster&apos;s Virtual machines page.
      </p>
      <ErrorAlert error={error} />
      {!loading && clusterNames.length === 0 ? (
        <p style={{ color: '#6a6e73' }}>No clusters belong to this site yet.</p>
      ) : null}
      {clusterNames.length > 0 ? (
        <Table aria-label="Site KubeVirt machines" variant="compact" style={{ marginTop: '0.75rem' }}>
          <Thead>
            <Tr>
              <Th>Cluster</Th>
              <Th>Namespace</Th>
              <Th>Name</Th>
              <Th>Phase</Th>
              <Th>Ready</Th>
              <Th>Node</Th>
              <Th>IP</Th>
            </Tr>
          </Thead>
          <Tbody>
            {rows.length === 0 ? (
              <Tr>
                <Td colSpan={7}>
                  {loading ? 'Loading…' : 'No VirtualMachines found on this site’s clusters.'}
                </Td>
              </Tr>
            ) : (
              rows.map((vm) => (
                <Tr key={`${vm.cluster}/${vm.namespace}/${vm.name}`}>
                  <Td>
                    <Link to={`/clusters/${encodeURIComponent(vm.cluster)}`}>{vm.cluster}</Link>
                  </Td>
                  <Td>{vm.namespace}</Td>
                  <Td>
                    <Link to={`/clusters/${encodeURIComponent(vm.cluster)}/vms`}>{vm.name}</Link>
                  </Td>
                  <Td>
                    <Label color={phaseColor(vm.phase || vm.printableStatus)}>
                      {vm.phase || vm.printableStatus || '—'}
                    </Label>
                  </Td>
                  <Td>{vm.ready ? 'yes' : 'no'}</Td>
                  <Td>{vm.nodeName || '—'}</Td>
                  <Td>{vm.ip || '—'}</Td>
                </Tr>
              ))
            )}
          </Tbody>
        </Table>
      ) : null}
      {failedClusters.length > 0 ? (
        <p style={{ marginTop: '0.5rem', fontSize: '0.85rem', color: '#6a6e73' }}>
          No VM data from: <code>{failedClusters.join(', ')}</code> (KubeVirt may not be enabled).
        </p>
      ) : null}
    </>
  );
}
