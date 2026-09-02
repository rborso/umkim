import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Button,
  Checkbox,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Form,
  PageSection,
  PageSectionVariants,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  TextInput,
} from '@patternfly/react-core';
import { SyncIcon } from '@patternfly/react-icons';
import { getHubInfrastructure, getIngressAuth, getAcme, getLLM, patchIngressAuth, patchAcme, patchLLM } from '../api/client';
import type { FleetIngressAuth, FleetAcme, FleetLLM, HubInfrastructure } from '../api/types';
import { useAuth } from '../auth/AuthContext';
import { ClusterNodesPanel } from '../components/clusters/ClusterNodesPanel';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';

export function InfrastructurePage() {
  const { user } = useAuth();
  const [hub, setHub] = useState<HubInfrastructure | null>(null);
  const [ingressAuth, setIngressAuth] = useState<FleetIngressAuth | null>(null);
  const [acme, setAcme] = useState<FleetAcme | null>(null);
  const [llm, setLlm] = useState<FleetLLM | null>(null);
  const [hubAcmeEnabled, setHubAcmeEnabled] = useState(false);
  const [hubLlmEnabled, setHubLlmEnabled] = useState(false);
  const [hubLlmPublicHost, setHubLlmPublicHost] = useState('');
  const [hubAuthEnabled, setHubAuthEnabled] = useState(false);
  const [hubLldapWebUI, setHubLldapWebUI] = useState(false);
  const [loading, setLoading] = useState(true);
  const [savingAuth, setSavingAuth] = useState(false);
  const [savingAcme, setSavingAcme] = useState(false);
  const [savingLlm, setSavingLlm] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [authError, setAuthError] = useState<unknown>(null);
  const [acmeError, setAcmeError] = useState<unknown>(null);
  const [llmError, setLlmError] = useState<unknown>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [hubData, authData, acmeData, llmData] = await Promise.all([
        getHubInfrastructure(),
        getIngressAuth(),
        getAcme(),
        getLLM(),
      ]);
      setHub(hubData);
      setIngressAuth(authData);
      setAcme(acmeData);
      setLlm(llmData);
      setHubAuthEnabled(!!authData.hubEnabled);
      setHubLldapWebUI(!!authData.hubExposeLldapWebUI);
      setHubAcmeEnabled(!!acmeData.hubEnabled);
      setHubLlmEnabled(!!llmData.hubEnabled);
      setHubLlmPublicHost(llmData.publicHost ?? '');
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const saveIngressAuth = async () => {
    setSavingAuth(true);
    setAuthError(null);
    try {
      const updated = await patchIngressAuth({
        hubEnabled: hubAuthEnabled,
        hubExposeLldapWebUI: hubLldapWebUI,
      });
      setIngressAuth(updated);
      setHubAuthEnabled(!!updated.hubEnabled);
      setHubLldapWebUI(!!updated.hubExposeLldapWebUI);
      await load();
    } catch (e) {
      setAuthError(e);
    } finally {
      setSavingAuth(false);
    }
  };

  const saveAcme = async () => {
    setSavingAcme(true);
    setAcmeError(null);
    try {
      const updated = await patchAcme({ hubEnabled: hubAcmeEnabled });
      setAcme(updated);
      setHubAcmeEnabled(!!updated.hubEnabled);
      await load();
    } catch (e) {
      setAcmeError(e);
    } finally {
      setSavingAcme(false);
    }
  };

  const saveLlm = async () => {
    setSavingLlm(true);
    setLlmError(null);
    try {
      const updated = await patchLLM({
        hubEnabled: hubLlmEnabled,
        ...(hubLlmPublicHost.trim() ? { publicHost: hubLlmPublicHost.trim() } : {}),
      });
      setLlm(updated);
      setHubLlmEnabled(!!updated.hubEnabled);
      setHubLlmPublicHost(updated.publicHost ?? hubLlmPublicHost.trim());
      await load();
    } catch (e) {
      setLlmError(e);
    } finally {
      setSavingLlm(false);
    }
  };

  const portalUrl = ingressAuth?.portalHost
    ? `https://${ingressAuth.portalHost}`
    : hub?.ingress_auth?.portalHost
      ? `https://${hub.ingress_auth.portalHost}`
      : '';

  const lldapWebUIUrl = ingressAuth?.hub?.lldapWebUIURL;
  const consoleLldapDeployed = ingressAuth?.consoleLldapDeployed ?? false;

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1">Infrastructure</Title>
        <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
          Physical hub host (K3s, DCM, Harbor, Fleet Console). DNS zones and WAN exposure are
          configured on{' '}
          <Link to={`/sites/${hub?.management_site_id || 'umkim'}`}>
            Site {hub?.management_site_id || 'umkim'}
          </Link>
          . OCM workload clusters are listed under <Link to="/clusters">Clusters</Link>.
        </p>
        <Toolbar style={{ marginTop: '0.75rem' }}>
          <ToolbarContent>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={() => void load()}>
                Refresh
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Link to={`/sites/${hub?.management_site_id || 'umkim'}`}>
                Edit hub DNS → Site {hub?.management_site_id || 'umkim'}
              </Link>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        {loading && <p>Loading…</p>}
        {hub && (
          <>
            <DescriptionList isHorizontal style={{ maxWidth: '48rem' }}>
              <DescriptionListGroup>
                <DescriptionListTerm>Host</DescriptionListTerm>
                <DescriptionListDescription>
                  <code>{hub.host_name}</code>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Management site</DescriptionListTerm>
                <DescriptionListDescription>
                  <Link to={`/sites/${hub.management_site_id}`}>{hub.management_site_id}</Link>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>WireGuard</DescriptionListTerm>
                <DescriptionListDescription>
                  <code>{hub.wireguard_ip || '—'}</code>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Public endpoint</DescriptionListTerm>
                <DescriptionListDescription>
                  {hub.public_endpoint || '—'}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Exposure</DescriptionListTerm>
                <DescriptionListDescription>{hub.exposure_mode || '—'}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Hub zone</DescriptionListTerm>
                <DescriptionListDescription>
                  <code>{hub.hub_zone || '—'}</code>
                </DescriptionListDescription>
              </DescriptionListGroup>
              {hub.dns_target_note && (
                <DescriptionListGroup>
                  <DescriptionListTerm>DNS note</DescriptionListTerm>
                  <DescriptionListDescription>{hub.dns_target_note}</DescriptionListDescription>
                </DescriptionListGroup>
              )}
            </DescriptionList>

            <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem' }}>
              Ingress authentication
            </Title>
            <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
              Authelia forward-auth with a dedicated LLDAP directory for public app hostnames. This is
              separate from fleet console login (Users / Groups). Manage ingress users via the LLDAP
              UI on the auth portal host after enabling.
            </p>
            <ErrorAlert error={authError} />
            <DescriptionList isHorizontal style={{ maxWidth: '48rem', marginTop: '0.75rem' }}>
              <DescriptionListGroup>
                <DescriptionListTerm>Status</DescriptionListTerm>
                <DescriptionListDescription>
                  {ingressAuth?.phase || hub.ingress_auth?.phase || '—'}
                  {ingressAuth?.message ? ` — ${ingressAuth.message}` : ''}
                </DescriptionListDescription>
              </DescriptionListGroup>
              {portalUrl && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Auth portal</DescriptionListTerm>
                  <DescriptionListDescription>
                    <a href={portalUrl} target="_blank" rel="noreferrer">
                      {portalUrl}
                    </a>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}
              {lldapWebUIUrl && (
                <DescriptionListGroup>
                  <DescriptionListTerm>LLDAP web UI (WireGuard)</DescriptionListTerm>
                  <DescriptionListDescription>
                    <a href={lldapWebUIUrl} target="_blank" rel="noreferrer">
                      {lldapWebUIUrl}
                    </a>
                    {ingressAuth?.hub?.lldapWebUIPhase
                      ? ` — ${ingressAuth.hub.lldapWebUIPhase}`
                      : ''}
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}
            </DescriptionList>
            {user?.dcAdmin && (
              <div style={{ marginTop: '1rem', maxWidth: '36rem' }}>
                <LabeledField
                  label="Hub ingress forward-auth"
                  fieldId="hubIngressAuth"
                  help={getFieldHelp('infra.hubIngressAuth')}
                >
                  <Checkbox
                    id="hubIngressAuth"
                    label="Hub ingress forward-auth (Authelia)"
                    isChecked={hubAuthEnabled}
                    onChange={(_e, checked) => setHubAuthEnabled(checked)}
                  />
                </LabeledField>
                <LabeledField
                  label="Expose LLDAP Web UI"
                  fieldId="hubLldapWebUI"
                  help={getFieldHelp('infra.hubLldapWebUI')}
                >
                  <Checkbox
                    id="hubLldapWebUI"
                    label="Expose LLDAP Web UI (WireGuard only)"
                    isChecked={hubLldapWebUI}
                    isDisabled={!consoleLldapDeployed}
                    onChange={(_e, checked) => setHubLldapWebUI(checked)}
                  />
                </LabeledField>
                <p style={{ marginTop: '0.35rem', fontSize: '0.875rem', color: 'var(--pf-v5-global--Color--200)' }}>
                  {consoleLldapDeployed
                    ? lldapWebUIUrl
                      ? `Opens fleet-console LLDAP user management at ${lldapWebUIUrl}. Not exposed on the public internet.`
                      : 'Opens fleet-console LLDAP user management over WireGuard. Not exposed on the public internet.'
                    : 'Console LLDAP is not deployed (enable fleet_auth_lldap_enabled on the hub).'}
                </p>
                <Button
                  variant="primary"
                  style={{ marginTop: '0.75rem' }}
                  isLoading={savingAuth}
                  onClick={() => void saveIngressAuth()}
                >
                  Apply infrastructure auth
                </Button>
              </div>
            )}
            <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem' }}>
              Let&apos;s Encrypt (TLS)
            </Title>
            <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
              Hub-routed apps use certificates from Let&apos;s Encrypt on hub Traefik
              ({hub?.hub_zone ? `*.${hub.hub_zone}` : 'app zone'}). Requires public DNS and WAN
              :80/:443 to this host.
            </p>
            <ErrorAlert error={acmeError} />
            <DescriptionList isHorizontal style={{ maxWidth: '48rem', marginTop: '0.75rem' }}>
              <DescriptionListGroup>
                <DescriptionListTerm>Status</DescriptionListTerm>
                <DescriptionListDescription>
                  {acme?.phase || '—'}
                  {acme?.message ? ` — ${acme.message}` : ''}
                </DescriptionListDescription>
              </DescriptionListGroup>
              {acme?.hub?.phase && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Hub gateway</DescriptionListTerm>
                  <DescriptionListDescription>{acme.hub.phase}</DescriptionListDescription>
                </DescriptionListGroup>
              )}
            </DescriptionList>
            {user?.dcAdmin && (
              <div style={{ marginTop: '1rem', maxWidth: '36rem' }}>
                <LabeledField
                  label="Let's Encrypt for hub apps"
                  fieldId="hubAcmeEnabled"
                  help={getFieldHelp('infra.hubAcme')}
                >
                  <Checkbox
                    id="hubAcmeEnabled"
                    label="Enable Let's Encrypt for hub app certificates"
                    isChecked={hubAcmeEnabled}
                    onChange={(_e, checked) => setHubAcmeEnabled(checked)}
                  />
                </LabeledField>
                <Button
                  variant="primary"
                  style={{ marginTop: '0.75rem' }}
                  isLoading={savingAcme}
                  onClick={() => void saveAcme()}
                >
                  Apply Let's Encrypt
                </Button>
              </div>
            )}
            <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem' }}>
              LLM platform
            </Title>
            <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
              Optional hub LiteLLM proxy (OpenAI-compatible API). Inference workloads deploy via the
              <code> llama-cpp-helm </code> catalog on GPU clusters.
              Requires at least one cluster with <code>fleet.capability/gpu=true</code>.
              Public requests use <code>FleetLLMApiKey</code> Bearer auth on{' '}
              <code>https://&lt;public-host&gt;/v1/</code>.
            </p>
            <ErrorAlert error={llmError} />
            <DescriptionList isHorizontal style={{ maxWidth: '48rem', marginTop: '0.75rem' }}>
              <DescriptionListGroup>
                <DescriptionListTerm>Status</DescriptionListTerm>
                <DescriptionListDescription>
                  {llm?.phase || 'Disabled'}
                  {llm?.message ? ` — ${llm.message}` : ''}
                </DescriptionListDescription>
              </DescriptionListGroup>
              {llm?.hub?.endpoint && (
                <DescriptionListGroup>
                  <DescriptionListTerm>API endpoint</DescriptionListTerm>
                  <DescriptionListDescription>
                    <code>{llm.hub.endpoint}</code>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}
              {llm?.publicHost && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Public host</DescriptionListTerm>
                  <DescriptionListDescription>{llm.publicHost}</DescriptionListDescription>
                </DescriptionListGroup>
              )}
            </DescriptionList>
            {user?.dcAdmin && (
              <div style={{ marginTop: '1rem', maxWidth: '36rem' }}>
                <Form>
                  <LabeledField
                    label="Public API domain"
                    fieldId="hubLlmPublicHost"
                    help={getFieldHelp('infra.hubLlmPublicHost')}
                  >
                    <TextInput
                      id="hubLlmPublicHost"
                      type="text"
                      value={hubLlmPublicHost}
                      onChange={(_e, value) => setHubLlmPublicHost(value)}
                      placeholder="llm.example.com"
                      isDisabled={!hubLlmEnabled}
                    />
                  </LabeledField>
                  <LabeledField
                    label="Hub LiteLLM proxy"
                    fieldId="hubLlmEnabled"
                    help={getFieldHelp('infra.hubLlmEnabled')}
                  >
                    <Checkbox
                      id="hubLlmEnabled"
                      label="Enable hub LiteLLM proxy"
                      isChecked={hubLlmEnabled}
                      onChange={(_e, checked) => setHubLlmEnabled(checked)}
                    />
                  </LabeledField>
                </Form>
                <Button
                  variant="primary"
                  style={{ marginTop: '0.75rem' }}
                  isLoading={savingLlm}
                  onClick={() => void saveLlm()}
                >
                  Apply LLM platform
                </Button>
              </div>
            )}

            <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem' }}>
              Services
            </Title>
            <ul style={{ marginTop: '0.5rem' }}>
              {(hub.services || []).map((svc) => (
                <li key={svc.name}>
                  <strong>{svc.name}</strong>
                  {svc.description ? ` — ${svc.description}` : ''}
                </li>
              ))}
            </ul>
            {(hub.nodes || []).length > 0 && (
              <>
                <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem' }}>
                  Hub nodes
                </Title>
                <div style={{ marginTop: '0.5rem' }}>
                  <ClusterNodesPanel nodes={hub.nodes!} />
                </div>
              </>
            )}
          </>
        )}
      </PageSection>
    </>
  );
}
