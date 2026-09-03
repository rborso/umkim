import {
  Button,
  Checkbox,
  Form,
  FormSelect,
  FormSelectOption,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { ExternalLinkAltIcon } from '@patternfly/react-icons';
import type { FleetCluster, FleetExposureMode, FleetTlsPolicy } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import { ExposureTlsPairWarning } from '../ExposureTlsPairWarning';
import type { ExposureTlsPairIssue } from '../../edge/exposureTlsPairs';
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
  exposureMode: FleetExposureMode | 'inherit';
  setExposureMode: (v: FleetExposureMode | 'inherit') => void;
  tlsPolicy: FleetTlsPolicy;
  setTlsPolicy: (v: FleetTlsPolicy) => void;
  directIngress: boolean;
  setDirectIngress: (v: boolean) => void;
  ingressEndpointId: string;
  setIngressEndpointId: (v: string) => void;
  domainBindings: string[];
  setDomainBindings: React.Dispatch<React.SetStateAction<string[]>>;
  upstreamHost: string;
  setUpstreamHost: (v: string) => void;
  traefikPort: string;
  setTraefikPort: (v: string) => void;
  siteExposureMode: string;
  exposureTlsIssue: ExposureTlsPairIssue | null;
  domainOptions: string[];
  authPrereqReady: boolean;
  authPrereqIngress: boolean;
  authPrereqDomains: boolean;
  authCanEnable: boolean;
  authEnabled: boolean;
  authActionBusy: boolean;
  authActionError: unknown;
  authReady: boolean;
  portalHost?: string;
  portalUrl?: string;
  lldapWebUIUrl?: string;
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
    exposureMode,
    setExposureMode,
    tlsPolicy,
    setTlsPolicy,
    directIngress,
    setDirectIngress,
    ingressEndpointId,
    setIngressEndpointId,
    domainBindings,
    setDomainBindings,
    upstreamHost,
    setUpstreamHost,
    traefikPort,
    setTraefikPort,
    siteExposureMode,
    exposureTlsIssue,
    domainOptions,
    authPrereqReady,
    authPrereqIngress,
    authPrereqDomains,
    authCanEnable,
    authEnabled,
    authActionBusy,
    authActionError,
    authReady,
    portalHost,
    portalUrl,
    lldapWebUIUrl,
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

  return (
    <Form style={{ maxWidth: '40rem', marginTop: '1rem' }}>
      <Title headingLevel="h3" size="md">
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
        Exposure and ingress
      </Title>
      <LabeledField label="Ingress enabled" fieldId="ingressEnabled" help={getFieldHelp('clusterDetail.ingressEnabled')}>
        <TextInput id="ingressEnabled" value={cluster.ingress_enabled ? 'yes' : 'no'} readOnly />
      </LabeledField>
      <LabeledField label="Exposure mode" fieldId="exposureMode" help={getFieldHelp('clusterDetail.exposureMode')}>
        <FormSelect
          id="exposureMode"
          value={exposureMode}
          onChange={(_e, v) => {
            const mode = v as FleetExposureMode | 'inherit';
            setExposureMode(mode);
            if (mode === 'port_forward' || mode === 'reverse_proxy') setDirectIngress(true);
            if (mode === 'none') setDirectIngress(false);
            if (mode === 'reverse_proxy' && (tlsPolicy === 'inherit' || tlsPolicy === 'acme_hub')) {
              setTlsPolicy('external');
            }
            if (mode === 'port_forward' && (tlsPolicy === 'inherit' || tlsPolicy === 'acme_hub')) {
              setTlsPolicy('acme_cluster');
            }
          }}
        >
          <FormSelectOption value="inherit" label={`Inherit from site (${siteExposureMode})`} />
          <FormSelectOption value="none" label="None" />
          <FormSelectOption value="hub_gateway" label="Hub gateway" />
          <FormSelectOption value="port_forward" label="Port forward" />
          <FormSelectOption value="reverse_proxy" label="External reverse proxy" />
        </FormSelect>
        {exposureMode === 'inherit' && (
          <p style={{ fontSize: '0.85rem', marginTop: '0.35rem' }}>
            Clusters without an override use the site default exposure mode ({siteExposureMode}).
          </p>
        )}
      </LabeledField>
      <LabeledField label="TLS policy" fieldId="tlsPolicy" help={getFieldHelp('clusterDetail.tlsPolicy')}>
        <FormSelect id="tlsPolicy" value={tlsPolicy} onChange={(_e, v) => setTlsPolicy(v as FleetTlsPolicy)}>
          <FormSelectOption value="inherit" label="Inherit" />
          <FormSelectOption value="external" label="External (no cluster LE)" />
          <FormSelectOption value="acme_cluster" label="ACME on cluster" />
          <FormSelectOption value="acme_hub" label="ACME on hub" />
          <FormSelectOption value="passthrough" label="TLS passthrough" />
        </FormSelect>
      </LabeledField>
      <ExposureTlsPairWarning issue={exposureTlsIssue} />
      <LabeledField
        label="Ingress endpoint ID"
        fieldId="ingressEndpointId"
        help={getFieldHelp('clusterDetail.ingressEndpointId')}
      >
        <TextInput
          id="ingressEndpointId"
          value={ingressEndpointId}
          onChange={(_e, v) => setIngressEndpointId(v)}
          placeholder="openwrt-wan"
        />
      </LabeledField>
      {domainOptions.length > 1 ? (
        <LabeledField label="Domain bindings" fieldId="domainBindings" help={getFieldHelp('clusterDetail.domainBindings')}>
          {domainOptions.map((id) => (
            <Checkbox
              key={id}
              id={`detail-domain-${id}`}
              label={id}
              isChecked={domainBindings.includes(id)}
              onChange={(_e, checked) =>
                setDomainBindings((current) =>
                  checked ? [...current, id] : current.filter((d) => d !== id),
                )
              }
            />
          ))}
        </LabeledField>
      ) : domainOptions.length === 1 ? (
        <LabeledField label="Domain binding" fieldId="domainBindings" help={getFieldHelp('clusterDetail.domainBindings')}>
          <FormSelect
            id="domainBindings"
            value={domainBindings[0] || ''}
            onChange={(_e, v) => setDomainBindings(v ? [v] : [])}
          >
            <FormSelectOption value="" label="None" />
            <FormSelectOption value={domainOptions[0]} label={domainOptions[0]} />
          </FormSelect>
        </LabeledField>
      ) : (
        <LabeledField label="Domain bindings (ids)" fieldId="domainBindings" help={getFieldHelp('clusterDetail.domainBindings')}>
          <TextInput
            id="domainBindings"
            value={domainBindings.join(', ')}
            onChange={(_e, v) =>
              setDomainBindings(
                v
                  .split(',')
                  .map((s) => s.trim())
                  .filter(Boolean),
              )
            }
            placeholder="site-zone, cluster-a"
          />
        </LabeledField>
      )}
      <LabeledField label="Direct ingress (legacy)" fieldId="directIngress" help={getFieldHelp('clusterDetail.directIngress')}>
        <Checkbox
          id="directIngress"
          isChecked={directIngress}
          onChange={(_e, checked) => setDirectIngress(checked)}
        />
      </LabeledField>
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
        <div style={{ fontSize: '0.875rem', marginBottom: '0.5rem' }}>
          <p style={{ margin: '0 0 0.35rem' }}>
            <strong>Prerequisites</strong>
          </p>
          <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
            <li style={{ color: authPrereqReady ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
              Cluster phase Ready {authPrereqReady ? '✓' : `(current: ${phase || '—'})`}
            </li>
            <li style={{ color: authPrereqIngress ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
              Ingress enabled (port forward or reverse proxy) {authPrereqIngress ? '✓' : '✗'}
            </li>
            <li style={{ color: authPrereqDomains ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
              Site domain bindings configured {authPrereqDomains ? '✓' : '✗'}
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
        <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
          <Button variant="primary" onClick={onEnableAuth} isDisabled={!authCanEnable || authActionBusy}>
            Enable ingress auth
          </Button>
          <Button variant="secondary" onClick={onDisableAuth} isDisabled={!authEnabled || authActionBusy}>
            Disable
          </Button>
        </div>
        {authActionError ? <ErrorAlert error={authActionError} /> : null}
      </LabeledField>

      {showClusterAcme && (
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
