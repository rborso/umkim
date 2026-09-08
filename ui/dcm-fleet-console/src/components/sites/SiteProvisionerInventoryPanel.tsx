import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Checkbox,
  Form,
  Label,
  Modal,
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
      setFiles(fileList.length ? fileList : env.files ?? []);
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
    setEditorContent('');
    setFileMissing(true);
    setEditorDirty(true);
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

  const runImportPreview = async () => {
    setBusy(true);
    setError(null);
    try {
      await previewSiteInventoryImport(siteName);
      for (let i = 0; i < 30; i++) {
        await new Promise((r) => setTimeout(r, 2000));
        const items = await getSiteInventoryImportPreview(siteName);
        if (items.length > 0) {
          setImportItems(items);
          const sel: Record<string, boolean> = {};
          for (const item of items) {
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
      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem', flexWrap: 'wrap' }}>
        <Button variant="primary" isLoading={busy} onClick={() => void runSync()}>
          Sync to jumphost
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

      <Modal title="Import preview" isOpen={importOpen} onClose={() => setImportOpen(false)} variant="large">
        <p>Select jumphost files to merge into the hub store. Hub-only files are never overwritten.</p>
        <Table variant="compact">
          <Thead>
            <Tr>
              <Th>Apply</Th>
              <Th>Path</Th>
              <Th>Status</Th>
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
