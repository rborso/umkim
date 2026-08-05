import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  Button,
  Label,
  Modal,
  ModalVariant,
  PageSection,
  PageSectionVariants,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import {
  Table,
  Thead,
  Tr,
  Th,
  Tbody,
  Td,
} from '@patternfly/react-table';
import { PlayIcon, SyncIcon } from '@patternfly/react-icons';
import { RowActionsMenu } from '../components/catalog/RowActionsMenu';
import {
  catalogItemId,
  catalogItemServiceType,
  deleteCatalogItemInstance,
  fleetDefaultWorkloadSiteId,
  getInstanceHubRoute,
  getInstancePlacementTargets,
  listCatalogItemInstances,
  listCatalogItems,
  listFleetSites,
} from '../api/client';
import type {
  CatalogItem,
  CatalogItemInstance,
  FleetSite,
  InstanceHubRouteResponse,
  PlacementTargetsResponse,
} from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { InstanceDeployModal } from '../components/catalog/InstanceDeployModal';
import { InstanceViewModal } from '../components/catalog/InstanceViewModal';
import { InstanceBackupModal } from '../components/catalog/InstanceBackupModal';
import { InstanceRestoreModal } from '../components/catalog/InstanceRestoreModal';
import {
  hubRouteApplicable,
  hubRouteSummary,
  replicationSummary,
} from '../components/catalog/PlacementTargetsPanel';

export function InstancesPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [instances, setInstances] = useState<CatalogItemInstance[]>([]);
  const [catalogItems, setCatalogItems] = useState<CatalogItem[]>([]);
  const [catalogById, setCatalogById] = useState<Record<string, CatalogItem>>({});
  const [fleetSites, setFleetSites] = useState<FleetSite[]>([]);
  const [replication, setReplication] = useState<Record<string, PlacementTargetsResponse>>({});
  const [hubRoutes, setHubRoutes] = useState<Record<string, InstanceHubRouteResponse | null>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const [deployOpen, setDeployOpen] = useState(false);
  const [deployCatalogItem, setDeployCatalogItem] = useState<CatalogItem | null>(null);
  const [deployLabelSelector, setDeployLabelSelector] = useState<Record<string, string> | undefined>();
  const [lockCatalogItem, setLockCatalogItem] = useState(false);

  const [viewInstance, setViewInstance] = useState<CatalogItemInstance | null>(null);
  const [viewInstanceOpen, setViewInstanceOpen] = useState(false);
  const [deleteInstance, setDeleteInstance] = useState<CatalogItemInstance | null>(null);
  const [deleteInstanceOpen, setDeleteInstanceOpen] = useState(false);
  const [deletingInstance, setDeletingInstance] = useState(false);
  const [deleteInstanceError, setDeleteInstanceError] = useState<unknown>(null);
  const [backupInstance, setBackupInstance] = useState<CatalogItemInstance | null>(null);
  const [backupOpen, setBackupOpen] = useState(false);
  const [restoreInstance, setRestoreInstance] = useState<CatalogItemInstance | null>(null);
  const [restoreOpen, setRestoreOpen] = useState(false);

  const loadReplication = async (deployed: CatalogItemInstance[]) => {
    const withResource = deployed.filter((i) => i.id && i.resource_id);
    const entries = await Promise.all(
      withResource.map(async (inst) => {
        try {
          const data = await getInstancePlacementTargets(inst.id!);
          return [inst.id!, data] as const;
        } catch {
          return [inst.id!, null] as const;
        }
      }),
    );
    const map: Record<string, PlacementTargetsResponse> = {};
    for (const [id, data] of entries) {
      if (data) map[id] = data;
    }
    setReplication(map);

    const withId = deployed.filter((i) => i.id);
    const routeEntries = await Promise.all(
      withId.map(async (inst) => {
        try {
          const route = await getInstanceHubRoute(inst.id!);
          return [inst.id!, route] as const;
        } catch {
          return [inst.id!, null] as const;
        }
      }),
    );
    const routeMap: Record<string, InstanceHubRouteResponse | null> = {};
    for (const [id, route] of routeEntries) {
      routeMap[id] = route;
    }
    setHubRoutes(routeMap);
  };

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [deployed, items, sites] = await Promise.all([
        listCatalogItemInstances(),
        listCatalogItems(),
        listFleetSites(),
      ]);
      setInstances(deployed);
      setCatalogItems(items);
      setFleetSites(sites);
      const byId: Record<string, CatalogItem> = {};
      for (const c of items) {
        const id = catalogItemId(c);
        if (id !== '—') byId[id] = c;
      }
      setCatalogById(byId);
      await loadReplication(deployed);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openDeploy = useCallback(
    (item: CatalogItem | null, labelSelector?: Record<string, string>, locked = false) => {
      setDeployCatalogItem(item);
      setDeployLabelSelector(labelSelector);
      setLockCatalogItem(locked);
      setDeployOpen(true);
    },
    [],
  );

  useEffect(() => {
    const catalogItemIdParam = searchParams.get('catalogItem');
    const shouldDeploy = searchParams.get('deploy') === '1';
    const zone = searchParams.get('zone');
    const siteParam = searchParams.get('site');
    const site = siteParam || fleetDefaultWorkloadSiteId(fleetSites);
    if (!shouldDeploy || catalogItems.length === 0) return;

    const item =
      (catalogItemIdParam && catalogById[catalogItemIdParam]) ||
      catalogItems[0] ||
      null;
    const labelSelector: Record<string, string> | undefined =
      site || zone
        ? {
            ...(zone ? { zone } : {}),
            ...(site ? { 'fleet.site': site } : {}),
          }
        : undefined;
    openDeploy(item, labelSelector, Boolean(catalogItemIdParam && item));

    const next = new URLSearchParams(searchParams);
    next.delete('deploy');
    setSearchParams(next, { replace: true });
  }, [searchParams, catalogItems, catalogById, fleetSites, openDeploy, setSearchParams]);

  const serviceTypeForInstance = (inst: CatalogItemInstance): string => {
    const cid = inst.spec?.catalog_item_id || '';
    const cat = catalogById[cid];
    return cat ? catalogItemServiceType(cat) : '—';
  };

  const confirmDeleteInstance = async (mode: 'strict' | 'deferred' | 'abandon' = 'strict') => {
    if (!deleteInstance?.id) return;
    setDeletingInstance(true);
    setDeleteInstanceError(null);
    try {
      await deleteCatalogItemInstance(deleteInstance.id, { deletionMode: mode });
      setDeleteInstanceOpen(false);
      setDeleteInstance(null);
      await load();
    } catch (e) {
      setDeleteInstanceError(e);
    } finally {
      setDeletingInstance(false);
    }
  };

  const catalogItemOptions = useMemo(() => catalogItems, [catalogItems]);

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1" size="2xl">
          Catalog instances
        </Title>
        <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
          All deployed workloads (containers, Helm, operators, clusters). Choose a{' '}
          <Link to="/catalog">catalog template</Link> then deploy here. Replication status
          reflects <Link to="/policies">policy</Link> placement.
        </p>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button
                variant="primary"
                icon={<PlayIcon />}
                onClick={() => openDeploy(catalogItems[0] ?? null)}
                isDisabled={catalogItems.length === 0}
                data-testid="instances-deploy-toolbar"
              >
                Deploy instance
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={load}>
                Refresh
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </PageSection>

      <PageSection>
        <ErrorAlert error={error} />
        <Table aria-label="Catalog instances" variant="compact">
          <Thead>
            <Tr>
              <Th>Display name</Th>
              <Th>Service type</Th>
              <Th>Catalog item</Th>
              <Th>Resource ID</Th>
              <Th>Replication</Th>
              <Th>Active cluster</Th>
              <Th>Hub route</Th>
              <Th>Instance ID</Th>
              <Th>Created</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {instances.length === 0 && !loading ? (
              <Tr>
                <Td colSpan={10}>No catalog instances deployed.</Td>
              </Tr>
            ) : (
              instances.map((inst) => {
                const id = inst.id || inst.uid || '';
                const rep = id ? replication[id] : undefined;
                const route = id ? hubRoutes[id] : undefined;
                const st = serviceTypeForInstance(inst);
                const cid = inst.spec?.catalog_item_id || '';
                const cat = catalogById[cid];
                return (
                  <Tr key={id}>
                    <Td>{inst.display_name || '—'}</Td>
                    <Td>
                      {st !== '—' ? (
                        <Label color={st === 'operator' ? 'blue' : 'grey'} isCompact>
                          {st}
                        </Label>
                      ) : (
                        '—'
                      )}
                    </Td>
                    <Td>
                      {cat ? (
                        <span>{cat.display_name || cid}</span>
                      ) : (
                        <code style={{ fontSize: '0.75rem' }}>{cid || '—'}</code>
                      )}
                    </Td>
                    <Td>
                      <code style={{ fontSize: '0.75rem' }}>{inst.resource_id || '—'}</code>
                    </Td>
                    <Td>{replicationSummary(rep)}</Td>
                    <Td>{route?.active_cluster || '—'}</Td>
                    <Td>
                      {route ? (
                        <Label
                          color={
                            !hubRouteApplicable(route)
                              ? route.effective_mode === 'site'
                                ? 'green'
                                : 'grey'
                              : route.failover_phase === 'Relocating'
                                ? 'blue'
                                : route.hub_route_healthy
                                  ? 'green'
                                  : route.failover_phase === 'Degraded' ||
                                      route.hub_route_healthy === false
                                    ? 'red'
                                    : 'grey'
                          }
                          isCompact
                        >
                          {hubRouteSummary(route)}
                        </Label>
                      ) : (
                        '—'
                      )}
                    </Td>
                    <Td>
                      <code style={{ fontSize: '0.75rem' }}>{id || '—'}</code>
                    </Td>
                    <Td>
                      {inst.create_time ? new Date(inst.create_time).toLocaleString() : '—'}
                    </Td>
                    <Td>
                      <RowActionsMenu
                        actions={[
                          {
                            key: `backup-${id}`,
                            label: 'Backup',
                            onClick: () => {
                              setBackupInstance(inst);
                              setBackupOpen(true);
                            },
                          },
                          {
                            key: `restore-${id}`,
                            label: 'Restore',
                            onClick: () => {
                              setRestoreInstance(inst);
                              setRestoreOpen(true);
                            },
                          },
                          {
                            key: `view-${id}`,
                            label: 'View instance',
                            onClick: () => {
                              setViewInstance(inst);
                              setViewInstanceOpen(true);
                            },
                          },
                          {
                            key: `delete-${id}`,
                            label: 'Delete instance',
                            onClick: () => {
                              setDeleteInstance(inst);
                              setDeleteInstanceOpen(true);
                            },
                            isDanger: true,
                          },
                        ]}
                      />
                    </Td>
                  </Tr>
                );
              })
            )}
          </Tbody>
        </Table>
      </PageSection>

      <InstanceDeployModal
        isOpen={deployOpen}
        catalogItem={deployCatalogItem}
        catalogItems={catalogItemOptions}
        initialLabelSelector={deployLabelSelector}
        lockCatalogItem={lockCatalogItem}
        onClose={() => {
          setDeployOpen(false);
          setDeployLabelSelector(undefined);
          setLockCatalogItem(false);
        }}
        onDeployed={load}
      />

      <InstanceViewModal
        instance={viewInstance}
        isOpen={viewInstanceOpen}
        onClose={() => setViewInstanceOpen(false)}
        catalogItems={catalogItems}
      />

      <InstanceBackupModal
        instance={backupInstance}
        isOpen={backupOpen}
        onClose={() => {
          setBackupOpen(false);
          setBackupInstance(null);
        }}
        onStarted={load}
      />

      <InstanceRestoreModal
        instance={restoreInstance}
        isOpen={restoreOpen}
        onClose={() => {
          setRestoreOpen(false);
          setRestoreInstance(null);
        }}
        onStarted={load}
      />

      <Modal
        variant={ModalVariant.small}
        title="Delete instance?"
        isOpen={deleteInstanceOpen}
        onClose={() => setDeleteInstanceOpen(false)}
        actions={[
          <Button
            key="del"
            variant="danger"
            onClick={() => void confirmDeleteInstance('strict')}
            isLoading={deletingInstance}
          >
            Delete
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteInstanceOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={deleteInstanceError} />
        <p>
          Delete instance <strong>{deleteInstance?.display_name || deleteInstance?.id}</strong>?
          This removes the placement record; workload cleanup depends on the service provider.
        </p>
        {deleteInstanceError ? (
          <div style={{ marginTop: '1rem', display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
            <p style={{ margin: 0, fontSize: '0.875rem' }}>
              Provider uninstall failed. Retry later (deferred cleanup) or force-remove the
              catalog record if the cluster/provider is gone.
            </p>
            <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
              <Button
                variant="secondary"
                onClick={() => void confirmDeleteInstance('deferred')}
                isDisabled={deletingInstance}
              >
                Retry later (deferred)
              </Button>
              <Button
                variant="danger"
                onClick={() => void confirmDeleteInstance('abandon')}
                isDisabled={deletingInstance}
              >
                Force remove record
              </Button>
            </div>
          </div>
        ) : null}
      </Modal>
    </>
  );
}
