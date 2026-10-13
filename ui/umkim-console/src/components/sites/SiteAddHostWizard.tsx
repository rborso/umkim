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
  /** Preselect the host kind when opened from a kind-specific action. */
  defaultKind?: SiteHostKind;
};

const KINDS: { value: SiteHostKind; label: string }[] = [
  { value: 'bare_metal', label: 'Bare metal node' },
  { value: 'vm', label: 'VM guest (libvirt)' },
  { value: 'hypervisor', label: 'Hypervisor' },
];

/** Number inputs keep the raw value so typing is never clamped; '' is empty. */
function numValue(raw: number | ''): number {
  return raw === '' ? 0 : raw;
}

/** Parse a NumberInput change; blank stays blank so the field can be cleared. */
function parseNumInput(raw: string): number | '' {
  const t = raw.trim();
  if (t === '') return '';
  const n = Number(t);
  return Number.isFinite(n) ? n : '';
}

/**
 * Guided, kind-aware "add host" wizard: minimal required fields, catalog-driven
 * selects, and discovery (SSH first, PXE fallback) as the next step.
 */
export function SiteAddHostWizard({ siteName, isOpen, onClose, onCreated, defaultKind }: Props) {
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
  const [cores, setCores] = useState<number | ''>(2);
  const [memory, setMemory] = useState<number | ''>(4096);
  const [disk, setDisk] = useState<number | ''>(40);
  const [extraDisks, setExtraDisks] = useState<{ name: string; size: number | '' }[]>([]);
  const [gpu, setGpu] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<{ paths: string[]; warnings: string[] } | null>(null);
  const [nextBusy, setNextBusy] = useState<string | null>(null);
  const [nextMessage, setNextMessage] = useState('');
  const [syncNote, setSyncNote] = useState('');

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
    setExtraDisks([]);
    setGpu(false);
    setError(null);
    setResult(null);
    setNextBusy(null);
    setNextMessage('');
    setSyncNote('');
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    reset();
    if (defaultKind) {
      setKind(defaultKind);
    }
    void getSiteInventoryCatalog(siteName)
      .then(setCatalog)
      .catch(() => setCatalog({}));
  }, [isOpen, siteName, reset, defaultKind]);

  const isVM = kind === 'vm';
  const needsMac = kind !== 'vm';
  // Sizes are validated here rather than clamped while typing, so any number can
  // be entered (and an intermediate empty value is allowed).
  const coresNum = numValue(cores);
  const memoryNum = numValue(memory);
  const diskNum = numValue(disk);
  const coresInvalid = coresNum < 1;
  const memoryInvalid = memoryNum < 512;
  const diskInvalid = diskNum < 10;
  const extraDiskInvalid = extraDisks.some((d) => numValue(d.size) < 1);
  const sizesValid = !isVM || (!coresInvalid && !memoryInvalid && !diskInvalid && !extraDiskInvalid);
  const canSubmit =
    name.trim().length > 0 &&
    (isVM || ip.trim().length > 0 || mac.trim().length > 0) &&
    sizesValid &&
    !saving;

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
        ...(isVM ? { cores: coresNum, memory: memoryNum, diskSizeGi: diskNum } : {}),
        ...(isVM && extraDisks.length > 0
          ? {
              disks: extraDisks.map((d) => ({
                sizeGi: numValue(d.size),
                ...(d.name.trim() ? { name: d.name.trim() } : {}),
              })),
            }
          : {}),
        ...(isVM && gpu ? { gpuPassthrough: true } : {}),
        ...(needsMac && pxe ? { pxe: true } : {}),
      };
      const res = await createSiteHost(siteName, intent);
      setResult({ paths: res.paths ?? [], warnings: res.warnings ?? [] });
      onCreated?.(intent.name);
      // Push the new files so YAIDAP actions on the host work without an extra
      // manual sync step (same behaviour as the other console add flows).
      try {
        await syncSiteInventory(siteName);
        setSyncNote('Synced to the jumphost.');
      } catch {
        setSyncNote('Inventory saved, but the sync to the jumphost failed — use Sync to jumphost.');
      }
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
          <Alert
            variant="success"
            isInline
            title={isVM ? `Declared ${name} in inventory` : `Added ${name} to inventory`}
            style={{ marginBottom: '0.75rem' }}
          >
            <div style={{ fontSize: '0.85rem' }}>
              Wrote: <code>{result.paths.join(', ')}</code>
            </div>
          </Alert>
          {result.warnings.length > 0 ? (
            <Alert variant="warning" isInline title="Placeholders detected" style={{ marginBottom: '0.75rem' }}>
              {result.warnings.join('; ')}
            </Alert>
          ) : null}
          {syncNote ? (
            <p style={{ fontSize: '0.85rem', marginBottom: '0.75rem' }}>{syncNote}</p>
          ) : null}
          <Title headingLevel="h4" size="md">
            Next step
          </Title>
          <p style={{ fontSize: '0.9rem', margin: '0.25rem 0 0.5rem' }}>
            {isVM ? (
              <>
                The guest is declared only — no VM exists yet. Review <code>host_vars/{name.trim()}.yml</code>{' '}
                on the Provisioner inventory tab, then <strong>Provision</strong> to create the libvirt
                domain.
              </>
            ) : (
              <>
                Discover the host to populate host_vars, then promote the drafts via import preview on the
                Provisioner inventory tab. SSH is non-destructive; PXE reimages the host.{' '}
                {kind === 'hypervisor'
                  ? 'Prepare hypervisor runs on the Site hosts tab.'
                  : 'Install bare metal runs on the Site hosts tab.'}
              </>
            )}
          </p>
          <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            {isVM ? (
              <Button
                variant="primary"
                isLoading={nextBusy === 'Provision'}
                onClick={() =>
                  void runNext('Provision', () =>
                    provisionSiteHost(siteName, name.trim(), { action: 'create_vm' }),
                  )
                }
              >
                Provision VM now
              </Button>
            ) : (
              <>
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
              </>
            )}
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
                  onMinus={() => setCores(Math.max(1, coresNum - 1))}
                  onPlus={() => setCores(coresNum + 1)}
                  onChange={(e) => setCores(parseNumInput((e.target as HTMLInputElement).value))}
                  style={{ maxWidth: '10rem' }}
                />
                {coresInvalid ? (
                  <FormHelperText>
                    <span style={{ color: 'var(--pf-v5-global--danger-color--100)' }}>At least 1 vCPU.</span>
                  </FormHelperText>
                ) : null}
              </FormGroup>
              <FormGroup label="Memory (MiB)" fieldId="ah-memory">
                <NumberInput
                  inputName="ah-memory"
                  inputAriaLabel="memory MiB"
                  value={memory}
                  min={512}
                  step={512}
                  onMinus={() => setMemory(Math.max(512, memoryNum - 512))}
                  onPlus={() => setMemory(memoryNum + 512)}
                  onChange={(e) => setMemory(parseNumInput((e.target as HTMLInputElement).value))}
                  style={{ maxWidth: '10rem' }}
                />
                <FormHelperText>
                  {memoryInvalid ? (
                    <span style={{ color: 'var(--pf-v5-global--danger-color--100)' }}>
                      Enter at least 512 MiB.{' '}
                    </span>
                  ) : null}
                  4096 MiB ≈ 4 GiB.
                </FormHelperText>
              </FormGroup>
              <FormGroup label="Boot disk (GiB)" fieldId="ah-disk">
                <NumberInput
                  inputName="ah-disk"
                  inputAriaLabel="disk GiB"
                  value={disk}
                  min={10}
                  step={10}
                  onMinus={() => setDisk(Math.max(10, diskNum - 10))}
                  onPlus={() => setDisk(diskNum + 10)}
                  onChange={(e) => setDisk(parseNumInput((e.target as HTMLInputElement).value))}
                  style={{ maxWidth: '10rem' }}
                />
                {diskInvalid ? (
                  <FormHelperText>
                    <span style={{ color: 'var(--pf-v5-global--danger-color--100)' }}>At least 10 GiB.</span>
                  </FormHelperText>
                ) : null}
              </FormGroup>
              <FormGroup label="Additional disks" fieldId="ah-extra-disks">
                {extraDisks.map((d, i) => (
                  <div
                    key={i}
                    style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', marginBottom: '0.5rem', flexWrap: 'wrap' }}
                  >
                    <NumberInput
                      inputName={`ah-disk-${i}`}
                      inputAriaLabel={`additional disk ${i + 1} size in GiB`}
                      value={d.size}
                      min={1}
                      step={10}
                      onMinus={() =>
                        setExtraDisks((list) =>
                          list.map((x, j) =>
                            j === i ? { ...x, size: Math.max(1, numValue(x.size) - 10) } : x,
                          ),
                        )
                      }
                      onPlus={() =>
                        setExtraDisks((list) =>
                          list.map((x, j) => (j === i ? { ...x, size: numValue(x.size) + 10 } : x)),
                        )
                      }
                      onChange={(e) => {
                        const v = parseNumInput((e.target as HTMLInputElement).value);
                        setExtraDisks((list) => list.map((x, j) => (j === i ? { ...x, size: v } : x)));
                      }}
                      style={{ maxWidth: '9rem' }}
                    />
                    <TextInput
                      id={`ah-disk-name-${i}`}
                      value={d.name}
                      onChange={(_e, v) =>
                        setExtraDisks((list) => list.map((x, j) => (j === i ? { ...x, name: v } : x)))
                      }
                      placeholder={`disk${i + 1} (optional name)`}
                      style={{ maxWidth: '14rem' }}
                    />
                    <Button
                      variant="link"
                      isInline
                      isDanger
                      onClick={() => setExtraDisks((list) => list.filter((_x, j) => j !== i))}
                    >
                      Remove
                    </Button>
                  </div>
                ))}
                <Button
                  variant="secondary"
                  isDisabled={extraDisks.length >= 8}
                  onClick={() => setExtraDisks((list) => [...list, { name: '', size: 100 }])}
                >
                  Add disk
                </Button>
                <FormHelperText>
                  {extraDiskInvalid ? (
                    <span style={{ color: 'var(--pf-v5-global--danger-color--100)' }}>
                      Each data disk needs at least 1 GiB.{' '}
                    </span>
                  ) : null}
                  Data disks follow the boot disk (vdb, vdc, …) and use the site storage pool. Up to 8.
                </FormHelperText>
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
