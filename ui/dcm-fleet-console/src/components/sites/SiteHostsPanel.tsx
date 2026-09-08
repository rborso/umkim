import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Button,
  Checkbox,
  Form,
  FormSelect,
  FormSelectOption,
  Label,
  Modal,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  applySiteHostCluster,
  createSiteHost,
  deleteSiteHost,
  listFleetClusters,
  listSiteHosts,
  patchSiteHost,
  provisionSiteHost,
} from '../../api/client';
import type {
  FleetCluster,
  SiteHostClusterIntent,
  SiteHostClusterMode,
  SiteHostKind,
  SiteHostRecord,
} from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import { LabeledField } from '../forms/LabeledField';
import { SiteProvisionHistory } from './SiteProvisionHistory';

type Props = {
  siteName: string;
};

type ClusterFormMode = '' | SiteHostClusterMode;

function phaseColor(phase?: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Declared':
      return 'blue';
    case 'Provisioning':
    case 'Applying':
    case 'WaitingGroup':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

const emptyForm = () => ({
  name: '',
  kind: 'vm' as SiteHostKind,
  ansible_host: '',
  ansible_user: 'linux',
  osFamily: 'rocky',
  vm_location: 'least-utilized',
  clusterMode: '' as ClusterFormMode,
  clusterName: '',
  targetCluster: '',
  topology: 'single_node' as 'single_node' | 'ha_3_node',
  haGroup: '',
  replaceNode: '',
  controlPlaneVip: '',
  provisionNow: false,
});

function defaultClusterName(hostName: string): string {
  const base = hostName.trim();
  return base ? `${base}-0` : '';
}

function buildClusterIntent(form: ReturnType<typeof emptyForm>): SiteHostClusterIntent | undefined {
  if (!form.clusterMode) return undefined;
  return {
    mode: form.clusterMode,
    clusterName: form.clusterMode === 'enroll' ? form.clusterName.trim() || undefined : undefined,
    targetCluster:
      form.clusterMode === 'add_worker' || form.clusterMode === 'replace_control'
        ? form.targetCluster.trim() || undefined
        : undefined,
    topology: form.clusterMode === 'enroll' ? form.topology : undefined,
    haGroup: form.clusterMode === 'enroll' && form.topology === 'ha_3_node' ? form.haGroup.trim() || undefined : undefined,
    replaceNode: form.clusterMode === 'replace_control' ? form.replaceNode.trim() || undefined : undefined,
    controlPlaneVip:
      form.clusterMode === 'enroll' && form.topology === 'ha_3_node'
        ? form.controlPlaneVip.trim() || undefined
        : undefined,
    autoApply: true,
  };
}

function formFromHost(host: SiteHostRecord): ReturnType<typeof emptyForm> {
  const intent = host.clusterIntent;
  return {
    name: host.name,
    kind: host.kind || 'vm',
    ansible_host: host.ansible_host || '',
    ansible_user: host.ansible_user || 'linux',
    osFamily: host.osFamily || 'rocky',
    vm_location: host.vm_location || '',
    clusterMode: (intent?.mode as ClusterFormMode) || '',
    clusterName: intent?.clusterName || '',
    targetCluster: intent?.targetCluster || '',
    topology: intent?.topology || 'single_node',
    haGroup: intent?.haGroup || '',
    replaceNode: intent?.replaceNode || '',
    controlPlaneVip: intent?.controlPlaneVip || '',
    provisionNow: false,
  };
}

export function SiteHostsPanel({ siteName }: Props) {
  const [hosts, setHosts] = useState<SiteHostRecord[]>([]);
  const [clusters, setClusters] = useState<FleetCluster[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<SiteHostRecord | null>(null);
  const [form, setForm] = useState(emptyForm());
  const [saving, setSaving] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<SiteHostRecord | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [provisionOpen, setProvisionOpen] = useState(false);
  const [provisionTarget, setProvisionTarget] = useState<SiteHostRecord | null>(null);
  const [provisionAction, setProvisionAction] = useState<'create_vm' | 'plan_vm'>('create_vm');
  const [provisionConfirm, setProvisionConfirm] = useState(false);
  const [provisioning, setProvisioning] = useState(false);
  const [applyingCluster, setApplyingCluster] = useState<string | null>(null);
  const [historyRefreshKey, setHistoryRefreshKey] = useState('0');

  const siteClusters = useMemo(
    () => clusters.filter((c) => (c.site || '') === siteName && c.console_managed),
    [clusters, siteName],
  );

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [hostList, clusterList] = await Promise.all([listSiteHosts(siteName), listFleetClusters()]);
      setHosts(hostList);
      setClusters(clusterList);
      setHistoryRefreshKey(String(Date.now()));
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [siteName]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const active = hosts.some(
      (h) =>
        h.status?.phase === 'Provisioning' ||
        h.status?.cluster?.phase === 'Applying' ||
        h.status?.cluster?.phase === 'WaitingGroup' ||
        h.status?.cluster?.phase === 'Pending',
    );
    if (!active) return;
    const t = setInterval(() => void load(), 5000);
    return () => clearInterval(t);
  }, [hosts, load]);

  const openAdd = () => {
    setEditing(null);
    setForm(emptyForm());
    setActionError(null);
    setEditorOpen(true);
  };

  const openEdit = (host: SiteHostRecord) => {
    setEditing(host);
    setForm(formFromHost(host));
    setActionError(null);
    setEditorOpen(true);
  };

  const saveHost = async () => {
    setSaving(true);
    setActionError(null);
    try {
      const clusterIntent = buildClusterIntent(form);
      if (editing) {
        await patchSiteHost(siteName, editing.name, {
          kind: form.kind,
          ansible_host: form.ansible_host.trim() || undefined,
          ansible_user: form.ansible_user.trim() || undefined,
          osFamily: form.osFamily.trim() || undefined,
          vm_location: form.vm_location.trim() || undefined,
          clusterIntent: clusterIntent ?? (form.clusterMode ? clusterIntent : undefined),
        });
      } else {
        await createSiteHost(siteName, {
          name: form.name.trim(),
          kind: form.kind,
          ansible_host: form.ansible_host.trim() || undefined,
          ansible_user: form.ansible_user.trim() || undefined,
          osFamily: form.osFamily.trim() || undefined,
          vm_location: form.vm_location.trim() || undefined,
          clusterIntent,
          provisionNow: form.provisionNow,
        });
      }
      setEditorOpen(false);
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    setActionError(null);
    try {
      await deleteSiteHost(siteName, deleteTarget.name);
      setDeleteOpen(false);
      setDeleteTarget(null);
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setDeleting(false);
    }
  };

  const runProvision = async () => {
    if (!provisionTarget) return;
    setProvisioning(true);
    setActionError(null);
    try {
      await provisionSiteHost(siteName, provisionTarget.name, {
        action: provisionAction,
        confirm: provisionConfirm,
      });
      setProvisionOpen(false);
      setProvisionTarget(null);
      setProvisionConfirm(false);
      setProvisionAction('create_vm');
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setProvisioning(false);
    }
  };

  const openProvision = (host: SiteHostRecord, action: 'create_vm' | 'plan_vm' = 'create_vm') => {
    setProvisionTarget(host);
    setProvisionAction(action);
    setProvisionConfirm(false);
    setProvisionOpen(true);
  };

  const runClusterApply = async (host: SiteHostRecord) => {
    setApplyingCluster(host.name);
    setActionError(null);
    try {
      await applySiteHostCluster(siteName, host.name);
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setApplyingCluster(null);
    }
  };

  const needsConfirm =
    provisionAction !== 'plan_vm' &&
    (provisionTarget?.kind === 'bare_metal' || provisionTarget?.status?.phase === 'Ready');

  const canEnrollNow = (host: SiteHostRecord) =>
    host.status?.phase === 'Ready' &&
    (host.ansible_host || host.status?.ansible_host) &&
    (!host.status?.cluster?.phase ||
      host.status.cluster.phase === 'Failed' ||
      host.status.cluster.phase === 'Pending');

  return (
    <div style={{ marginTop: '1rem' }}>
      <ErrorAlert error={error} />
      <ErrorAlert error={actionError} />
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.75rem' }}>
        <Title headingLevel="h3" style={{ margin: 0 }}>
          Site host inventory
        </Title>
        <Button variant="primary" onClick={openAdd} data-testid="site-hosts-add">
          Add host
        </Button>
      </div>
      <p style={{ fontSize: '0.85rem', marginBottom: '0.75rem' }}>
        Hub-owned host intent for provisioner connectors. Optionally enroll as a cluster, add to an
        existing cluster, or replace a control-plane node after provision completes.
      </p>
      {loading ? (
        <p>Loading hosts…</p>
      ) : hosts.length === 0 ? (
        <p>No hosts yet. Add a VM or bare-metal host to track provisioning intent.</p>
      ) : (
        <Table aria-label="Site hosts" variant="compact">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Kind</Th>
              <Th>Phase</Th>
              <Th>Cluster</Th>
              <Th>ansible_host</Th>
              <Th>Location</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {hosts.map((host) => (
              <Tr key={host.name}>
                <Td>
                  <code>{host.name}</code>
                </Td>
                <Td>{host.kind || '—'}</Td>
                <Td>
                  <Label color={phaseColor(host.status?.phase)}>{host.status?.phase || 'Declared'}</Label>
                </Td>
                <Td>
                  {host.status?.cluster?.clusterName || host.clusterIntent?.clusterName || host.clusterIntent?.targetCluster ? (
                    <div>
                      {host.status?.cluster?.clusterName || host.clusterIntent?.clusterName || host.clusterIntent?.targetCluster}
                      {host.status?.cluster?.phase && (
                        <>
                          {' '}
                          <Label color={phaseColor(host.status.cluster.phase)} isCompact>
                            {host.status.cluster.phase}
                          </Label>
                        </>
                      )}
                      {host.status?.cluster?.phase === 'Ready' && host.status.cluster.clusterName && (
                        <>
                          {' '}
                          <Link to={`/clusters/${encodeURIComponent(host.status.cluster.clusterName)}`}>View</Link>
                        </>
                      )}
                    </div>
                  ) : (
                    '—'
                  )}
                </Td>
                <Td>
                  <code>{host.ansible_host || host.status?.ansible_host || '—'}</code>
                </Td>
                <Td>{host.vm_location || '—'}</Td>
                <Td>
                  <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                    <Button variant="link" isInline onClick={() => openEdit(host)}>
                      Edit
                    </Button>
                    <Button
                      variant="link"
                      isInline
                      onClick={() => openProvision(host, 'create_vm')}
                    >
                      Provision
                    </Button>
                    {host.kind === 'vm' && (
                      <Button variant="link" isInline onClick={() => openProvision(host, 'plan_vm')}>
                        Plan placement
                      </Button>
                    )}
                    {canEnrollNow(host) && (
                      <Button
                        variant="link"
                        isInline
                        isLoading={applyingCluster === host.name}
                        onClick={() => {
                          if (host.clusterIntent) {
                            void runClusterApply(host);
                          } else {
                            setEditing(host);
                            setForm({
                              ...formFromHost(host),
                              clusterMode: 'enroll',
                              clusterName: defaultClusterName(host.name),
                            });
                            setActionError(null);
                            setEditorOpen(true);
                          }
                        }}
                      >
                        {host.clusterIntent ? 'Apply cluster' : 'Enroll cluster…'}
                      </Button>
                    )}
                    <Button
                      variant="link"
                      isInline
                      isDanger
                      onClick={() => {
                        setDeleteTarget(host);
                        setDeleteOpen(true);
                      }}
                    >
                      Delete
                    </Button>
                  </div>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      <SiteProvisionHistory siteName={siteName} refreshKey={historyRefreshKey} />

      <Modal
        title={editing ? 'Edit host' : 'Add host'}
        isOpen={editorOpen}
        onClose={() => setEditorOpen(false)}
        variant="medium"
        actions={[
          <Button
            key="save"
            variant="primary"
            onClick={() => void saveHost()}
            isLoading={saving}
            isDisabled={!editing && !form.name.trim()}
          >
            Save
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setEditorOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          {!editing && (
            <LabeledField label="Name" fieldId="hostName" isRequired>
              <TextInput
                id="hostName"
                value={form.name}
                onChange={(_e, v) =>
                  setForm({
                    ...form,
                    name: v,
                    clusterName: form.clusterName || defaultClusterName(v),
                  })
                }
                placeholder="vm01"
              />
            </LabeledField>
          )}
          <LabeledField label="Kind" fieldId="hostKind" isRequired>
            <FormSelect
              id="hostKind"
              value={form.kind}
              onChange={(_e, v) => setForm({ ...form, kind: v as SiteHostKind })}
            >
              <FormSelectOption value="vm" label="VM" />
              <FormSelectOption value="bare_metal" label="Bare metal" />
            </FormSelect>
          </LabeledField>
          <LabeledField label="VM location" fieldId="hostVmLocation">
            <TextInput
              id="hostVmLocation"
              value={form.vm_location}
              onChange={(_e, v) => setForm({ ...form, vm_location: v })}
              placeholder="least-utilized or hypervisor hostname"
            />
          </LabeledField>
          <LabeledField label="ansible_host" fieldId="hostAnsibleHost">
            <TextInput
              id="hostAnsibleHost"
              value={form.ansible_host}
              onChange={(_e, v) => setForm({ ...form, ansible_host: v })}
              placeholder="optional until provision completes"
            />
          </LabeledField>
          <LabeledField label="ansible_user" fieldId="hostAnsibleUser">
            <TextInput
              id="hostAnsibleUser"
              value={form.ansible_user}
              onChange={(_e, v) => setForm({ ...form, ansible_user: v })}
            />
          </LabeledField>
          <LabeledField label="OS family" fieldId="hostOsFamily">
            <TextInput
              id="hostOsFamily"
              value={form.osFamily}
              onChange={(_e, v) => setForm({ ...form, osFamily: v })}
            />
          </LabeledField>

          <Title headingLevel="h4" style={{ marginTop: '1rem' }}>
            Cluster automation
          </Title>
          <LabeledField label="After provision" fieldId="hostClusterMode">
            <FormSelect
              id="hostClusterMode"
              value={form.clusterMode}
              onChange={(_e, v) => {
                const mode = v as ClusterFormMode;
                setForm({
                  ...form,
                  clusterMode: mode,
                  clusterName:
                    mode === 'enroll' && !form.clusterName
                      ? defaultClusterName(form.name)
                      : form.clusterName,
                });
              }}
            >
              <FormSelectOption value="" label="None (SSH-ready host only)" />
              <FormSelectOption value="enroll" label="Enroll new cluster" />
              <FormSelectOption value="add_worker" label="Add worker to existing cluster" />
              <FormSelectOption value="replace_control" label="Replace control-plane node" />
            </FormSelect>
          </LabeledField>

          {form.clusterMode === 'enroll' && (
            <>
              <LabeledField label="Topology" fieldId="hostClusterTopology">
                <FormSelect
                  id="hostClusterTopology"
                  value={form.topology}
                  onChange={(_e, v) =>
                    setForm({ ...form, topology: v as 'single_node' | 'ha_3_node' })
                  }
                >
                  <FormSelectOption value="single_node" label="Single node" />
                  <FormSelectOption value="ha_3_node" label="HA 3-node (requires 3 hosts in group)" />
                </FormSelect>
              </LabeledField>
              <LabeledField label="Cluster name" fieldId="hostClusterName" isRequired>
                <TextInput
                  id="hostClusterName"
                  value={form.clusterName}
                  onChange={(_e, v) => setForm({ ...form, clusterName: v })}
                />
              </LabeledField>
              {form.topology === 'ha_3_node' && (
                <>
                  <LabeledField label="HA group" fieldId="hostHaGroup">
                    <TextInput
                      id="hostHaGroup"
                      value={form.haGroup}
                      onChange={(_e, v) => setForm({ ...form, haGroup: v })}
                      placeholder={form.clusterName || 'ha-group-id'}
                    />
                  </LabeledField>
                  <LabeledField label="Control-plane VIP" fieldId="hostCpVip" isRequired>
                    <TextInput
                      id="hostCpVip"
                      value={form.controlPlaneVip}
                      onChange={(_e, v) => setForm({ ...form, controlPlaneVip: v })}
                    />
                  </LabeledField>
                </>
              )}
            </>
          )}

          {(form.clusterMode === 'add_worker' || form.clusterMode === 'replace_control') && (
            <LabeledField label="Target cluster" fieldId="hostTargetCluster" isRequired>
              <FormSelect
                id="hostTargetCluster"
                value={form.targetCluster}
                onChange={(_e, v) => setForm({ ...form, targetCluster: v })}
              >
                <FormSelectOption value="" label="Select cluster…" />
                {siteClusters.map((c) => (
                  <FormSelectOption key={c.name} value={c.name} label={c.name} />
                ))}
              </FormSelect>
            </LabeledField>
          )}

          {form.clusterMode === 'replace_control' && (
            <LabeledField label="Replace node" fieldId="hostReplaceNode" isRequired>
              <TextInput
                id="hostReplaceNode"
                value={form.replaceNode}
                onChange={(_e, v) => setForm({ ...form, replaceNode: v })}
                placeholder="existing control-plane hostname"
              />
            </LabeledField>
          )}

          {!editing && (
            <div style={{ marginTop: '0.75rem' }}>
              <Checkbox
                id="hostProvisionNow"
                isChecked={form.provisionNow}
                onChange={(_e, c) => setForm({ ...form, provisionNow: c })}
                label="Provision immediately after save"
              />
            </div>
          )}
        </Form>
      </Modal>

      <Modal
        title="Delete host"
        isOpen={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        actions={[
          <Button key="del" variant="danger" onClick={() => void confirmDelete()} isLoading={deleting}>
            Delete
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        Delete host <code>{deleteTarget?.name}</code> from site inventory?
      </Modal>

      <Modal
        title={provisionAction === 'plan_vm' ? 'Plan VM placement' : 'Provision host'}
        isOpen={provisionOpen}
        onClose={() => setProvisionOpen(false)}
        actions={[
          <Button
            key="run"
            variant="primary"
            onClick={() => void runProvision()}
            isLoading={provisioning}
            isDisabled={needsConfirm && !provisionConfirm}
          >
            {provisionAction === 'plan_vm' ? 'Run placement plan' : 'Run provision'}
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setProvisionOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Start <code>{provisionAction}</code> for <code>{provisionTarget?.name}</code>.
          {provisionTarget?.kind === 'vm' && provisionAction === 'create_vm' && (
            <>
              {' '}
              VM location <code>{provisionTarget.vm_location || 'least-utilized'}</code> is synced to YAIDAP
              inventory before create.
            </>
          )}
          {provisionAction === 'plan_vm' && (
            <>
              {' '}
              Dry-run only: shows memory-based least-utilized placement without creating a VM or
              persisting inventory.
            </>
          )}
        </p>
        {needsConfirm && (
          <Checkbox
            id="provisionConfirm"
            isChecked={provisionConfirm}
            onChange={(_e, c) => setProvisionConfirm(c)}
            label="I confirm this destructive or bare-metal install action"
          />
        )}
      </Modal>
    </div>
  );
}
