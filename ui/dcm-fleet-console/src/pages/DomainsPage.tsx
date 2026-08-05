import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Button,
  PageSection,
  PageSectionVariants,
  Title,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  TextInput,
  Form,
} from '@patternfly/react-core';
import { SyncIcon } from '@patternfly/react-icons';
import { getEdgeDomains, validateEdgeDNS } from '../api/client';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';

export function DomainsPage() {
  const [registry, setRegistry] = useState<Record<string, unknown> | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const [dnsHost, setDnsHost] = useState('');
  const [expectedIp, setExpectedIp] = useState('');
  const [dnsResult, setDnsResult] = useState<Record<string, unknown> | null>(null);
  const [dnsLoading, setDnsLoading] = useState(false);
  const [dnsError, setDnsError] = useState<unknown>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getEdgeDomains();
      setRegistry((data.registry as Record<string, unknown>) || null);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const runDnsValidate = async () => {
    if (!dnsHost.trim()) return;
    setDnsLoading(true);
    setDnsError(null);
    try {
      setDnsResult(await validateEdgeDNS(dnsHost.trim(), expectedIp.trim() || undefined));
    } catch (e) {
      setDnsError(e);
      setDnsResult(null);
    } finally {
      setDnsLoading(false);
    }
  };

  const reg = registry || {};

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1" size="2xl">
          Domains
        </Title>
        <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
          Hub and site DNS zones from the fleet edge domain registry. Routable catalog instances
          use <code>routing.*</code> fields; hostnames must be unique across{' '}
          <code>FleetAppRoute</code> objects. Edit hub zones on{' '}
          <Link to="/sites/umkim">Site umkim</Link> (Edge and DNS tab).
        </p>
        <Button variant="secondary" icon={<SyncIcon />} onClick={load}>
          Refresh
        </Button>
      </PageSection>

      <PageSection>
        <ErrorAlert error={error} />
        {loading ? (
          <p>Loading domain registry…</p>
        ) : registry ? (
          <DescriptionList isCompact>
            <DescriptionListGroup>
              <DescriptionListTerm>Hub zone</DescriptionListTerm>
              <DescriptionListDescription>
                {String(reg.hub_zone || '—')}
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Homelab zone</DescriptionListTerm>
              <DescriptionListDescription>
                {String(reg.homelab_zone || '—')}
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Hub wildcard</DescriptionListTerm>
              <DescriptionListDescription>
                <code>{String(reg.hub_wildcard || '—')}</code>
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Homelab wildcard</DescriptionListTerm>
              <DescriptionListDescription>
                <code>{String(reg.homelab_wildcard || '—')}</code>
              </DescriptionListDescription>
            </DescriptionListGroup>
          </DescriptionList>
        ) : (
          <p>No domain registry ConfigMap found on the hub.</p>
        )}

        {Array.isArray(reg.sites) && (reg.sites as Record<string, unknown>[]).length > 0 ? (
          <>
            <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem' }}>
              Datacenter sites
            </Title>
            <table style={{ width: '100%', fontSize: '0.875rem', marginTop: '0.75rem' }}>
              <thead>
                <tr>
                  <th align="left">Site</th>
                  <th align="left">Wildcard zone</th>
                  <th align="left">Clusters</th>
                  <th align="left">DNS note</th>
                </tr>
              </thead>
              <tbody>
                {(reg.sites as Record<string, unknown>[]).map((site) => (
                  <tr key={String(site.id)}>
                    <td>
                      <strong>{String(site.display_name || site.id)}</strong>
                      <br />
                      <code>{String(site.id)}</code>
                    </td>
                    <td>
                      <code>{String(site.wildcard || '—')}</code>
                    </td>
                    <td>
                      {Array.isArray(site.clusters)
                        ? (site.clusters as string[]).join(', ')
                        : '—'}
                    </td>
                    <td style={{ fontSize: '0.8rem' }}>
                      {String(site.dns_target_note || '—')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        ) : null}

        <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem' }}>
          DNS validation
        </Title>
        <Form style={{ maxWidth: '28rem', marginTop: '0.75rem' }}>
          <LabeledField label="Hostname" fieldId="dns-host" help={getFieldHelp('domains.dnsHost')}>
            <TextInput
              id="dns-host"
              value={dnsHost}
              onChange={(_e, v) => setDnsHost(v)}
              placeholder="whoami.mgmt.umkim.com"
            />
          </LabeledField>
          <LabeledField label="Expected IP (optional)" fieldId="dns-ip" help={getFieldHelp('domains.expectedIp')}>
            <TextInput
              id="dns-ip"
              value={expectedIp}
              onChange={(_e, v) => setExpectedIp(v)}
              placeholder="198.51.100.10"
            />
          </LabeledField>
          <Button variant="primary" onClick={runDnsValidate} isLoading={dnsLoading}>
            Validate DNS
          </Button>
        </Form>
        <ErrorAlert error={dnsError} />
        {dnsResult ? (
          <pre
            style={{
              marginTop: '1rem',
              fontSize: '0.8rem',
              padding: '0.75rem',
              background: 'var(--pf-v5-global--BackgroundColor--200)',
            }}
          >
            {JSON.stringify(dnsResult, null, 2)}
          </pre>
        ) : null}
      </PageSection>
    </>
  );
}
