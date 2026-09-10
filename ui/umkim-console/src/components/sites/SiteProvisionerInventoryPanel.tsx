import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Form,
  FormGroup,
  FormHelperText,
  Label,
  Modal,
  NumberInput,
  TextArea,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  applySiteInventoryImport,
  deleteSiteInventoryFile,
  getSiteInventory,
  getSiteInventoryFileOptional,
  getSiteInventoryImportPreview,
  getSiteInventoryRevision,
  listSiteInventoryFiles,
  listSiteInventoryRevisions,
  previewSiteInventoryImport,
  putSiteInventoryFile,
  syncSiteInventory,
} from '../../api/client';
import type { SiteInventoryFileInfo, SiteInventoryImportChange, SiteInventoryRevision } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import {
  guestHostVarsTemplate,
  guestNameFromHostVarsPath,
  hypervisorDiscoveryStubTemplate,
  mergeGuestIntoGuestsYaml,
  mergeHypervisorIntoHypervisorsYaml,
  templateForNewInventoryFile,
} from './inventoryVmTemplates';

type Props = {
  siteName: string;
};

type TreeNode = {
  name: string;
  path: string;
  isDir: boolean;
  children: TreeNode[];
};

const defaultPaths = [
  'guests.yml',
  'hypervisors.yml',
  'group_vars/all/all.yml',
];

function buildInventoryTree(paths: string[]): TreeNode[] {
  const root: TreeNode = { name: '', path: '', isDir: true, children: [] };
  const sorted = [...paths].sort((a, b) => a.localeCompare(b));
  for (const p of sorted) {
    if (!p || p.endsWith('/')) {
      continue;
    }
    const parts = p.split('/');
    let node = root;
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const isLeaf = i === parts.length - 1;
      const childPath = parts.slice(0, i + 1).join('/');
      let child = node.children.find((c) => c.name === part);
      if (!child) {
        child = {
          name: part,
          path: isLeaf ? p : childPath + '/',
          isDir: !isLeaf,
          children: [],
        };
        node.children.push(child);
      }
      node = child;
    }
  }
  for (const dir of ['group_vars', 'host_vars']) {
    if (!sorted.some((p) => p.startsWith(dir + '/'))) {
      const existing = root.children.find((c) => c.name === dir);
      if (!existing) {
        root.children.push({ name: dir, path: dir + '/', isDir: true, children: [] });
      }
    }
  }
  root.children.sort((a, b) => {
    if (a.isDir !== b.isDir) {
      return a.isDir ? -1 : 1;
    }
    return a.name.localeCompare(b.name);
  });
  const sortChildren = (n: TreeNode) => {
    n.children.sort((a, b) => {
      if (a.isDir !== b.isDir) {
        return a.isDir ? -1 : 1;
      }
      return a.name.localeCompare(b.name);
    });
    n.children.forEach(sortChildren);
  };
  root.children.forEach(sortChildren);
  return root.children;
}

function TreeList({
  nodes,
  depth,
  selectedPath,
  files,
  onSelect,
}: {
  nodes: TreeNode[];
  depth: number;
  selectedPath: string;
  files: SiteInventoryFileInfo[];
  onSelect: (path: string) => void;
}) {
  return (
    <ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>
      {nodes.map((node) => {
        const info = files.find((f) => f.path === node.path);
        const label = node.isDir ? node.name + '/' : node.name;
        return (
          <li key={node.path || node.name} style={{ marginBottom: '0.15rem' }}>
            {node.isDir ? (
              <div style={{ paddingLeft: depth * 12, fontWeight: 600, fontSize: '0.85rem' }}>{label}</div>
            ) : (
              <div style={{ paddingLeft: depth * 12 }}>
                <Button
                  variant={node.path === selectedPath ? 'primary' : 'link'}
                  onClick={() => onSelect(node.path)}
                  style={{ fontSize: '0.85rem' }}
                >
                  {label}
                  {info?.synced === false ? ' (hub)' : ''}
                </Button>
              </div>
            )}
            {node.children.length > 0 && (
              <TreeList
                nodes={node.children}
                depth={depth + 1}
                selectedPath={selectedPath}
                files={files}
                onSelect={onSelect}
              />
            )}
          </li>
        );
      })}
    </ul>
  );
}

export function SiteProvisionerInventoryPanel({ siteName }: Props) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [metaPhase, setMetaPhase] = useState('');
  const [files, setFiles] = useState<SiteInventoryFileInfo[]>([]);
  const [selectedPath, setSelectedPath] = useState('guests.yml');
  const [editorContent, setEditorContent] = useState('');
  const [editorDirty, setEditorDirty] = useState(false);
  const [fileMissing, setFileMissing] = useState(false);
  const [revisions, setRevisions] = useState<SiteInventoryRevision[]>([]);
  const [diffOpen, setDiffOpen] = useState(false);
  const [diffRev, setDiffRev] = useState<SiteInventoryRevision | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [importItems, setImportItems] = useState<SiteInventoryImportChange[]>([]);
  const [importSelected, setImportSelected] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);
  const [newFileOpen, setNewFileOpen] = useState(false);
  const [newFilePath, setNewFilePath] = useState('');
  const [addVmOpen, setAddVmOpen] = useState(false);
  const [addVmStep, setAddVmStep] = useState(1);
  const [vmGuestName, setVmGuestName] = useState('');
  const [vmLocation, setVmLocation] = useState('least-utilized');
  const [vmIp, setVmIp] = useState('');
  const [vmCores, setVmCores] = useState(2);
  const [vmMemory, setVmMemory] = useState(4096);
  const [vmDiskSize, setVmDiskSize] = useState(40);
  const [vmMessage, setVmMessage] = useState('');
  const [addDiscOpen, setAddDiscOpen] = useState(false);
  const [discHostName, setDiscHostName] = useState('');
  const [discAnsibleHost, setDiscAnsibleHost] = useState('');
  const [discMac, setDiscMac] = useState('');
  const [discPxeEnabled, setDiscPxeEnabled] = useState(true);

  const resetAddVmForm = () => {
    setAddVmStep(1);
    setVmGuestName('');
    setVmLocation('least-utilized');
    setVmIp('');
    setVmCores(2);
    setVmMemory(4096);
    setVmDiskSize(40);
  };

  const openAddVmModal = () => {
    resetAddVmForm();
    setAddVmOpen(true);
  };

  const resetAddDiscForm = () => {
    setDiscHostName('');
    setDiscAnsibleHost('');
    setDiscMac('');
    setDiscPxeEnabled(true);
  };

  const openAddDiscModal = () => {
    resetAddDiscForm();
    setAddDiscOpen(true);
  };

  const filePaths = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const p of defaultPaths) {
      if (p.endsWith('/')) {
        continue;
      }
      seen.add(p);
      out.push(p);
    }
    for (const f of files) {
      if (!seen.has(f.path)) {
        seen.add(f.path);
        out.push(f.path);
      }
    }
    return out;
  }, [files]);

  const tree = useMemo(() => buildInventoryTree(filePaths), [filePaths]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [env, fileList, revs] = await Promise.all([
        getSiteInventory(siteName),
        listSiteInventoryFiles(siteName),
        listSiteInventoryRevisions(siteName, 20),
      ]);
      setMetaPhase(env.meta?.syncPhase || 'pending');
      setFiles(fileList);
      setRevisions(revs);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [siteName]);

  const loadFile = useCallback(
    async (path: string) => {
      setBusy(true);
      setError(null);
      try {
        const result = await getSiteInventoryFileOptional(siteName, path);
        setSelectedPath(path);
        setEditorContent(result.content);
        setFileMissing(result.missing);
        setEditorDirty(false);
      } catch (e) {
        setError(e);
      } finally {
        setBusy(false);
      }
    },
    [siteName],
  );

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!loading && selectedPath) {
      void loadFile(selectedPath);
    }
  }, [loading, selectedPath, loadFile]);

  const saveFile = async () => {
    setBusy(true);
    setError(null);
    try {
      await putSiteInventoryFile(siteName, selectedPath, editorContent);
      setEditorDirty(false);
      setFileMissing(false);
      await load();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const removeFile = async () => {
    if (!selectedPath || fileMissing) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await deleteSiteInventoryFile(siteName, selectedPath);
      await load();
      setSelectedPath('guests.yml');
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const createFile = async () => {
    const path = newFilePath.trim();
    if (!path) {
      return;
    }
    setNewFileOpen(false);
    setNewFilePath('');
    setSelectedPath(path);
    const guestFromPath = guestNameFromHostVarsPath(path);
    setEditorContent(
      templateForNewInventoryFile(path) ||
        (guestFromPath ? guestHostVarsTemplate({ guestName: guestFromPath }) : ''),
    );
    setFileMissing(true);
    setEditorDirty(true);
  };

  const addVmGuest = async () => {
    const name = vmGuestName.trim();
    if (!name) {
      setError(new Error('Guest name is required'));
      return;
    }
    setBusy(true);
    setError(null);
    setVmMessage('');
    try {
      const guestsResult = await getSiteInventoryFileOptional(siteName, 'guests.yml');
      const guestsYaml = mergeGuestIntoGuestsYaml(guestsResult.content, name, {
        vm_location: vmLocation.trim() || 'least-utilized',
        ansible_host: vmIp.trim() || undefined,
      });
      const hostVarsPath = `host_vars/${name}.yml`;
      const hostVarsYaml = guestHostVarsTemplate({
        guestName: name,
        vmLocation: vmLocation.trim() || 'least-utilized',
        ip: vmIp.trim() || undefined,
        cores: vmCores,
        memory: vmMemory,
        diskSizeGi: vmDiskSize,
      });
      await putSiteInventoryFile(siteName, 'guests.yml', guestsYaml);
      await putSiteInventoryFile(siteName, hostVarsPath, hostVarsYaml);
      setAddVmOpen(false);
      resetAddVmForm();
      setVmMessage(`Saved guests.yml and ${hostVarsPath}. Sync to jumphost, then provision on the Site hosts tab.`);
      await load();
      setSelectedPath(hostVarsPath);
      setEditorContent(hostVarsYaml);
      setFileMissing(false);
      setEditorDirty(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const runSync = async () => {
    setBusy(true);
    setError(null);
    try {
      await syncSiteInventory(siteName);
      await load();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const addDiscoveryStub = async () => {
    const name = discHostName.trim();
    if (!name) {
      setError(new Error('Host name is required'));
      return;
    }
    if (discPxeEnabled && !discMac.trim()) {
      setError(new Error('MAC address is required for PXE discovery profile'));
      return;
    }
    setBusy(true);
    setError(null);
    setVmMessage('');
    try {
      const hvResult = await getSiteInventoryFileOptional(siteName, 'hypervisors.yml');
      const hypervisorsYaml = mergeHypervisorIntoHypervisorsYaml(hvResult.content, name, {
        ansible_host: discAnsibleHost.trim() || undefined,
      });
      const hostVarsPath = `host_vars/${name}.yml`;
      const hostVarsYaml = hypervisorDiscoveryStubTemplate({
        name,
        ansibleHost: discAnsibleHost.trim() || undefined,
        macAddress: discMac.trim() || undefined,
        pxeDiscovery: discPxeEnabled,
      });
      await putSiteInventoryFile(siteName, 'hypervisors.yml', hypervisorsYaml);
      await putSiteInventoryFile(siteName, hostVarsPath, hostVarsYaml);
      setAddDiscOpen(false);
      resetAddDiscForm();
      setVmMessage(
        `Saved hypervisors.yml and ${hostVarsPath}. Sync to jumphost, then run Discover on the Site hosts tab.`,
      );
      await load();
      setSelectedPath(hostVarsPath);
      setEditorContent(hostVarsYaml);
      setFileMissing(false);
      setEditorDirty(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const runImportPreview = async (filterHost?: string) => {
    setBusy(true);
    setError(null);
    try {
      await previewSiteInventoryImport(siteName);
      for (let i = 0; i < 30; i++) {
        await new Promise((r) => setTimeout(r, 2000));
        const items = await getSiteInventoryImportPreview(siteName);
        if (items.length > 0) {
          const filtered =
            filterHost?.trim()
              ? items.filter(
                  (item) =>
                    item.path === 'hypervisors.yml' ||
                    item.path === `host_vars/${filterHost.trim()}.yml`,
                )
              : items;
          setImportItems(filtered.length > 0 ? filtered : items);
          const sel: Record<string, boolean> = {};
          for (const item of filtered.length > 0 ? filtered : items) {
            if (item.status !== 'unchanged') {
              sel[item.path] = true;
            }
          }
          setImportSelected(sel);
          setImportOpen(true);
          return;
        }
      }
      throw new Error('Timed out waiting for import preview');
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const applyImport = async () => {
    setBusy(true);
    setError(null);
    try {
      const paths = Object.entries(importSelected)
        .filter(([, v]) => v)
        .map(([p]) => p);
      await applySiteInventoryImport(siteName, paths, true);
      setImportOpen(false);
      await load();
      if (selectedPath) {
        await loadFile(selectedPath);
      }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const openDiff = async (rev: SiteInventoryRevision) => {
    setBusy(true);
    try {
      const full = await getSiteInventoryRevision(siteName, rev.id);
      setDiffRev(full);
      setDiffOpen(true);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  if (loading) {
    return <p>Loading provisioner inventory…</p>;
  }

  return (
    <>
      <ErrorAlert error={error} />
      <Title headingLevel="h3">Provisioner inventory (YAIDAP YAML)</Title>
      <p style={{ marginTop: '0.5rem' }}>
        Native Ansible/YAIDAP tree on the hub. Root group files (<code>guests.yml</code>,{' '}
        <code>hypervisors.yml</code>, cluster groups like <code>lab-ha01.yml</code>),{' '}
        <code>group_vars/</code> (one file per group name), and <code>host_vars/</code> sync to the
        jumphost. Hub-only <code>_meta/*</code> is never synced.
      </p>
      <p>
        Sync phase:{' '}
        <Label color={metaPhase === 'synced' ? 'green' : metaPhase === 'dirty' ? 'orange' : metaPhase === 'syncing' ? 'blue' : 'grey'}>
          {metaPhase || 'pending'}
        </Label>
      </p>
      {vmMessage ? (
        <p style={{ fontSize: '0.9rem', color: 'var(--pf-v6-global--Color--200)' }}>{vmMessage}</p>
      ) : null}
      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem', flexWrap: 'wrap' }}>
        <Button variant="primary" isLoading={busy} onClick={() => void runSync()}>
          Sync to jumphost
        </Button>
        <Button variant="secondary" onClick={openAddVmModal}>
          Add VM guest
        </Button>
        <Button variant="secondary" onClick={openAddDiscModal}>
          Add discovery stub
        </Button>
        <Button variant="secondary" isLoading={busy} onClick={() => void runImportPreview()}>
          Import preview
        </Button>
        <Button variant="secondary" onClick={() => setNewFileOpen(true)}>
          New file
        </Button>
        <Button variant="link" onClick={() => void load()}>
          Refresh
        </Button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '16rem 1fr', gap: '1rem' }}>
        <div>
          <Title headingLevel="h4">Inventory tree</Title>
          <TreeList
            nodes={tree}
            depth={0}
            selectedPath={selectedPath}
            files={files}
            onSelect={(path) => {
              if (path !== selectedPath) {
                setSelectedPath(path);
              }
            }}
          />
        </div>
        <div>
          <Title headingLevel="h4">{selectedPath}</Title>
          {fileMissing && (
            <p style={{ fontSize: '0.85rem', color: 'var(--pf-v6-global--Color--200)' }}>
              File not created yet — edit and save to create.
            </p>
          )}
          {editorContent.includes('CHANGE_ME') ? (
            <Alert variant="warning" title="CHANGE_ME placeholders present" isInline style={{ marginBottom: '0.75rem' }}>
              Replace placeholder values before syncing to the jumphost or running provision actions.
            </Alert>
          ) : null}
          <Form style={{ maxWidth: '56rem' }}>
            <TextArea
              value={editorContent}
              onChange={(_e, v) => {
                setEditorContent(v);
                setEditorDirty(true);
              }}
              rows={18}
              resizeOrientation="vertical"
            />
            <div style={{ marginTop: '0.5rem', display: 'flex', gap: '0.5rem' }}>
              <Button variant="primary" isLoading={busy} isDisabled={!editorDirty} onClick={() => void saveFile()}>
                Save file
              </Button>
              <Button variant="danger" isLoading={busy} isDisabled={fileMissing} onClick={() => void removeFile()}>
                Delete file
              </Button>
            </div>
          </Form>
        </div>
      </div>

      <Title headingLevel="h4" style={{ marginTop: '1.5rem' }}>
        Revisions
      </Title>
      <Table variant="compact">
        <Thead>
          <Tr>
            <Th>Time</Th>
            <Th>Actor</Th>
            <Th>Summary</Th>
            <Th />
          </Tr>
        </Thead>
        <Tbody>
          {revisions.map((r) => (
            <Tr key={r.id}>
              <Td>{r.ts || r.id}</Td>
              <Td>{r.actor || '—'}</Td>
              <Td>{r.summary || r.scope || '—'}</Td>
              <Td>
                <Button variant="link" onClick={() => void openDiff(r)}>
                  Diff
                </Button>
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>

      <Modal title="New inventory file" isOpen={newFileOpen} onClose={() => setNewFileOpen(false)}>
        <Form>
          <TextInput
            value={newFilePath}
            onChange={(_e, v) => setNewFilePath(v)}
            placeholder="e.g. host_vars/my-guest.yml, lab-ha01.yml, or group_vars/lab-ha01.yml"
          />
          <p style={{ fontSize: '0.85rem', marginTop: '0.5rem' }}>
            <code>host_vars/*.yml</code> and cluster paths open with a starter template.
          </p>
        </Form>
        <div style={{ marginTop: '1rem', display: 'flex', gap: '0.5rem' }}>
          <Button variant="primary" onClick={() => void createFile()}>
            Create
          </Button>
          <Button variant="link" onClick={() => setNewFileOpen(false)}>
            Cancel
          </Button>
        </div>
      </Modal>

      <Modal
        title={`Add VM guest — step ${addVmStep} of 2`}
        isOpen={addVmOpen}
        onClose={() => {
          setAddVmOpen(false);
          resetAddVmForm();
        }}
      >
        {addVmStep === 1 ? (
          <>
            <p style={{ marginBottom: '1rem', fontSize: '0.9rem' }}>
              Guest identity and placement. VM sizing is on the next step.
            </p>
            <Form>
              <FormGroup label="Guest name" isRequired fieldId="vmGuestName">
                <TextInput
                  id="vmGuestName"
                  value={vmGuestName}
                  onChange={(_e, v) => setVmGuestName(v)}
                  placeholder="e.g. lab-single-1-0"
                />
              </FormGroup>
              <FormGroup label="VM location (hypervisor)" fieldId="vmLocation">
                <TextInput
                  id="vmLocation"
                  value={vmLocation}
                  onChange={(_e, v) => setVmLocation(v)}
                  placeholder="least-utilized or virt01"
                />
              </FormGroup>
              <FormGroup label="ansible_host / static IP (optional)" fieldId="vmIp">
                <TextInput
                  id="vmIp"
                  value={vmIp}
                  onChange={(_e, v) => setVmIp(v)}
                  placeholder="172.16.10.161"
                />
                <FormHelperText>
                  Leave empty for DHCP in host_vars; set if the address is already known.
                </FormHelperText>
              </FormGroup>
            </Form>
            <div style={{ marginTop: '1rem', display: 'flex', gap: '0.5rem' }}>
              <Button
                variant="primary"
                onClick={() => {
                  if (!vmGuestName.trim()) {
                    setError(new Error('Guest name is required'));
                    return;
                  }
                  setError(null);
                  setAddVmStep(2);
                }}
              >
                Next
              </Button>
              <Button
                variant="link"
                onClick={() => {
                  setAddVmOpen(false);
                  resetAddVmForm();
                }}
              >
                Cancel
              </Button>
            </div>
          </>
        ) : (
          <>
            <p style={{ marginBottom: '1rem', fontSize: '0.9rem' }}>
              Common VM settings for <code>{vmGuestName.trim()}</code>. Uplink network defaults stay in the
              generated <code>host_vars</code> file — edit there if your site uses different zones or bridges.
            </p>
            <Form>
              <FormGroup label="CPU cores" fieldId="vmCores">
                <NumberInput
                  inputName="vmCores"
                  inputAriaLabel="CPU cores"
                  value={vmCores}
                  min={1}
                  max={64}
                  onChange={(event) => {
                    const n = Number((event.target as HTMLInputElement).value);
                    if (!Number.isNaN(n)) {
                      setVmCores(Math.max(1, Math.min(64, n)));
                    }
                  }}
                  onMinus={() => setVmCores((v) => Math.max(1, v - 1))}
                  onPlus={() => setVmCores((v) => Math.min(64, v + 1))}
                  style={{ maxWidth: '10rem' }}
                />
              </FormGroup>
              <FormGroup label="Memory (MiB)" fieldId="vmMemory">
                <NumberInput
                  inputName="vmMemory"
                  inputAriaLabel="Memory in MiB"
                  value={vmMemory}
                  min={512}
                  step={512}
                  onChange={(event) => {
                    const n = Number((event.target as HTMLInputElement).value);
                    if (!Number.isNaN(n)) {
                      setVmMemory(Math.max(512, n));
                    }
                  }}
                  onMinus={() => setVmMemory((v) => Math.max(512, v - 512))}
                  onPlus={() => setVmMemory((v) => v + 512)}
                  style={{ maxWidth: '10rem' }}
                />
                <FormHelperText>4096 MiB ≈ 4 GiB.</FormHelperText>
              </FormGroup>
              <FormGroup label="Boot disk size (GiB)" fieldId="vmDiskSize">
                <NumberInput
                  inputName="vmDiskSize"
                  inputAriaLabel="Boot disk size in GiB"
                  value={vmDiskSize}
                  min={10}
                  step={10}
                  onChange={(event) => {
                    const n = Number((event.target as HTMLInputElement).value);
                    if (!Number.isNaN(n)) {
                      setVmDiskSize(Math.max(10, n));
                    }
                  }}
                  onMinus={() => setVmDiskSize((v) => Math.max(10, v - 10))}
                  onPlus={() => setVmDiskSize((v) => v + 10)}
                  style={{ maxWidth: '10rem' }}
                />
              </FormGroup>
            </Form>
            <div style={{ marginTop: '1rem', display: 'flex', gap: '0.5rem' }}>
              <Button variant="primary" isLoading={busy} onClick={() => void addVmGuest()}>
                Save guest files
              </Button>
              <Button variant="secondary" onClick={() => setAddVmStep(1)}>
                Back
              </Button>
              <Button
                variant="link"
                onClick={() => {
                  setAddVmOpen(false);
                  resetAddVmForm();
                }}
              >
                Cancel
              </Button>
            </div>
          </>
        )}
      </Modal>

      <Modal title="Add discovery stub" isOpen={addDiscOpen} onClose={() => setAddDiscOpen(false)}>
        <p style={{ marginBottom: '1rem', fontSize: '0.9rem' }}>
          Creates a minimal hypervisor entry for YAIDAP hardware discovery (SSH or TempXE PXE).
        </p>
        <Form>
          <FormGroup label="Host name" isRequired fieldId="discHostName">
            <TextInput
              id="discHostName"
              value={discHostName}
              onChange={(_e, v) => setDiscHostName(v)}
              placeholder="e.g. new-hv01"
            />
          </FormGroup>
          <FormGroup label="ansible_host (optional for PXE-only)" fieldId="discAnsibleHost">
            <TextInput
              id="discAnsibleHost"
              value={discAnsibleHost}
              onChange={(_e, v) => setDiscAnsibleHost(v)}
              placeholder="172.16.10.99"
            />
            <FormHelperText>Required for SSH discovery; optional when using PXE-only workflow.</FormHelperText>
          </FormGroup>
          <FormGroup label="MAC address" fieldId="discMac">
            <TextInput
              id="discMac"
              value={discMac}
              onChange={(_e, v) => setDiscMac(v)}
              placeholder="aa:bb:cc:dd:ee:ff"
            />
            <FormHelperText>Required for PXE discovery archive matching.</FormHelperText>
          </FormGroup>
          <FormGroup label="PXE discovery profile" fieldId="discPxeEnabled">
            <Checkbox
              id="discPxeEnabled"
              isChecked={discPxeEnabled}
              onChange={(_e, c) => setDiscPxeEnabled(c)}
              label="Enable tempxe_discovery in host_vars (required for Discover PXE)"
            />
          </FormGroup>
        </Form>
        <div style={{ marginTop: '1rem', display: 'flex', gap: '0.5rem' }}>
          <Button variant="primary" isLoading={busy} onClick={() => void addDiscoveryStub()}>
            Save stub files
          </Button>
          <Button variant="link" onClick={() => setAddDiscOpen(false)}>
            Cancel
          </Button>
        </div>
      </Modal>

      <Modal title="Import preview" isOpen={importOpen} onClose={() => setImportOpen(false)} variant="large">
        <p>Select jumphost files to merge into the hub store. Hub-only files are never overwritten.</p>
        {importItems.some((item) => (item.warnings?.length ?? 0) > 0) ? (
          <Alert variant="warning" title="CHANGE_ME placeholders detected" isInline style={{ marginBottom: '1rem' }}>
            Resolve placeholder fields before sync or the next YAIDAP/ansible run.
          </Alert>
        ) : null}
        <Table variant="compact">
          <Thead>
            <Tr>
              <Th>Apply</Th>
              <Th>Path</Th>
              <Th>Status</Th>
              <Th>Warnings</Th>
            </Tr>
          </Thead>
          <Tbody>
            {importItems.map((item) => (
              <Tr key={item.path}>
                <Td>
                  <Checkbox
                    id={'imp-' + item.path}
                    isChecked={!!importSelected[item.path]}
                    isDisabled={item.status === 'unchanged'}
                    onChange={(_e, checked) =>
                      setImportSelected((prev) => ({ ...prev, [item.path]: checked }))
                    }
                  />
                </Td>
                <Td>{item.path}</Td>
                <Td>{item.status}</Td>
                <Td>
                  {(item.warnings ?? []).length > 0 ? (
                    <ul style={{ margin: 0, paddingLeft: '1.1rem', fontSize: '0.85rem' }}>
                      {(item.warnings ?? []).map((w) => (
                        <li key={w}>{w}</li>
                      ))}
                    </ul>
                  ) : (
                    '—'
                  )}
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
        {importItems.map(
          (item) =>
            item.diff && importSelected[item.path] ? (
              <div key={item.path + '-diff'} style={{ marginTop: '1rem' }}>
                <Title headingLevel="h5">{item.path}</Title>
                <pre style={{ fontSize: '0.75rem', maxHeight: '12rem', overflow: 'auto' }}>{item.diff}</pre>
              </div>
            ) : null,
        )}
        <div style={{ marginTop: '1rem', display: 'flex', gap: '0.5rem' }}>
          <Button variant="primary" isLoading={busy} onClick={() => void applyImport()}>
            Apply selected
          </Button>
          <Button variant="link" onClick={() => setImportOpen(false)}>
            Cancel
          </Button>
        </div>
      </Modal>

      <Modal title="Revision diff" isOpen={diffOpen} onClose={() => setDiffOpen(false)} variant="medium">
        {diffRev && (
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
            <div>
              <Title headingLevel="h5">Before</Title>
              <pre style={{ fontSize: '0.75rem', maxHeight: '24rem', overflow: 'auto' }}>
                {JSON.stringify(diffRev.before ?? null, null, 2)}
              </pre>
            </div>
            <div>
              <Title headingLevel="h5">After</Title>
              <pre style={{ fontSize: '0.75rem', maxHeight: '24rem', overflow: 'auto' }}>
                {JSON.stringify(diffRev.after ?? null, null, 2)}
              </pre>
            </div>
          </div>
        )}
      </Modal>
    </>
  );
}
