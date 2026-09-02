import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  PageSection,
  PageSectionVariants,
  Spinner,
  Title,
} from '@patternfly/react-core';
import { SyncIcon } from '@patternfly/react-icons';
import {
  fleetSitePhase,
  listFleetClusters,
  listFleetSites,
} from '../api/client';
import { ErrorAlert } from '../components/ErrorAlert';
import { HealthCards, type HealthEntry } from '../components/HealthCards';
import { usePoll } from '../hooks/usePoll';

export function OverviewPage() {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [health, setHealth] = useState<HealthEntry[]>([]);
  const [updated, setUpdated] = useState<string>('');

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [sites, clusters] = await Promise.all([listFleetSites(), listFleetClusters()]);
      const ready = clusters.filter((c) => c.provisioning_phase === 'Ready').length;
      const failed = clusters.filter((c) => c.provisioning_phase === 'Failed').length;
      const sitePhases = sites.map(fleetSitePhase);
      setHealth([
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
      ]);
      setUpdated(new Date().toLocaleString());
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  usePoll(load, 30_000);

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
          <Button variant="secondary" icon={<SyncIcon />} onClick={load} isDisabled={loading}>
            Refresh
          </Button>
          {loading && <Spinner size="md" />}
          {updated && !loading && (
            <span style={{ fontSize: '0.9rem', color: '#6a6e73' }}>Updated {updated}</span>
          )}
        </div>
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        {!error && <HealthCards entries={health} />}
      </PageSection>
    </>
  );
}
