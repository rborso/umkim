import {
  Button,
  Form,
  FormSelect,
  FormSelectOption,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { ExternalLinkAltIcon } from '@patternfly/react-icons';
import type { FleetCluster, FleetSite, FleetSiteIngress } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import { effectiveClusterJumphost } from '../../edge/jumphost';
import {
  formatIngressOptionLabel,
  ingressAuthSupported,
  ingressMethodLabel,
  ingressOptionDisabled,
  siteIngressById,
} from '../../edge/ingressMethod';
import { LabeledField } from '../forms/LabeledField';
import { LabelKeyValueEditor } from '../forms/LabelKeyValueEditor';
import { getFieldHelp } from '../../help/fieldHelp';

const CAPABILITY_LABEL_PREFIX = 'fleet.capability/';

function editableClusterLabels(labels: Record<string, string>): Record<string, string> {
  return Object.fromEntries(
    Object.entries(labels).filter(([k]) => !k.startsWith(CAPABILITY_LABEL_PREFIX)),
  );
}

export type ClusterDetailEdgeTabProps = {
  cluster: FleetCluster;
  phase: string;
  site?: FleetSite;
  siteId: string;
  setSiteId: (v: string) => void;
  clusterLabels: Record<string, string>;
  setClusterLabels: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  region: string;
  setRegion: (v: string) => void;
  nestDepth: string;
  setNestDepth: (v: string) => void;
  provisioner: string;
  setProvisioner: (v: string) => void;
  ingressId: string;
  setIngressId: (v: string) => void;
  jumphost: string;
  setJumphost: (v: string) => void;
  upstreamHost: string;
  setUpstreamHost: (v: string) => void;
  traefikPort: string;
  setTraefikPort: (v: string) => void;
  ingressOptions: FleetSiteIngress[];
  authPrereqReady: boolean;
  authPrereqIngress: boolean;
  authPrereqIngressBinding: boolean;
  authCanEnable: boolean;
  authEnabled: boolean;
  authActionBusy: boolean;
  authActionError: unknown;
  authReady: boolean;
  portalHost?: string;
  portalUrl?: string;
  lldapWebUIUrl?: string;
  directory?: string;
  onSetDirectory: (dir: 'local' | 'hub') => void;
  showClusterAcme: boolean;
  acmeEnabled: boolean;
  acmeCanEnable: boolean;
  onEnableAuth: () => void;
  onDisableAuth: () => void;
  onEnableAcme: () => void;
  onDisableAcme: () => void;
  saving: boolean;
  canSave: boolean;
  onSave: () => void;
};

export function ClusterDetailEdgeTab(props: ClusterDetailEdgeTabProps) {
  const {
    cluster,
    phase,
    site,
    siteId,
    setSiteId,
    clusterLabels,
    setClusterLabels,
    region,
    setRegion,
    nestDepth,
    setNestDepth,
    provisioner,
    setProvisioner,
    ingressId,
    setIngressId,
    jumphost,
    setJumphost,
    upstreamHost,
    setUpstreamHost,
    traefikPort,
    setTraefikPort,
    ingressOptions,
    authPrereqReady,
    authPrereqIngress,
    authPrereqIngressBinding,
    authCanEnable,
    authEnabled,
    authActionBusy,
    authActionError,
    authReady,
    portalHost,
    portalUrl,
    lldapWebUIUrl,
    directory,
    onSetDirectory,
    showClusterAcme,
    acmeEnabled,
    acmeCanEnable,
    onEnableAuth,
    onDisableAuth,
    onEnableAcme,
    onDisableAcme,
    saving,
    canSave,
    onSave,
  } = props;

  const boundIngress = site && ingressId ? siteIngressById(site, ingressId) : undefined;
  const ingressMethod = cluster.method || boundIngress?.method;
  const authSupported = ingressAuthSupported(ingressMethod);
  const effectiveJumphost = effectiveClusterJumphost(jumphost, site?.spec?.jumphost);

  return (
    <Form style={{ maxWidth: '40rem', marginTop: '1rem' }}>
      <Title headingLevel="h3" size="md">
        Connectivity
      </Title>
      <LabeledField label="Jumphost override" fieldId="clusterJumphost" help={getFieldHelp('clusterDetail.jumphost')}>
        <TextInput
          id="clusterJumphost"
          data-testid="cluster-jumphost"
          value={jumphost}
          onChange={(_e, v) => setJumphost(v)}
          placeholder="user@bastion.example.com:22"
        />
      </LabeledField>
      <p style={{ fontSize: '0.85rem', marginTop: '-0.25rem' }}>
        Effective jumphost:{' '}
        {effectiveJumphost.source === 'none' ? (
          <em>none (direct SSH)</em>
        ) : effectiveJumphost.source === 'site' ? (
          <>
            <code>{effectiveJumphost.effective}</code> <em>(from site)</em>
          </>
        ) : (
          <code>{effectiveJumphost.effective}</code>
        )}
      </p>

      <Title headingLevel="h3" size="md" style={{ marginTop: '1.5rem' }}>
        Site and topology
      </Title>
      <LabeledField label="Site ID (fleet.site)" fieldId="siteId" help={getFieldHelp('clusterDetail.siteId')}>
        <TextInput
          id="siteId"
          value={clusterLabels['fleet.site'] ?? siteId}
          onChange={(_e, v) => {
            setSiteId(v);
            setClusterLabels((current) => ({ ...current, 'fleet.site': v }));
          }}
        />
      </LabeledField>
      <LabeledField label="Region" fieldId="region" help={getFieldHelp('clusterDetail.region')}>
        <TextInput
          id="region"
          value={clusterLabels['fleet.topology/region'] ?? region}
          onChange={(_e, v) => {
            setRegion(v);
            setClusterLabels((current) => ({ ...current, 'fleet.topology/region': v }));
          }}
        />
      </LabeledField>
      <LabeledField label="Nest depth" fieldId="nestDepth" help={getFieldHelp('clusterDetail.nestDepth')}>
        <TextInput
          id="nestDepth"
          value={clusterLabels['fleet.topology/nest_depth'] ?? nestDepth}
          onChange={(_e, v) => {
            setNestDepth(v);
            setClusterLabels((current) => ({ ...current, 'fleet.topology/nest_depth': v }));
          }}
        />
      </LabeledField>
      <LabeledField label="Provisioner" fieldId="provisioner" help={getFieldHelp('clusterDetail.provisioner')}>
        <TextInput
          id="provisioner"
          value={clusterLabels['fleet.topology/provisioner'] ?? provisioner}
          onChange={(_e, v) => {
            setProvisioner(v);
            setClusterLabels((current) => ({ ...current, 'fleet.topology/provisioner': v }));
          }}
        />
      </LabeledField>
      <LabeledField
        label="Cluster labels (OCM)"
        fieldId="clusterLabels"
        help={getFieldHelp('clusterDetail.clusterLabels')}
      >
        <LabelKeyValueEditor
          labels={editableClusterLabels(clusterLabels)}
          onChange={(next) =>
            setClusterLabels((current) => ({
              ...Object.fromEntries(
                Object.entries(current).filter(([k]) => k.startsWith(CAPABILITY_LABEL_PREFIX)),
              ),
              ...next,
            }))
          }
          idPrefix="cluster-detail-label"
        />
        {Object.entries(clusterLabels).some(([k]) => k.startsWith(CAPABILITY_LABEL_PREFIX)) ? (
          <div style={{ marginTop: '0.5rem', fontSize: '0.85rem' }}>
            <strong>Discovered capabilities (read-only):</strong>
            <pre style={{ fontSize: '0.75rem', margin: '0.35rem 0 0' }}>
              {JSON.stringify(
                Object.fromEntries(
                  Object.entries(clusterLabels).filter(([k]) => k.startsWith(CAPABILITY_LABEL_PREFIX)),
                ),
                null,
                2,
              )}
            </pre>
          </div>
        ) : null}
      </LabeledField>

      <Title headingLevel="h3" size="md" style={{ marginTop: '1.5rem' }}>
        Ingress binding
      </Title>
      <LabeledField label="Ingress enabled" fieldId="ingressEnabled" help={getFieldHelp('clusterDetail.ingressEnabled')}>
        <TextInput id="ingressEnabled" value={cluster.ingress_enabled ? 'yes' : 'no'} readOnly />
      </LabeledField>
      {ingressOptions.length > 0 ? (
        <LabeledField label="Site ingress" fieldId="ingressId" help={getFieldHelp('clusterDetail.domainBindings')}>
          <FormSelect id="ingressId" value={ingressId} onChange={(_e, v) => setIngressId(v)}>
            <FormSelectOption value="" label="None" />
            {ingressOptions.map((ent) => (
              <FormSelectOption
                key={ent.id}
                value={ent.id}
                label={formatIngressOptionLabel(ent)}
                isDisabled={ingressOptionDisabled(ent, cluster.name)}
              />
            ))}
          </FormSelect>
        </LabeledField>
      ) : (
        <LabeledField label="Site ingress ID" fieldId="ingressId" help={getFieldHelp('clusterDetail.domainBindings')}>
          <TextInput id="ingressId" value={ingressId} onChange={(_e, v) => setIngressId(v)} />
        </LabeledField>
      )}
      <LabeledField label="Ingress method" fieldId="ingressMethod">
        <TextInput
          id="ingressMethod"
          value={ingressMethod ? ingressMethodLabel(ingressMethod) : '—'}
          readOnly
        />
      </LabeledField>
      {boundIngress?.method === 'port_forward' && (
        <>
          <LabeledField label="DNS zone" fieldId="dnsZone">
            <TextInput id="dnsZone" value={cluster.dns_zone || boundIngress.zone || '—'} readOnly />
          </LabeledField>
          <LabeledField label="WAN address" fieldId="edgeWanIp">
            <TextInput id="edgeWanIp" value={cluster.edge_wan_ip || boundIngress.address || '—'} readOnly />
          </LabeledField>
        </>
      )}
      {boundIngress?.method === 'reverse_proxy' && boundIngress.description && (
        <LabeledField label="Ingress description" fieldId="ingressDescription">
          <TextInput id="ingressDescription" value={boundIngress.description} readOnly />
        </LabeledField>
      )}
      <LabeledField label="Upstream host (WG or LAN IP)" fieldId="upstreamHost" help={getFieldHelp('clusterDetail.upstreamHost')}>
        <TextInput id="upstreamHost" value={upstreamHost} onChange={(_e, v) => setUpstreamHost(v)} />
      </LabeledField>
      <LabeledField label="Traefik HTTP NodePort" fieldId="traefikPort" help={getFieldHelp('clusterDetail.traefikPort')}>
        <TextInput id="traefikPort" value={traefikPort} onChange={(_e, v) => setTraefikPort(v)} />
      </LabeledField>

      <Title headingLevel="h3" size="md" style={{ marginTop: '1.5rem' }}>
        Ingress authentication
      </Title>
      <LabeledField label="Forward-auth (Authelia + LLDAP)" fieldId="ingressAuth" help={getFieldHelp('clusterDetail.ingressAuth')}>
        {!authSupported ? (
          <p style={{ fontSize: '0.875rem', margin: '0 0 0.5rem' }}>
            Ingress authentication is only available for <code>port_forward</code> clusters.
          </p>
        ) : (
          <div style={{ fontSize: '0.875rem', marginBottom: '0.5rem' }}>
            <p style={{ margin: '0 0 0.35rem' }}>
              <strong>Prerequisites</strong>
            </p>
            <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
              <li style={{ color: authPrereqReady ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
                Cluster phase Ready {authPrereqReady ? '✓' : `(current: ${phase || '—'})`}
              </li>
              <li style={{ color: authPrereqIngress ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
                Ingress enabled {authPrereqIngress ? '✓' : '✗'}
              </li>
              <li style={{ color: authPrereqIngressBinding ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
                Site ingress binding configured {authPrereqIngressBinding ? '✓' : '✗'}
              </li>
              <li>
                DNS: point <code>auth.&lt;site-zone&gt;</code> and app wildcards at this cluster edge
                {portalHost ? (
                  <>
                    {' '}
                    (portal: <code>{portalHost}</code>)
                  </>
                ) : null}
              </li>
            </ul>
          </div>
        )}
        {cluster.ingressAuthStatus?.phase && (
          <p style={{ marginTop: '0.35rem', fontSize: '0.875rem' }}>
            Status: {cluster.ingressAuthStatus.phase}
            {cluster.ingressAuthStatus.message ? ` — ${cluster.ingressAuthStatus.message}` : ''}
          </p>
        )}
        {authReady && portalUrl && (
          <p style={{ marginTop: '0.35rem', fontSize: '0.875rem' }}>
            <a href={portalUrl} target="_blank" rel="noreferrer">
              Auth portal <ExternalLinkAltIcon />
            </a>
            {lldapWebUIUrl ? (
              <>
                {' '}
                ·{' '}
                <a href={lldapWebUIUrl} target="_blank" rel="noreferrer">
                  LLDAP UI <ExternalLinkAltIcon />
                </a>
              </>
            ) : null}
            {cluster.ingressAuthStatus?.accessDomains?.length ? (
              <> — access domains: {cluster.ingressAuthStatus.accessDomains.join(', ')}</>
            ) : null}
          </p>
        )}
        {authEnabled ? (
          <LabeledField label="Directory" fieldId="clusterDirectory" help={getFieldHelp('clusterDetail.ingressAuth')}>
            <FormSelect
              id="clusterDirectory"
              value={directory === 'hub' ? 'hub' : 'local'}
              onChange={(_e, v) => onSetDirectory(v as 'local' | 'hub')}
              isDisabled={authActionBusy}
            >
              <FormSelectOption value="local" label="local — this cluster runs its own LLDAP" />
              <FormSelectOption value="hub" label="hub — shared hub directory (WireGuard)" />
            </FormSelect>
          </LabeledField>
        ) : null}
        <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
          <Button
            variant="primary"
            onClick={onEnableAuth}
            isDisabled={!authSupported || !authCanEnable || authActionBusy}
          >
            Enable ingress auth
          </Button>
          <Button
            variant="secondary"
            onClick={onDisableAuth}
            isDisabled={!authSupported || !authEnabled || authActionBusy}
          >
            Disable
          </Button>
        </div>
        {authActionError ? <ErrorAlert error={authActionError} /> : null}
      </LabeledField>

      {showClusterAcme && authSupported && (
        <>
          <Title headingLevel="h3" size="md" style={{ marginTop: '1.5rem' }}>
            Certificates
          </Title>
          <LabeledField label="ACME on cluster" fieldId="clusterAcme" help={getFieldHelp('clusterDetail.clusterAcme')}>
            {cluster.acmeStatus?.phase && (
              <p style={{ marginTop: 0, fontSize: '0.875rem' }}>
                Status: {cluster.acmeStatus.phase}
                {cluster.acmeStatus.message ? ` — ${cluster.acmeStatus.message}` : ''}
              </p>
            )}
            <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
              <Button variant="primary" onClick={onEnableAcme} isDisabled={!acmeCanEnable || authActionBusy}>
                Enable Let&apos;s Encrypt on cluster
              </Button>
              <Button variant="secondary" onClick={onDisableAcme} isDisabled={!acmeEnabled || authActionBusy}>
                Disable
              </Button>
            </div>
          </LabeledField>
        </>
      )}

      <div style={{ marginTop: '1.5rem' }}>
        <Button variant="primary" isLoading={saving} isDisabled={!canSave} onClick={onSave}>
          Save cluster settings
        </Button>
      </div>
    </Form>
  );
}
