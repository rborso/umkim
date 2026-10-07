import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Form,
  FormGroup,
  FormHelperText,
  FormSelect,
  FormSelectOption,
  Modal,
  NumberInput,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { createSiteHost, getSiteInventoryCatalog, provisionSiteHost, syncSiteInventory } from '../../api/client';
import type { SiteHostIntent, SiteHostKind, SiteInventoryCatalog } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';

type Props = {
  siteName: string;
  isOpen: boolean;
  onClose: () => void;
  onCreated?: (name: string) => void;
};

const KINDS: { value: SiteHostKind; label: string }[] = [
  { value: 'bare_metal', label: 'Bare metal node' },
  { value: 'vm', label: 'VM guest (libvirt)' },
  { value: 'hypervisor', label: 'Hypervisor' },
];

/**
 * Guided, kind-aware "add host" wizard: minimal required fields, catalog-driven
 * selects, and discovery (SSH first, PXE fallback) as the next step.
 */
export function SiteAddHostWizard({ siteName, isOpen, onClose, onCreated }: Props) {
  const [catalog, setCatalog] = useState<SiteInventoryCatalog>({});
  const [kind, setKind] = useState<SiteHostKind>('bare_metal');
  const [name, setName] = useState('');
  const [ip, setIp] = useState('');
  const [mac, setMac] = useState('');
  const [pxe, setPxe] = useState(false);
  const [hypervisor, setHypervisor] = useState('');
  const [network, setNetwork] = useState('');
  const [zone, setZone] = useState('');
  const [storage, setStorage] = useState('');
  const [os, setOs] = useState('');
  const [cluster, setCluster] = useState('');
  const [cores, setCores] = useState(2);
  const [memory, setMemory] = useState(4096);
  const [disk, setDisk] = useState(40);
  const [gpu, setGpu] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<{ paths: string[]; warnings: string[] } | null>(null);
  const [nextBusy, setNextBusy] = useState<string | null>(null);
  const [nextMessage, setNextMessage] = useState('');

  const reset = useCallback(() => {
    setKind('bare_metal');
    setName('');
    setIp('');
    setMac('');
    setPxe(false);
    setHypervisor('');
    setNetwork('');
    setZone('');
    setStorage('');
    setOs('');
    setCluster('');
    setCores(2);
    setMemory(4096);
    setDisk(40);
    setGpu(false);
    setError(null);
    setResult(null);
    setNextBusy(null);
    setNextMessage('');
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    reset();
    void getSiteInventoryCatalog(siteName)
      .then(setCatalog)
      .catch(() => setCatalog({}));
  }, [isOpen, siteName, reset]);

  const isVM = kind === 'vm';
  const needsMac = kind !== 'vm';
  const canSubmit = name.trim().length > 0 && (isVM || ip.trim().length > 0 || mac.trim().length > 0) && !saving;

  const defaults = catalog.defaults ?? {};
  const networkOptions = useMemo(
    () => [
      { value: '', label: defaults.network_name ? `default (${defaults.network_name})` : 'default' },
      ...(catalog.networks ?? []).map((n) => ({
        value: n.name,
        label: n.zone ? `${n.name} (${n.zone})` : n.name,
      })),
    ],
    [catalog.networks, defaults.network_name],
  );
  const storageOptions = useMemo(
    () => [
      { value: '', label: defaults.storage ? `default (${defaults.storage})` : 'default' },
      ...(catalog.storages ?? []).map((s) => ({ value: s.name, label: s.name })),
    ],
    [catalog.storages, defaults.storage],
  );

  const submit = async () => {
    setSaving(true);
    setError(null);
    setNextMessage('');
    try {
      const intent: SiteHostIntent = {
        kind,
        name: name.trim(),
        ...(ip.trim() ? { ansibleHost: ip.trim() } : {}),
        ...(mac.trim() ? { macAddress: mac.trim() } : {}),
        ...(isVM && hypervisor.trim() ? { hypervisor: hypervisor.trim() } : {}),
        ...(network ? { networkName: network } : {}),
        ...(zone.trim() ? { zone: zone.trim() } : {}),
        ...(storage ? { storage } : {}),
        ...(os.trim() ? { os: os.trim() } : {}),
        ...(cluster.trim() ? { cluster: cluster.trim() } : {}),
        ...(isVM ? { cores, memory, diskSizeGi: disk } : {}),
        ...(isVM && gpu ? { gpuPassthrough: true } : {}),
        ...(needsMac && pxe ? { pxe: true } : {}),
      };
      const res = await createSiteHost(siteName, intent);
      setResult({ paths: res.paths ?? [], warnings: res.warnings ?? [] });
      onCreated?.(intent.name);
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  const runNext = async (label: string, fn: () => Promise<unknown>) => {
    setNextBusy(label);
    setNextMessage('');
    setError(null);
    try {
      await fn();
      setNextMessage(`${label} started. Track it on the Site hosts tab.`);
    } catch (e) {
      setError(e);
    } finally {
      setNextBusy(null);
    }
  };

  return (
    <Modal
      variant="medium"
      title="Add host"
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        <Button key="add" variant="primary" onClick={() => void submit()} isDisabled={!canSubmit} isLoading={saving}>
          Add host
        </Button>,
        <Button key="close" variant="link" onClick={onClose}>
          Close
        </Button>,
      ]}
    >
      <ErrorAlert error={error} />
      {result ? (
        <>
          <Alert variant="success" isInline title={`Added ${name} to inventory`} style={{ marginBottom: '0.75rem' }}>
            <div style={{ fontSize: '0.85rem' }}>
              Wrote: <code>{result.paths.join(', ')}</code>
            </div>
          </Alert>
          {result.warnings.length > 0 ? (
            <Alert variant="warning" isInline title="Placeholders detected" style={{ marginBottom: '0.75rem' }}>
              {result.warnings.join('; ')}
            </Alert>
          ) : null}
          <Title headingLevel="h4" size="md">
            Next step
          </Title>
          <p style={{ fontSize: '0.9rem', margin: '0.25rem 0 0.5rem' }}>
            Discover the host to populate host_vars, then promote the drafts via import preview on the
            Provisioner inventory tab. SSH is non-destructive; PXE reimages the host.
          </p>
          <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            <Button
              variant="primary"
              isLoading={nextBusy === 'Discover (SSH)'}
              onClick={() =>
                void runNext('Discover (SSH)', () =>
                  provisionSiteHost(siteName, name.trim(), { action: 'discover' }),
                )
              }
            >
              Discover (SSH)
            </Button>
            {needsMac ? (
              <Button
                variant="secondary"
                isLoading={nextBusy === 'Discover (PXE)'}
                onClick={() =>
                  void runNext('Discover (PXE)', () =>
                    provisionSiteHost(siteName, name.trim(), { action: 'discover_pxe', confirm: true }),
                  )
                }
              >
                Discover (PXE)
              </Button>
            ) : null}
            <Button
              variant="secondary"
              isLoading={nextBusy === 'Sync'}
              onClick={() => void runNext('Sync', () => syncSiteInventory(siteName))}
            >
              Sync to jumphost
            </Button>
          </div>
          {nextMessage ? (
            <p style={{ marginTop: '0.5rem', fontSize: '0.9rem' }}>{nextMessage}</p>
          ) : null}
        </>
      ) : (
        <Form>
          <FormGroup label="Host kind" fieldId="ah-kind">
            <FormSelect id="ah-kind" value={kind} onChange={(_e, v) => setKind(v as SiteHostKind)}>
              {KINDS.map((k) => (
                <FormSelectOption key={k.value} value={k.value} label={k.label} />
              ))}
            </FormSelect>
          </FormGroup>
          <FormGroup label="Name" isRequired fieldId="ah-name">
            <TextInput id="ah-name" value={name} onChange={(_e, v) => setName(v)} placeholder="bm01" />
          </FormGroup>

          {needsMac ? (
            <>
              <FormGroup label="Management IP (SSH)" fieldId="ah-ip">
                <TextInput
                  id="ah-ip"
                  value={ip}
                  onChange={(_e, v) => setIp(v)}
                  placeholder="172.16.10.50"
                />
                <FormHelperText>Leave empty if the host has no OS yet (use PXE below).</FormHelperText>
              </FormGroup>
              <FormGroup label="MAC address (PXE)" fieldId="ah-mac">
                <TextInput
                  id="ah-mac"
                  value={mac}
                  onChange={(_e, v) => setMac(v)}
                  placeholder="aa:bb:cc:dd:ee:ff"
                />
              </FormGroup>
              <FormGroup fieldId="ah-pxe">
                <Checkbox
                  id="ah-pxe"
                  isChecked={pxe}
                  onChange={(_e, c) => setPxe(c)}
                  label="PXE host (arm tempxe_discovery; reimage on install)"
                />
              </FormGroup>
            </>
          ) : (
            <>
              <FormGroup label="Hypervisor" fieldId="ah-hv">
                <TextInput
                  id="ah-hv"
                  value={hypervisor}
                  onChange={(_e, v) => setHypervisor(v)}
                  placeholder="virt04 (empty = least-utilized)"
                />
              </FormGroup>
              <FormGroup label="vCPU cores" fieldId="ah-cores">
                <NumberInput
                  inputName="ah-cores"
                  inputAriaLabel="vCPU cores"
                  value={cores}
                  min={1}
                  onMinus={() => setCores((v) => Math.max(1, v - 1))}
                  onPlus={() => setCores((v) => v + 1)}
                  onChange={(e) => {
                    const n = Number((e.target as HTMLInputElement).value);
                    if (!Number.isNaN(n)) setCores(Math.max(1, n));
                  }}
                  style={{ maxWidth: '10rem' }}
                />
              </FormGroup>
              <FormGroup label="Memory (MiB)" fieldId="ah-memory">
                <NumberInput
                  inputName="ah-memory"
                  inputAriaLabel="memory MiB"
                  value={memory}
                  min={512}
                  step={512}
                  onMinus={() => setMemory((v) => Math.max(512, v - 512))}
                  onPlus={() => setMemory((v) => v + 512)}
                  onChange={(e) => {
                    const n = Number((e.target as HTMLInputElement).value);
                    if (!Number.isNaN(n)) setMemory(Math.max(512, n));
                  }}
                  style={{ maxWidth: '10rem' }}
                />
              </FormGroup>
              <FormGroup label="Boot disk (GiB)" fieldId="ah-disk">
                <NumberInput
                  inputName="ah-disk"
                  inputAriaLabel="disk GiB"
                  value={disk}
                  min={10}
                  step={10}
                  onMinus={() => setDisk((v) => Math.max(10, v - 10))}
                  onPlus={() => setDisk((v) => v + 10)}
                  onChange={(e) => {
                    const n = Number((e.target as HTMLInputElement).value);
                    if (!Number.isNaN(n)) setDisk(Math.max(10, n));
                  }}
                  style={{ maxWidth: '10rem' }}
                />
              </FormGroup>
              <FormGroup fieldId="ah-gpu">
                <Checkbox id="ah-gpu" isChecked={gpu} onChange={(_e, c) => setGpu(c)} label="GPU passthrough guest" />
              </FormGroup>
            </>
          )}

          <FormGroup label="Network" fieldId="ah-network">
            <FormSelect id="ah-network" value={network} onChange={(_e, v) => setNetwork(v)}>
              {networkOptions.map((o) => (
                <FormSelectOption key={o.value || 'default'} value={o.value} label={o.label} />
              ))}
            </FormSelect>
          </FormGroup>
          {isVM ? (
            <FormGroup label="Storage" fieldId="ah-storage">
              <FormSelect id="ah-storage" value={storage} onChange={(_e, v) => setStorage(v)}>
                {storageOptions.map((o) => (
                  <FormSelectOption key={o.value || 'default'} value={o.value} label={o.label} />
                ))}
              </FormSelect>
            </FormGroup>
          ) : null}
          <FormGroup label="Zone (optional)" fieldId="ah-zone">
            <TextInput id="ah-zone" value={zone} onChange={(_e, v) => setZone(v)} placeholder="lab" />
          </FormGroup>
          <FormGroup label="OS (optional)" fieldId="ah-os">
            <TextInput id="ah-os" value={os} onChange={(_e, v) => setOs(v)} placeholder="rocky-10" />
          </FormGroup>
          <FormGroup label="Join cluster (optional)" fieldId="ah-cluster">
            <TextInput id="ah-cluster" value={cluster} onChange={(_e, v) => setCluster(v)} placeholder="lab-ha-0" />
          </FormGroup>
        </Form>
      )}
    </Modal>
  );
}
