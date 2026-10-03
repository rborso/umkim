import { useMemo } from 'react';
import {
  Button,
  PageSection,
  PageSectionVariants,
  Spinner,
  Title,
} from '@patternfly/react-core';
import { SyncIcon } from '@patternfly/react-icons';
import { fleetSitePhase } from '../api/client';
import { ErrorAlert } from '../components/ErrorAlert';
import { HealthCards, type HealthEntry } from '../components/HealthCards';
import { useFleetClustersQuery, useFleetSitesQuery, useInvalidateFleet } from '../hooks/useFleetQueries';

export function OverviewPage() {
  const { invalidateSites, invalidateClusters } = useInvalidateFleet();
  const sitesQuery = useFleetSitesQuery(30_000);
  const clustersQuery = useFleetClustersQuery(30_000);

  const loading = sitesQuery.isLoading || clustersQuery.isLoading;
  const fetching = sitesQuery.isFetching || clustersQuery.isFetching;
  const error = sitesQuery.error || clustersQuery.error;

  const health = useMemo((): HealthEntry[] => {
    const sites = sitesQuery.data ?? [];
    const clusters = clustersQuery.data ?? [];
    const ready = clusters.filter((c) => c.provisioning_phase === 'Ready').length;
    const failed = clusters.filter((c) => c.provisioning_phase === 'Failed').length;
    const sitePhases = sites.map(fleetSitePhase);
    return [
      {
        label: 'Sites',
        ok: true,
        detail: `${sites.length} registered` + (sitePhases.length ? ` (${sitePhases.join(', ')})` : ''),
      },
      {
        label: 'Clusters',
        ok: failed === 0,
        detail: `${clusters.length} total · ${ready} Ready` + (failed ? ` · ${failed} Failed` : ''),
      },
    ];
  }, [sitesQuery.data, clustersQuery.data]);

  const updated = useMemo(() => {
    const t = Math.max(sitesQuery.dataUpdatedAt, clustersQuery.dataUpdatedAt);
    return t ? new Date(t).toLocaleString() : '';
  }, [sitesQuery.dataUpdatedAt, clustersQuery.dataUpdatedAt]);

  const refresh = () => {
    void invalidateSites();
    void invalidateClusters();
  };

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1" size="2xl">
          Overview
        </Title>
        <p style={{ marginTop: '0.5rem', color: '#6a6e73' }}>
          Fleet sites and clusters on this hub. Create a workload site, then join Rocky 10 nodes
          from Clusters. When a cluster is Ready, install Helm charts from Apps (you select the
          cluster).
        </p>
        <div
          style={{
            marginTop: '1rem',
            display: 'flex',
            gap: '0.75rem',
            alignItems: 'center',
            flexWrap: 'wrap',
          }}
        >
          <Button variant="secondary" icon={<SyncIcon />} onClick={refresh} isDisabled={fetching}>
            Refresh
          </Button>
          {loading && <Spinner size="md" />}
          {fetching && !loading && <Spinner size="sm" />}
          {updated && !loading && (
            <span style={{ fontSize: '0.9rem', color: '#6a6e73' }}>Updated {updated}</span>
          )}
        </div>
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        {!error && (health.length > 0 || !loading) && <HealthCards entries={health} />}
      </PageSection>
    </>
  );
}
