import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  Alert,
  AlertActionCloseButton,
  Button,
  Checkbox,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Label,
  Modal,
  ModalVariant,
  PageSection,
  PageSectionVariants,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { PlusIcon, SyncIcon, TrashIcon } from '@patternfly/react-icons';
import { parse as parseYaml } from 'yaml';
import {
  createFleetCluster,
  deleteFleetCluster,
  fleetClusterIsConsoleProvisioned,
  fleetClusterHasNfsStorage,
  fleetClusterPhase,
  fleetDefaultWorkloadSiteId,
  fleetSiteById,
  fleetSiteIsYaidap,
  fleetSiteEffectiveId,
  fleetSiteIsManagement,
  fleetSiteLabelForId,
  getFleetCluster,
  getSiteInventoryFileOptional,
  listSiteHosts,
} from '../api/client';
import type {
  FleetCluster,
  FleetClusterCreate,
  FleetClusterNodeInput,
  FleetClusterTopology,
  FleetSite,
  SiteHostRecord,
} from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { NfsStorageSetupHelp } from '../components/clusters/NfsStorageSetupHelp';
import { LabeledField } from '../components/forms/LabeledField';
import { LabelKeyValueEditor } from '../components/forms/LabelKeyValueEditor';
import {
  formatIngressOptionLabel,
  ingressOptionDisabled,
} from '../edge/ingressMethod';
import { getFieldHelp } from '../help/fieldHelp';
import { eligibleClusterHosts, hostAnsibleAddr, hostSelectOptions } from '../utils/clusterHosts';
import { useFleetClustersQuery, useFleetSitesQuery, useInvalidateFleet } from '../hooks/useFleetQueries';
import { useAuth } from '../auth/AuthContext';
import { canMutate } from '../auth/roles';

const defaultHaControlNodes = (clusterName: string): FleetClusterNodeInput[] =>
  [1, 2, 3].map((i) => ({
    hostname: clusterName ? `${clusterName}-${i}` : '',
    address: `198.51.100.2${i}`,
    role: 'control' as const,
  }));

const defaultSingleNode = (clusterName: string, address: string): FleetClusterNodeInput[] => [
  {
    hostname: clusterName || 'node',
    address,
    role: 'control',
  },
];

const emptyClusterForm = (siteId = ''): FleetClusterCreate => ({
  clusterName: '',
  siteId,
  bootstrapHost: '198.51.100.11',
  minRootDiskGi: 40,
  clusterRegion: '',
  providers: {
    kubevirt: false,
    kubevirtBridge: {
      enabled: false,
      bridgeName: 'mgmt-l2-br',
      keepIp: true,
    },
  },
  edge: {},
  topology: 'single_node',
  networkMode: 'wireguard',
  nodes: defaultSingleNode('', '198.51.100.11'),
});

function defaultClusterSiteId(sites: FleetSite[], prefill: string): string {
  if (prefill) return prefill;
  return fleetDefaultWorkloadSiteId(sites);
}

function clusterRegionForSite(site: FleetSite | undefined, siteId: string): string {
  const region = site?.spec?.region?.trim();
  if (region) return region;
  return siteId || 'fleet';
}

function phaseColor(phase: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Enrolled':
    case 'Validated':
    case 'ScriptIssued':
    case 'Pending':
      return 'blue';
    case 'Provisioning':
    case 'Deleting':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

export function ClustersPage() {
  const { user } = useAuth();
  const { invalidateClusters, invalidateSites } = useInvalidateFleet();
  const clustersQuery = useFleetClustersQuery(30_000);
  const sitesQuery = useFleetSitesQuery(30_000);
  const rows = clustersQuery.data ?? [];
  const sites = sitesQuery.data ?? [];
  const loading = clustersQuery.isLoading || sitesQuery.isLoading;
  const fetching = clustersQuery.isFetching || sitesQuery.isFetching;
  const error = clustersQuery.error || sitesQuery.error;
  const [searchParams] = useSearchParams();
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState<FleetClusterCreate>(emptyClusterForm());
  const [saving, setSaving] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<FleetCluster | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<unknown>(null);
  const [deleteCleanupKubernetes, setDeleteCleanupKubernetes] = useState(false);
  const [deleteCleanupVms, setDeleteCleanupVms] = useState(false);
  const [deleteCleanupVmDecls, setDeleteCleanupVmDecls] = useState(false);
  const [deleteCleanupApps, setDeleteCleanupApps] = useState(false);
  const [deleteCleanupStorage, setDeleteCleanupStorage] = useState(false);
  const [localPathEnabled, setLocalPathEnabled] = useState(true);
  const [nfsLocalEnabled, setNfsLocalEnabled] = useState(false);
  const [nfsEnabled, setNfsEnabled] = useState(false);
  const [longhornEnabled, setLonghornEnabled] = useState(false);
  const [longhornDevice, setLonghornDevice] = useState('/dev/sdb');
  const [controlPlaneVip, setControlPlaneVip] = useState('');
  const [nfsServer, setNfsServer] = useState('');
  const [nfsExportBase, setNfsExportBase] = useState('/srv/fleet-nfs');
  const [nfsDelegateHost, setNfsDelegateHost] = useState('');
  const [nfsMountPath, setNfsMountPath] = useState('');
  const [clusterLabels, setClusterLabels] = useState<Record<string, string>>({});
  const [topology, setTopology] = useState<FleetClusterTopology>('single_node');
  const [siteHosts, setSiteHosts] = useState<SiteHostRecord[]>([]);
  const [pendingNode, setPendingNode] = useState('');
  const [provisionNow, setProvisionNow] = useState(false);
  const [progress, setProgress] = useState<string[]>([]);
  const [createdNotice, setCreatedNotice] = useState<{ name: string; provisioning: boolean } | null>(null);
  const [lhDiskWarnings, setLhDiskWarnings] = useState<string[]>([]);
  const [filterGpu, setFilterGpu] = useState(false);
  const [filterKvm, setFilterKvm] = useState(false);
  const [filterKubevirt, setFilterKubevirt] = useState(false);
  const [filterHa, setFilterHa] = useState(false);
  const [filterRwx, setFilterRwx] = useState(false);

  const filteredRows = useMemo(() => {
    return rows.filter((c) => {
      const caps = c.capabilities || {};
      if (filterGpu && !caps.gpu) return false;
      if (filterKvm && !caps.kvm) return false;
      if (filterKubevirt && !caps.kubevirt) return false;
      if (filterHa && !caps.ha_topology) return false;
      if (filterRwx && !caps['storage.rwx']) return false;
      return true;
    });
  }, [rows, filterGpu, filterKvm, filterKubevirt, filterHa, filterRwx]);

  const sitePrefill = searchParams.get('site') || '';
  const createPrefill = searchParams.get('create') === '1';
  const nodePrefill = searchParams.get('node') || '';

  const registrySites = useMemo(
    () =>
      sites.filter(
        (s) =>
          (s.spec.provisioningMode === 'registry' || !s.spec.bootstrapHost) &&
          !fleetSiteIsManagement(s),
      ),
    [sites],
  );

  const selectedSite = useMemo(
    () => fleetSiteById(registrySites, form.siteId),
    [registrySites, form.siteId],
  );

  const prefillSiteId = useMemo(() => {
    const s = fleetSiteById(registrySites, sitePrefill);
    return s ? fleetSiteEffectiveId(s) : sitePrefill;
  }, [registrySites, sitePrefill]);

  const ingressOptions = useMemo(
    () => selectedSite?.spec.edge?.ingress || [],
    [selectedSite],
  );

  const autoIngressId = useMemo(() => {
    if (ingressOptions.length !== 1) return '';
    return ingressOptions[0].method === 'port_forward' ? ingressOptions[0].id : '';
  }, [ingressOptions]);

  const controlNodes = useMemo(
    () => (form.nodes || []).filter((n) => (n.role || 'control') === 'control'),
    [form.nodes],
  );
  const workerNodes = useMemo(
    () => (form.nodes || []).filter((n) => n.role === 'worker'),
    [form.nodes],
  );

  const hostAddr = hostAnsibleAddr;
  const selectedHostNames = useMemo(
    () => new Set((form.nodes || []).map((n) => (n.hostname || '').trim()).filter(Boolean)),
    [form.nodes],
  );
  // Selectable nodes: has an address, is not a hypervisor, and is not already a
  // member of a cluster.
  const hostOptions = useMemo(() => eligibleClusterHosts(siteHosts), [siteHosts]);
  // Keep a slot's own value visible while hiding hosts picked in other slots.
  const slotHostOptions = (current: string) => hostSelectOptions(hostOptions, selectedHostNames, current);
  const hostByName = useMemo(() => {
    const m: Record<string, SiteHostRecord> = {};
    for (const h of siteHosts) m[h.name] = h;
    return m;
  }, [siteHosts]);

  // Load the selected site's inventory hosts for the node pickers.
  useEffect(() => {
    if (!modalOpen || !form.siteId) {
      setSiteHosts([]);
      return;
    }
    let cancelled = false;
    listSiteHosts(form.siteId)
      .then((list) => {
        if (!cancelled) setSiteHosts(list);
      })
      .catch(() => {
        if (!cancelled) setSiteHosts([]);
      });
    return () => {
      cancelled = true;
    };
  }, [modalOpen, form.siteId]);

  // Longhorn wants a dedicated block device (/dev/vdb by default). Warn when the
  // selected node guests only have a boot disk: the bootstrap validates the
  // device and fails the cluster otherwise.
  useEffect(() => {
    if (!modalOpen || !longhornEnabled) {
      setLhDiskWarnings([]);
      return;
    }
    const declared = new Set(siteHosts.map((h) => h.name));
    const names = [
      ...new Set(
        (form.nodes || [])
          .filter((n) => (n.role || 'control') === 'control')
          .map((n) => n.hostname)
          .filter((h): h is string => !!h && declared.has(h)),
      ),
    ];
    if (names.length === 0) {
      setLhDiskWarnings([]);
      return;
    }
    let cancelled = false;
    void (async () => {
      const missing: string[] = [];
      for (const h of names) {
        try {
          const res = await getSiteInventoryFileOptional(form.siteId, `host_vars/${h}.yml`);
          const doc = res.missing ? null : (parseYaml(res.content) as { disks?: { bootdisk?: boolean }[] } | null);
          const data = (doc?.disks || []).filter((d) => d?.bootdisk !== true);
          if (data.length === 0) missing.push(h);
        } catch {
          // best-effort: an unreadable host_vars must not block the wizard
        }
      }
      if (!cancelled) setLhDiskWarnings(missing);
    })();
    return () => {
      cancelled = true;
    };
  }, [modalOpen, longhornEnabled, form.nodes, form.siteId, siteHosts]);

  const applyHostToNode = (index: number, role: FleetClusterNodeInput['role'], hostName: string) => {
    const h = hostByName[hostName];
    const addr = h ? hostAddr(h) : '';
    setForm((f) => {
      const base =
        topology === 'ha_3_node' ? f.nodes || defaultHaControlNodes(f.clusterName) : f.nodes || [];
      const nodes = [...base];
      const targets = nodes.map((n, i) => ({ n, i })).filter((x) => (x.n.role || 'control') === role);
      const t = targets[index]?.i ?? index;
      nodes[t] = { ...nodes[t], hostname: hostName, address: addr, role };
      return {
        ...f,
        nodes,
        bootstrapHost: role === 'control' && index === 0 && addr ? addr : f.bootstrapHost,
      };
    });
  };

  // Apply a queued host selection once the site's hosts are loaded.
  useEffect(() => {
    if (!pendingNode) return;
    const h = hostByName[pendingNode];
    if (!h) return;
    const addr = hostAddr(h);
    setForm((f) => ({
      ...f,
      bootstrapHost: addr || f.bootstrapHost,
      nodes:
        topology === 'ha_3_node'
          ? (f.nodes || []).map((n, i) =>
              i === 0 ? { ...n, hostname: h.name, address: addr || n.address } : n,
            )
          : [
              {
                hostname: h.name,
                address: addr || f.bootstrapHost,
                role: 'control' as const,
                labels: f.nodes?.[0]?.labels,
              },
            ],
    }));
    setPendingNode('');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingNode, hostByName, topology]);

  const deleteConsoleProvisioned = deleteTarget
    ? fleetClusterIsConsoleProvisioned(deleteTarget)
    : false;
  const deleteYaidapSite = fleetSiteIsYaidap(
    deleteTarget ? fleetSiteById(sites, deleteTarget.site) : null,
  );

  const openDelete = (c: FleetCluster) => {
    setDeleteTarget(c);
    setDeleteCleanupKubernetes(false);
    setDeleteCleanupVms(false);
    setDeleteCleanupVmDecls(false);
    setDeleteCleanupApps(false);
    setDeleteCleanupStorage(false);
    setDeleteError(null);
    setDeleteOpen(true);
  };

  const confirmDelete = async () => {
    if (!deleteTarget?.name) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deleteFleetCluster(deleteTarget.name, {
        cleanupKubernetes: deleteConsoleProvisioned && deleteCleanupKubernetes,
        cleanupVms: deleteConsoleProvisioned && deleteCleanupVms,
        cleanupVmDeclarations: deleteConsoleProvisioned && deleteCleanupVmDecls,
        cleanupApps: deleteConsoleProvisioned && deleteCleanupApps,
        cleanupStorage: deleteConsoleProvisioned && deleteCleanupStorage,
      });
      setDeleteOpen(false);
      setDeleteTarget(null);
      await load();
    } catch (e) {
      setDeleteError(e);
    } finally {
      setDeleting(false);
    }
  };

  const load = () => {
    void invalidateClusters();
    void invalidateSites();
  };

  useEffect(() => {
    if (createPrefill) return;
    if (prefillSiteId) {
      setForm((f) => ({ ...f, siteId: prefillSiteId }));
    }
  }, [prefillSiteId, createPrefill]);

  const openWizard = () => {
    const siteId = defaultClusterSiteId(registrySites, prefillSiteId);
    const site = registrySites.find((s) => fleetSiteEffectiveId(s) === siteId);
    setForm({
      ...emptyClusterForm(siteId),
      clusterRegion: clusterRegionForSite(site, siteId),
    });
    setLocalPathEnabled(true);
    setNfsLocalEnabled(false);
    setNfsEnabled(false);
    setLonghornEnabled(false);
    setLonghornDevice('/dev/sdb');
    setControlPlaneVip('');
    setNfsServer('');
    setNfsExportBase('/srv/fleet-nfs');
    setNfsDelegateHost('');
    setNfsMountPath('');
    setClusterLabels({});
    setTopology('single_node');
    setActionError(null);
    setModalOpen(true);
  };

  // Hosts-tab shortcut: /clusters?site=X&create=1&node=Y opens the wizard with the
  // selected inventory host prefilled as the node.
  const prefillApplied = useRef(false);
  useEffect(() => {
    if (prefillApplied.current || !createPrefill || registrySites.length === 0) return;
    prefillApplied.current = true;
    openWizard();
    if (nodePrefill) setPendingNode(nodePrefill);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [createPrefill, nodePrefill, registrySites.length]);

  const addWorkerNode = () => {
    const workers = workerNodes.length;
    setForm((f) => ({
      ...f,
      nodes: [
        ...(f.nodes || []),
        {
          hostname: `${f.clusterName || 'cluster'}-w${workers + 1}`,
          address: '',
          role: 'worker' as const,
        },
      ],
    }));
  };

  const removeWorkerNode = (hostname: string) => {
    setForm((f) => ({
      ...f,
      nodes: (f.nodes || []).filter((n) => n.hostname !== hostname || n.role !== 'worker'),
    }));
  };

  const updateControlNodeLabels = (controlIndex: number, labels: Record<string, string>) => {
    setForm((f) => {
      const nodes = [...(f.nodes || defaultHaControlNodes(f.clusterName))];
      let seen = -1;
      return {
        ...f,
        nodes: nodes.map((n) => {
          if ((n.role || 'control') !== 'control') return n;
          seen += 1;
          if (seen !== controlIndex) return n;
          return { ...n, labels: Object.keys(labels).length > 0 ? labels : undefined };
        }),
      };
    });
  };

  const updateWorkerNodeLabels = (hostname: string, labels: Record<string, string>) => {
    setForm((f) => ({
      ...f,
      nodes: (f.nodes || []).map((n) =>
        n.role === 'worker' && n.hostname === hostname
          ? { ...n, labels: Object.keys(labels).length > 0 ? labels : undefined }
          : n,
      ),
    }));
  };

  const updateSingleNodeLabels = (labels: Record<string, string>) => {
    setForm((f) => {
      const nodes = [...(f.nodes || defaultSingleNode(f.clusterName, f.bootstrapHost))];
      nodes[0] = {
        ...nodes[0],
        hostname: f.clusterName || nodes[0].hostname || 'node',
        address: f.bootstrapHost,
        role: 'control',
        labels: Object.keys(labels).length > 0 ? labels : undefined,
      };
      return { ...f, nodes };
    });
  };

  const handleCreate = async () => {
    setSaving(true);
    setActionError(null);
    setProgress([]);
    try {
      const caps: Record<string, boolean> = {};
      const prov = form.providers || {};
      if (prov.kubevirt) caps.kvm = true;
      const isHa = topology === 'ha_3_node';
      if (isHa) caps.ha_topology = true;
      let nodes: FleetClusterNodeInput[] | undefined;
      if (isHa) {
        const controls = (form.nodes || defaultHaControlNodes(form.clusterName))
          .filter((n) => (n.role || 'control') === 'control')
          .slice(0, 3)
          .map((n, i) => ({
            ...n,
            role: 'control' as const,
            hostname: n.hostname || `${form.clusterName}-${i + 1}`,
          }));
        while (controls.length < 3) {
          const i = controls.length;
          const fallback = defaultHaControlNodes(form.clusterName)[i];
          controls.push({
            ...fallback,
            role: 'control' as const,
            hostname: fallback.hostname || `${form.clusterName}-${i + 1}`,
          });
        }
        const workers = (form.nodes || [])
          .filter((n) => n.role === 'worker')
          .map((n, i) => ({
            ...n,
            role: 'worker' as const,
            hostname: n.hostname || `${form.clusterName}-w${i + 1}`,
          }));
        nodes = [...controls, ...workers];
      } else {
        nodes = [
          {
            hostname: form.clusterName || 'node',
            address: form.bootstrapHost,
            role: 'control' as const,
            labels: form.nodes?.[0]?.labels,
          },
        ];
      }
      const bootstrapHost = isHa ? nodes[0]?.address || form.bootstrapHost : form.bootstrapHost;
      if (isHa && !controlPlaneVip.trim()) {
        setActionError(new Error('Control-plane VIP is required for 3-node HA'));
        setSaving(false);
        return;
      }
      const log = (m: string) => setProgress((p) => [...p, m]);
      const createBody = {
        ...form,
        topology,
        networkMode: 'wireguard',
        bootstrapHost,
        controlPlaneVip: isHa ? controlPlaneVip.trim() : undefined,
        nodes,
        clusterLabels,
        clusterCapabilities: caps,
        autoProvision: provisionNow,
        storage: nfsLocalEnabled && !isHa
          ? {
              backend: 'nfs_local',
              localPath: { enabled: localPathEnabled },
              rwo: true,
              rwx: true,
              nfs: {
                exportBase: '/srv/fleet-nfs',
                clusterPath: form.clusterName.trim() || undefined,
                storageClass: 'nfs-client',
              },
              longhorn: { enabled: false },
            }
          : {
              localPath: { enabled: longhornEnabled ? false : localPathEnabled },
              longhorn: longhornEnabled
                ? {
                    enabled: true,
                    device: longhornDevice.trim() || '/dev/sdb',
                  }
                : { enabled: false },
              nfs: nfsEnabled
                ? {
                    enabled: true,
                    server: nfsServer.trim(),
                    exportBase: nfsExportBase.trim() || '/srv/fleet-nfs',
                    clusterPath: form.clusterName.trim() || undefined,
                    delegateHost: nfsDelegateHost.trim() || undefined,
                    mountPath: nfsMountPath.trim() || undefined,
                  }
                : { enabled: false },
            },
      };
      try {
        await createFleetCluster(createBody as FleetClusterCreate);
      } catch (e) {
        // autoApply can already have created the intent from the cluster group;
        // only fail when the cluster truly does not exist.
        const exists = await getFleetCluster(form.clusterName)
          .then(() => true)
          .catch(() => false);
        if (!exists) throw e;
        log('Cluster intent already existed; continuing.');
      }
      // Bring-up runs in the controller (spec.autoProvision) or in the bootstrap
      // bundle the operator runs. Either way the create call is done here: close
      // the wizard and let the list / cluster page track progress in the
      // background instead of holding the modal open until the cluster is Ready.
      setCreatedNotice({ name: form.clusterName, provisioning: provisionNow });
      setModalOpen(false);
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1">Clusters</Title>
        <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
          OCM managed clusters and pending joins. Create a Rocky cluster (single-node or 3-node HA with
          optional workers) under a site. Download the bootstrap bundle, run it on the first control node
          (WireGuard + K3s on all nodes), then Provision when Validated.
        </p>
        <Toolbar style={{ marginTop: '0.75rem' }}>
          <ToolbarContent>
            <ToolbarItem>
              <Button variant="primary" icon={<PlusIcon />} isDisabled={!canMutate(user)} onClick={openWizard}>
                Create cluster
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={() => void load()} isDisabled={fetching}>
                Refresh
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Checkbox
                id="filter-gpu"
                label="GPU"
                isChecked={filterGpu}
                onChange={(_e, checked) => setFilterGpu(checked)}
              />
            </ToolbarItem>
            <ToolbarItem>
              <Checkbox
                id="filter-kvm"
                label="KVM"
                isChecked={filterKvm}
                onChange={(_e, checked) => setFilterKvm(checked)}
              />
            </ToolbarItem>
            <ToolbarItem>
              <Checkbox
                id="filter-kubevirt"
                label="KubeVirt"
                isChecked={filterKubevirt}
                onChange={(_e, checked) => setFilterKubevirt(checked)}
              />
            </ToolbarItem>
            <ToolbarItem>
              <Checkbox
                id="filter-ha"
                label="HA topology"
                isChecked={filterHa}
                onChange={(_e, checked) => setFilterHa(checked)}
              />
            </ToolbarItem>
            <ToolbarItem>
              <Checkbox
                id="filter-rwx"
                label="RWX storage"
                isChecked={filterRwx}
                onChange={(_e, checked) => setFilterRwx(checked)}
              />
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
        <p style={{ marginTop: '0.5rem', fontSize: '0.85rem', color: 'var(--pf-v6-global--Color--200)' }}>
          Capability filters (placement helper): show clusters that report the selected capabilities.
        </p>
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        <ErrorAlert error={actionError} />
        {createdNotice ? (
          <Alert
            variant="info"
            isInline
            title={`Created ${createdNotice.name}`}
            actionClose={<AlertActionCloseButton onClose={() => setCreatedNotice(null)} />}
            style={{ marginBottom: '0.75rem' }}
          >
            {createdNotice.provisioning
              ? 'The controller is creating the nodes and provisioning in the background.'
              : 'Download the bootstrap bundle from the cluster page to bring it up.'}{' '}
            <Link to={`/clusters/${encodeURIComponent(createdNotice.name)}`}>Open the cluster</Link> to
            track progress.
          </Alert>
        ) : null}
        <Table aria-label="Fleet clusters">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Site</Th>
              <Th>Phase</Th>
              <Th>Available</Th>
              <Th>Caps</Th>
              <Th>Ingress</Th>
              <Th>Auth</Th>
              <Th>Provisioner</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {filteredRows.map((c) => {
              const phase = fleetClusterPhase(c);
              const canDelete = !c.read_only && !c.management;
              const caps = c.capabilities || {};
              const capBits = [
                caps.gpu ? 'GPU' : null,
                caps.kvm ? 'KVM' : null,
                caps.kubevirt ? 'KV' : null,
                caps.ha_topology ? 'HA' : null,
                caps['storage.rwx'] ? 'RWX' : null,
              ].filter(Boolean);
              return (
                <Tr key={c.name}>
                  <Td dataLabel="Name">
                    <Link to={`/clusters/${c.name}`}>{c.name}</Link>
                  </Td>
                  <Td dataLabel="Site">
                    {fleetSiteLabelForId(sites, c.site)}
                    {c.site && (
                      <>
                        <br />
                        <code>{c.site}</code>
                      </>
                    )}
                  </Td>
                  <Td dataLabel="Phase">
                    <Label color={phaseColor(phase)}>{phase}</Label>
                  </Td>
                  <Td dataLabel="Available">
                    <Label color={c.available ? 'green' : 'grey'}>
                      {c.available ? 'Yes' : 'No'}
                    </Label>
                  </Td>
                  <Td dataLabel="Caps">
                    {capBits.length > 0 ? (
                      <span style={{ fontSize: '0.85rem' }}>{capBits.join(' · ')}</span>
                    ) : (
                      <span style={{ color: 'var(--pf-v6-global--Color--200)' }}>—</span>
                    )}
                  </Td>
                  <Td dataLabel="Ingress">{c.ingress_enabled ? 'Yes' : 'No'}</Td>
                  <Td dataLabel="Auth">{c.ingress_auth_enabled ? 'Yes' : 'No'}</Td>
                  <Td dataLabel="Provisioner">
                    {c.topology?.provisioner || '—'}
                    {c.topology?.nest_depth && c.topology.nest_depth !== '0'
                      ? ` · depth ${c.topology.nest_depth}`
                      : ''}
                    {c.topology?.parent_cluster ? ` · parent ${c.topology.parent_cluster}` : ''}
                  </Td>
                  <Td dataLabel="Actions">
                    {canDelete && (
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => openDelete(c)}
                        data-testid={`cluster-delete-${c.name}`}
                      >
                        Delete
                      </Button>
                    )}
                  </Td>
                </Tr>
              );
            })}
          </Tbody>
        </Table>
        {!loading && filteredRows.length === 0 && (
          <p>{rows.length === 0 ? 'No managed clusters found.' : 'No clusters match the capability filters.'}</p>
        )}
      </PageSection>

      <Modal
        title="Delete cluster"
        isOpen={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        actions={[
          <Button
            key="del"
            variant="danger"
            onClick={() => void confirmDelete()}
            isLoading={deleting}
            data-testid="cluster-delete-confirm"
          >
            Delete
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={deleteError} />
        <p>
          Delete cluster <strong>{deleteTarget?.name}</strong>? Hub teardown unjoins OCM, removes
          ManifestWorks, revokes hub WireGuard peers, and removes the fleet overlay (wg-umkim) on
          nodes. User workloads on K3s are kept unless you opt in below.
        </p>
        {deleteConsoleProvisioned && (
          <LabeledField
            label="Uninstall applications first"
            fieldId="deleteCleanupApps"
            help={getFieldHelp('clusterDelete.cleanupApps')}
          >
            <Checkbox
              id="deleteCleanupApps"
              label="Uninstall applications first"
              isChecked={deleteCleanupApps}
              onChange={(_e, checked) => setDeleteCleanupApps(checked)}
              data-testid="cluster-delete-cleanup-apps"
            />
          </LabeledField>
        )}
        {deleteConsoleProvisioned && (
          <LabeledField
            label="Uninstall Kubernetes (destroy cluster)"
            fieldId="deleteCleanupKubernetes"
            help={getFieldHelp('clusterDelete.cleanupKubernetes')}
          >
            <Checkbox
              id="deleteCleanupKubernetes"
              label="Uninstall Kubernetes (destroy cluster)"
              isChecked={deleteCleanupKubernetes}
              onChange={(_e, checked) => setDeleteCleanupKubernetes(checked)}
              data-testid="cluster-delete-cleanup-kubernetes"
            />
          </LabeledField>
        )}
        {deleteConsoleProvisioned && fleetClusterHasNfsStorage(deleteTarget) && (
          <LabeledField
            label="Delete the cluster's storage export"
            fieldId="deleteCleanupStorage"
            help={getFieldHelp('clusterDelete.cleanupStorage')}
          >
            <Checkbox
              id="deleteCleanupStorage"
              label="Delete the cluster's storage export (data)"
              isChecked={deleteCleanupStorage}
              onChange={(_e, checked) => setDeleteCleanupStorage(checked)}
              data-testid="cluster-delete-cleanup-storage"
            />
          </LabeledField>
        )}
        {!deleteConsoleProvisioned && deleteTarget?.available && (
          <p>
            <strong>Warning:</strong> this is a fleet inventory spoke; deleting it removes OCM
            registration for the whole spoke.
          </p>
        )}
        {deleteConsoleProvisioned && deleteYaidapSite && (
          <>
            <LabeledField
              label="Destroy the site VMs"
              fieldId="deleteCleanupVms"
              help={getFieldHelp('clusterDelete.cleanupVms')}
            >
              <Checkbox
                id="deleteCleanupVms"
                label="Destroy the site VMs"
                isChecked={deleteCleanupVms}
                onChange={(_e, checked) => setDeleteCleanupVms(checked)}
                data-testid="cluster-delete-cleanup-vms"
              />
            </LabeledField>
            {deleteCleanupVms && (
              <LabeledField
                label="Also remove the VMs from the site inventory"
                fieldId="deleteCleanupVmDecls"
                help={getFieldHelp('clusterDelete.cleanupVmDeclarations')}
              >
                <Checkbox
                  id="deleteCleanupVmDecls"
                  label="Also remove the VMs from the site inventory"
                  isChecked={deleteCleanupVmDecls}
                  onChange={(_e, checked) => setDeleteCleanupVmDecls(checked)}
                  data-testid="cluster-delete-cleanup-vm-declarations"
                />
              </LabeledField>
            )}
          </>
        )}
      </Modal>

      <Modal
        variant={ModalVariant.medium}
        title="Create cluster"
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        actions={[
          <Button key="create" variant="primary" isLoading={saving} onClick={() => void handleCreate()}>
            Create
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setModalOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <Checkbox
            id="provisionNow"
            isChecked={provisionNow}
            isDisabled={saving}
            onChange={(_e, c) => setProvisionNow(c)}
            label="Provision now: create the VMs, run bootstrap, and bring the cluster up"
          />
          <p
            style={{
              fontSize: '0.8rem',
              marginTop: '0.25rem',
              marginBottom: '0.75rem',
              color: 'var(--pf-v5-global--Color--200)',
            }}
          >
            The controller creates the Declared VM nodes, bootstraps via the site jumphost, and provisions
            the cluster (each step runs in a pod). The whole flow can take 30+ minutes and runs server-side
            even if you close this page. Leave unchecked to only create the cluster intent.
          </p>
          {progress.length > 0 && (
            <Alert
              isInline
              variant={actionError ? 'danger' : 'info'}
              title="Provisioning progress"
              style={{ marginBottom: '0.75rem' }}
            >
              <ul style={{ margin: 0, paddingLeft: '1.1rem', fontSize: '0.85rem' }}>
                {progress.map((m, i) => (
                  <li key={i}>{m}</li>
                ))}
              </ul>
            </Alert>
          )}
          <LabeledField label="Parent site" isRequired fieldId="siteId" help={getFieldHelp('clusterJoin.siteId')}>
            <FormSelect
              id="siteId"
              value={form.siteId}
              onChange={(_e, v) => {
                const site = registrySites.find((s) => fleetSiteEffectiveId(s) === v);
                setForm({
                  ...form,
                  siteId: v,
                  edge: {},
                  clusterRegion: clusterRegionForSite(site, v),
                });
              }}
            >
              {registrySites.map((s) => {
                const id = fleetSiteEffectiveId(s);
                return (
                  <FormSelectOption
                    key={id}
                    value={id || ''}
                    label={s.spec?.displayName || s.spec?.siteName || s.metadata?.name || id || ''}
                  />
                );
              })}
            </FormSelect>
          </LabeledField>
          <LabeledField label="Cluster name (OCM)" isRequired fieldId="clusterName" help={getFieldHelp('clusterJoin.clusterName')}>
            <TextInput
              id="clusterName"
              value={form.clusterName}
              onChange={(_e, v) =>
                setForm((f) => ({
                  ...f,
                  clusterName: v,
                  nodes:
                    topology === 'ha_3_node'
                      ? [
                          ...defaultHaControlNodes(v).map((n, i) => {
                            const prevControls = (f.nodes || []).filter(
                              (x) => (x.role || 'control') === 'control',
                            );
                            return { ...n, labels: prevControls[i]?.labels };
                          }),
                          ...(f.nodes || []).filter((n) => n.role === 'worker'),
                        ]
                      : defaultSingleNode(v, f.bootstrapHost).map((n) => ({
                          ...n,
                          labels: f.nodes?.[0]?.labels,
                        })),
                }))
              }
              placeholder="lab-cluster1"
            />
          </LabeledField>
          <LabeledField label="Jumphost" fieldId="clusterJumphost" help={getFieldHelp('clusterJoin.jumphost')}>
            <TextInput
              id="clusterJumphost"
              value={form.jumphost || ''}
              onChange={(_e, v) => setForm({ ...form, jumphost: v })}
              placeholder="Leave empty to use site jumphost"
            />
          </LabeledField>
          <LabeledField label="Topology" fieldId="topology" help={getFieldHelp('clusterJoin.topology')}>
            <FormSelect
              id="topology"
              value={topology}
              onChange={(_e, v) => {
                const t = v as FleetClusterTopology;
                setTopology(t);
                if (t === 'ha_3_node') {
                  setForm((f) => ({
                    ...f,
                    nodes: defaultHaControlNodes(f.clusterName),
                    bootstrapHost: defaultHaControlNodes(f.clusterName)[0]?.address || f.bootstrapHost,
                  }));
                  setLonghornEnabled(true);
                  setLocalPathEnabled(false);
                  setNfsLocalEnabled(false);
                  setControlPlaneVip('198.51.100.84');
                  setLonghornDevice('/dev/vdb');
                } else {
                  setLonghornEnabled(false);
                  setLocalPathEnabled(true);
                  setControlPlaneVip('');
                  setLonghornDevice('/dev/sdb');
                  setForm((f) => ({
                    ...f,
                    nodes: defaultSingleNode(f.clusterName, f.bootstrapHost),
                  }));
                }
              }}
            >
              <FormSelectOption value="single_node" label="Single node" />
              <FormSelectOption value="ha_3_node" label="3-node HA (K3s cluster-init)" />
            </FormSelect>
          </LabeledField>
          {topology === 'ha_3_node' ? (
            <>
              {[0, 1, 2].map((i) => (
                <div key={i}>
                  <LabeledField
                    label={`Control node ${i + 1} IP`}
                    isRequired
                    fieldId={`node-${i}`}
                    help={getFieldHelp('clusterJoin.controlNodeIp')}
                  >
                    {hostOptions.length > 0 && (
                      <FormSelect
                        id={`node-host-${i}`}
                        value={hostByName[controlNodes[i]?.hostname || ''] ? controlNodes[i]?.hostname : ''}
                        onChange={(_e, v) => applyHostToNode(i, 'control', v)}
                        aria-label={`Control node ${i + 1} inventory host`}
                        style={{ marginBottom: '0.5rem' }}
                      >
                        <FormSelectOption value="" label="Select inventory host…" />
                        {slotHostOptions(controlNodes[i]?.hostname || '').map((h) => (
                          <FormSelectOption
                            key={h.name}
                            value={h.name}
                            label={`${h.name} — ${hostAddr(h)}${h.kind ? ` (${h.kind})` : ''}${h.status?.phase ? ` · ${h.status.phase}` : ''}`}
                          />
                        ))}
                      </FormSelect>
                    )}
                    <TextInput
                      id={`node-${i}`}
                      value={controlNodes[i]?.address || ''}
                      onChange={(_e, v) => {
                        const nodes = [...(form.nodes || defaultHaControlNodes(form.clusterName))];
                        const idx = nodes.findIndex(
                          (n, j) => (n.role || 'control') === 'control' && j === i,
                        );
                        const target = idx >= 0 ? idx : i;
                        nodes[target] = {
                          ...nodes[target],
                          role: 'control',
                          hostname: `${form.clusterName || 'cluster'}-${i + 1}`,
                          address: v,
                        };
                        setForm({
                          ...form,
                          nodes,
                          bootstrapHost: i === 0 ? v : form.bootstrapHost,
                        });
                      }}
                    />
                    <small>{controlNodes[i]?.hostname || `${form.clusterName || 'cluster'}-${i + 1}`}</small>
                  </LabeledField>
                  <LabeledField
                    label={`Control node ${i + 1} labels`}
                    fieldId={`node-labels-${i}`}
                    help={getFieldHelp('clusterJoin.nodeLabels')}
                  >
                    <LabelKeyValueEditor
                      labels={controlNodes[i]?.labels || {}}
                      onChange={(labels) => updateControlNodeLabels(i, labels)}
                      idPrefix={`control-${i}-label`}
                    />
                  </LabeledField>
                </div>
              ))}
              <LabeledField
                label="Control-plane VIP"
                isRequired
                fieldId="controlPlaneVip"
                help={getFieldHelp('clusterJoin.controlPlaneVip')}
              >
                <TextInput
                  id="controlPlaneVip"
                  value={controlPlaneVip}
                  onChange={(_e, v) => setControlPlaneVip(v)}
                  placeholder="198.51.100.84"
                />
              </LabeledField>
              <LabeledField label="Longhorn (RWO)" fieldId="longhornEnabled" help={getFieldHelp('clusterJoin.longhornEnabled')}>
                <Checkbox
                  id="longhornEnabled"
                  label="Install Longhorn on dedicated data disks"
                  isChecked={longhornEnabled}
                  onChange={(_e, checked) => {
                    setLonghornEnabled(checked);
                    if (checked) setLocalPathEnabled(false);
                  }}
                />
              </LabeledField>
              {longhornEnabled && lhDiskWarnings.length > 0 && (
                <Alert
                  variant="warning"
                  isInline
                  title="Longhorn needs a data disk on every node"
                  style={{ marginBottom: '0.75rem' }}
                >
                  <code>{lhDiskWarnings.join(', ')}</code>{' '}
                  {lhDiskWarnings.length === 1 ? 'has' : 'have'} no second disk, but this cluster installs
                  Longhorn on <code>{longhornDevice.trim() || '/dev/vdb'}</code>. The bootstrap validates
                  that device and fails the cluster without it. Add a data disk to those guests
                  (Sites → Hosts → Add VM guest → Additional disks) or turn Longhorn off.
                </Alert>
              )}
              {longhornEnabled && (
                <LabeledField
                  label="Longhorn data disk"
                  fieldId="longhornDevice"
                  help={getFieldHelp('clusterJoin.longhornDevice')}
                >
                  <TextInput
                    id="longhornDevice"
                    value={longhornDevice}
                    onChange={(_e, v) => setLonghornDevice(v)}
                    placeholder="/dev/sdb"
                  />
                </LabeledField>
              )}
              {workerNodes.map((w, wi) => (
                <div key={w.hostname || `worker-${wi}`}>
                  <LabeledField
                    label={`Worker ${wi + 1} IP`}
                    isRequired
                    fieldId={`worker-${wi}`}
                    help={getFieldHelp('clusterJoin.workerNodeIp')}
                  >
                    {hostOptions.length > 0 && (
                      <FormSelect
                        id={`worker-host-${wi}`}
                        value={hostByName[w.hostname || ''] ? w.hostname : ''}
                        onChange={(_e, v) => applyHostToNode(wi, 'worker', v)}
                        aria-label={`Worker ${wi + 1} inventory host`}
                        style={{ marginBottom: '0.5rem' }}
                      >
                        <FormSelectOption value="" label="Select inventory host…" />
                        {slotHostOptions(w.hostname || '').map((h) => (
                          <FormSelectOption
                            key={h.name}
                            value={h.name}
                            label={`${h.name} — ${hostAddr(h)}${h.kind ? ` (${h.kind})` : ''}${h.status?.phase ? ` · ${h.status.phase}` : ''}`}
                          />
                        ))}
                      </FormSelect>
                    )}
                    <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                      <TextInput
                        id={`worker-${wi}`}
                        value={w.address}
                        onChange={(_e, v) => {
                          const nodes = [...(form.nodes || [])];
                          const idx = nodes.findIndex((n) => n === w);
                          if (idx >= 0) {
                            nodes[idx] = { ...nodes[idx], address: v };
                            setForm({ ...form, nodes });
                          }
                        }}
                      />
                      <Button
                        variant="plain"
                        icon={<TrashIcon />}
                        aria-label="Remove worker"
                        onClick={() => removeWorkerNode(w.hostname || '')}
                      />
                    </div>
                    <small>{w.hostname}</small>
                  </LabeledField>
                  <LabeledField
                    label={`Worker ${wi + 1} labels`}
                    fieldId={`worker-labels-${wi}`}
                    help={getFieldHelp('clusterJoin.nodeLabels')}
                  >
                    <LabelKeyValueEditor
                      labels={w.labels || {}}
                      onChange={(labels) => updateWorkerNodeLabels(w.hostname || '', labels)}
                      idPrefix={`worker-${wi}-label`}
                    />
                  </LabeledField>
                </div>
              ))}
              <FormGroup fieldId="addWorker">
                <Button variant="secondary" icon={<PlusIcon />} onClick={addWorkerNode}>
                  Add worker node
                </Button>
              </FormGroup>
            </>
          ) : (
            <>
              <LabeledField label="Control node IP" isRequired fieldId="bootstrapHost" help={getFieldHelp('clusterJoin.bootstrapHost')}>
                {hostOptions.length > 0 && (
                  <FormSelect
                    id="bootstrapHostHost"
                    value={hostByName[form.nodes?.[0]?.hostname || ''] ? form.nodes?.[0]?.hostname : ''}
                    onChange={(_e, v) => applyHostToNode(0, 'control', v)}
                    aria-label="Control node inventory host"
                    style={{ marginBottom: '0.5rem' }}
                  >
                    <FormSelectOption value="" label="Select inventory host…" />
                    {slotHostOptions(form.nodes?.[0]?.hostname || '').map((h) => (
                      <FormSelectOption
                        key={h.name}
                        value={h.name}
                        label={`${h.name} — ${hostAddr(h)}${h.kind ? ` (${h.kind})` : ''}${h.status?.phase ? ` · ${h.status.phase}` : ''}`}
                      />
                    ))}
                  </FormSelect>
                )}
                <TextInput
                  id="bootstrapHost"
                  value={form.bootstrapHost}
                  onChange={(_e, v) =>
                    setForm((f) => ({
                      ...f,
                      bootstrapHost: v,
                      nodes: defaultSingleNode(f.clusterName, v).map((n, i) => ({
                        ...n,
                        labels: f.nodes?.[i]?.labels,
                      })),
                    }))
                  }
                  placeholder="198.51.100.50 or hostname"
                />
              </LabeledField>
              <LabeledField label="Node labels" fieldId="singleNodeLabels" help={getFieldHelp('clusterJoin.nodeLabels')}>
                <LabelKeyValueEditor
                  labels={form.nodes?.[0]?.labels || {}}
                  onChange={updateSingleNodeLabels}
                  idPrefix="single-node-label"
                />
              </LabeledField>
            </>
          )}
          <LabeledField label="Min root disk (GiB)" fieldId="minRootDiskGi" help={getFieldHelp('clusterJoin.minRootDiskGi')}>
            <TextInput
              id="minRootDiskGi"
              type="number"
              value={String(form.minRootDiskGi ?? 40)}
              onChange={(_e, v) => setForm({ ...form, minRootDiskGi: parseInt(v, 10) || 40 })}
            />
          </LabeledField>
          <LabeledField label="Region" fieldId="clusterRegion" help={getFieldHelp('clusterJoin.region')}>
            <TextInput
              id="clusterRegion"
              value={form.clusterRegion || 'fleet'}
              onChange={(_e, v) => setForm({ ...form, clusterRegion: v })}
            />
          </LabeledField>
          <LabeledField label="Cluster labels" fieldId="clusterLabels" help={getFieldHelp('clusterJoin.clusterLabels')}>
            <LabelKeyValueEditor
              labels={clusterLabels}
              onChange={setClusterLabels}
              idPrefix="cluster-label"
            />
          </LabeledField>
          <LabeledField label="Local path (RWO)" fieldId="localPath" help={getFieldHelp('clusterJoin.localPath')}>
            <Checkbox
              id="localPath"
              label="Enable K3s local-path provisioner (default)"
              isChecked={localPathEnabled}
              onChange={(_e, checked) => setLocalPathEnabled(checked)}
            />
          </LabeledField>
          {topology === 'single_node' && (
            <LabeledField
              label="Local RWX (NFS on this node)"
              fieldId="nfsLocalEnabled"
              help={getFieldHelp('clusterJoin.nfsLocal')}
            >
              <Checkbox
                id="nfsLocalEnabled"
                label="Install nfs-server on this node and nfs-client StorageClass"
                isChecked={nfsLocalEnabled}
                onChange={(_e, checked) => {
                  setNfsLocalEnabled(checked);
                  if (checked) setNfsEnabled(false);
                }}
              />
              <p style={{ fontSize: '0.85rem', marginTop: '0.35rem' }}>
                Evaluation only: exports <code>/srv/fleet-nfs</code> on the node and deploys
                StorageClass <code>nfs-client</code> during bootstrap.
              </p>
            </LabeledField>
          )}
          <LabeledField
            label="KubeVirt"
            fieldId="kubevirtEnabled"
            help={getFieldHelp('clusterJoin.kubevirt')}
          >
            <Checkbox
              id="kubevirtEnabled"
              label="Install KubeVirt platform after Ready (Multus + CDI + KubeVirt)"
              isChecked={!!form.providers?.kubevirt}
              onChange={(_e, checked) =>
                setForm({
                  ...form,
                  providers: {
                    ...(form.providers || {}),
                    kubevirt: checked,
                    kubevirtBridge: {
                      ...(form.providers?.kubevirtBridge || {
                        enabled: false,
                        bridgeName: 'mgmt-l2-br',
                        keepIp: true,
                      }),
                      enabled: checked ? !!form.providers?.kubevirtBridge?.enabled : false,
                    },
                  },
                })
              }
            />
          </LabeledField>
          {!!form.providers?.kubevirt && (
            <LabeledField
              label="LAN Multus bridge"
              fieldId="kubevirtBridge"
              help={getFieldHelp('clusterJoin.kubevirtBridgeName')}
            >
              <Checkbox
                id="kubevirtBridge"
                label="Configure host bridge at bootstrap for VMs on the node LAN"
                isChecked={!!form.providers?.kubevirtBridge?.enabled}
                onChange={(_e, checked) =>
                  setForm({
                    ...form,
                    providers: {
                      ...(form.providers || {}),
                      kubevirt: true,
                      kubevirtBridge: {
                        enabled: checked,
                        bridgeName: form.providers?.kubevirtBridge?.bridgeName || 'mgmt-l2-br',
                        slaveNic: form.providers?.kubevirtBridge?.slaveNic,
                        keepIp: form.providers?.kubevirtBridge?.keepIp !== false,
                      },
                    },
                  })
                }
              />
            </LabeledField>
          )}
          <LabeledField label="External NFS (RWX)" fieldId="nfsEnabled" help={getFieldHelp('clusterJoin.nfsEnabled')}>
            <Checkbox
              id="nfsEnabled"
              label="Install nfs-subdir-external-provisioner on a shared export"
              isChecked={nfsEnabled}
              isDisabled={nfsLocalEnabled && topology === 'single_node'}
              onChange={(_e, checked) => {
                setNfsEnabled(checked);
                if (checked) setNfsLocalEnabled(false);
              }}
            />
          </LabeledField>
          {nfsEnabled && (
            <>
              <NfsStorageSetupHelp
                clusterName={form.clusterName.trim() || 'my-cluster'}
                exportBase={nfsExportBase}
              />
              <LabeledField label="NFS server" fieldId="nfsServer" isRequired help={getFieldHelp('clusterJoin.nfsServer')}>
                <TextInput
                  id="nfsServer"
                  value={nfsServer}
                  onChange={(_e, v) => setNfsServer(v)}
                  placeholder="198.51.100.40"
                />
              </LabeledField>
              <LabeledField label="NFS export base" fieldId="nfsExportBase" help={getFieldHelp('clusterJoin.nfsExportBase')}>
                <TextInput
                  id="nfsExportBase"
                  value={nfsExportBase}
                  onChange={(_e, v) => setNfsExportBase(v)}
                  placeholder="/srv/fleet-nfs"
                />
              </LabeledField>
              <LabeledField label="Client mount path" fieldId="nfsMountPath" help={getFieldHelp('clusterJoin.nfsMountPath')}>
                <TextInput
                  id="nfsMountPath"
                  value={nfsMountPath}
                  onChange={(_e, v) => setNfsMountPath(v)}
                  placeholder={`${nfsExportBase.replace(/\/$/, '')}/${form.clusterName || 'cluster'}`}
                />
              </LabeledField>
              <LabeledField label="Cluster subdirectory" fieldId="nfsClusterPath" help={getFieldHelp('clusterJoin.nfsClusterPath')}>
                <TextInput id="nfsClusterPath" value={form.clusterName} readOnly />
              </LabeledField>
              <LabeledField label="Delegate host (SSH mkdir)" fieldId="nfsDelegateHost" help={getFieldHelp('clusterJoin.nfsDelegateHost')}>
                <TextInput
                  id="nfsDelegateHost"
                  value={nfsDelegateHost}
                  onChange={(_e, v) => setNfsDelegateHost(v)}
                  placeholder="NFS server IP when not a cluster node"
                />
              </LabeledField>
            </>
          )}
          {ingressOptions.length > 0 && (
            <LabeledField label="Site ingress" fieldId="ingressId" help={getFieldHelp('clusterJoin.domainBindings')}>
              <FormSelect
                id="ingressId"
                value={form.edge?.ingressId || autoIngressId || ''}
                onChange={(_e, v) =>
                  setForm({
                    ...form,
                    edge: { ingressId: v || undefined },
                  })
                }
              >
                <FormSelectOption value="" label="None" />
                {ingressOptions.map((ent) => (
                  <FormSelectOption
                    key={ent.id}
                    value={ent.id}
                    label={formatIngressOptionLabel(ent)}
                    isDisabled={ingressOptionDisabled(ent)}
                  />
                ))}
              </FormSelect>
              <p style={{ fontSize: '0.85rem', marginTop: '0.35rem' }}>
                Port-forward ingress allows one cluster; reverse-proxy rows stay available for
                multiple clusters.
              </p>
            </LabeledField>
          )}
          <p style={{ fontSize: '0.85rem', color: 'var(--pf-v6-global--Color--200)' }}>
            GPU and storage capabilities (RWO/RWX) are detected automatically after the cluster reaches Ready.
          </p>
        </Form>
      </Modal>
    </>
  );
}
