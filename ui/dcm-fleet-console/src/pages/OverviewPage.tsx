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
  getHealth,
  listCatalogItemInstances,
  listCatalogItems,
  listPolicies,
  listProviders,
} from '../api/client';
import { ErrorAlert } from '../components/ErrorAlert';
import { HealthCards, type HealthEntry } from '../components/HealthCards';
import { usePoll } from '../hooks/usePoll';

function formatHealthDetail(value: unknown): string {
  if (value === null || value === undefined) return 'No data';
  if (typeof value === 'string') return value.slice(0, 160);
  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>;
    if ('status' in obj) return `status: ${String(obj.status)}`;
    if ('healthy' in obj) return `healthy: ${String(obj.healthy)}`;
    if ('message' in obj) return String(obj.message).slice(0, 160);
    const keys = Object.keys(obj);
    if (keys.length === 0) return 'OK';
    return keys.slice(0, 4).map((k) => `${k}: ${String(obj[k])}`).join(', ');
  }
  return String(value).slice(0, 160);
}

export function OverviewPage() {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [health, setHealth] = useState<HealthEntry[]>([]);
  const [counts, setCounts] = useState({
    providers: 0,
    catalog: 0,
    instances: 0,
    policies: 0,
  });
  const [updated, setUpdated] = useState<string>('');

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const healthPaths = [
        ['Providers', 'providers'],
        ['Catalog', 'catalog'],
        ['Policies', 'policies'],
        ['Placement', 'placement'],
      ] as const;

      const healthResults = await Promise.allSettled(
        healthPaths.map(([, p]) => getHealth(p)),
      );

      const cards: HealthEntry[] = healthPaths.map(([label], i) => {
        const r = healthResults[i];
        const ok = r.status === 'fulfilled';
        const detail = ok
          ? formatHealthDetail(r.value)
          : (r.reason as Error).message;
        return { label, ok, detail };
      });
      setHealth(cards);

      const [prov, catalog, instances, policies] = await Promise.all([
        listProviders(),
        listCatalogItems(),
        listCatalogItemInstances(),
        listPolicies(),
      ]);
      setCounts({
        providers: prov.length,
        catalog: catalog.length,
        instances: instances.length,
        policies: policies.length,
      });
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
          Control plane health, providers, catalog, and policies via the hub API gateway.
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
        {!error && !loading && (
          <p style={{ marginTop: '1.25rem' }}>
            <strong>{counts.providers}</strong> providers ·{' '}
            <strong>{counts.catalog}</strong> catalog items ·{' '}
            <strong>{counts.instances}</strong> deployed instances ·{' '}
            <strong>{counts.policies}</strong> policies
          </p>
        )}
      </PageSection>
    </>
  );
}
