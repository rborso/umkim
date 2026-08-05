import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  Button,
  Chip,
  ChipGroup,
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
import { PlusIcon, SyncIcon } from '@patternfly/react-icons';
import { RowActionsMenu } from '../components/catalog/RowActionsMenu';
import {
  catalogItemDisplayName,
  catalogItemId,
  catalogItemServiceType,
  createCatalogItem,
  deleteCatalogItem,
  listCatalogItemInstances,
  listCatalogItems,
  listServiceTypes,
  updateCatalogItem,
} from '../api/client';
import type { CatalogItem, CatalogItemCreate } from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { CatalogItemFormModal } from '../components/catalog/CatalogItemFormModal';
import { CatalogItemViewModal } from '../components/catalog/CatalogItemViewModal';
import { HelmChartImportModal } from '../components/catalog/HelmChartImportModal';
import { useAuth } from '../auth/AuthContext';

export function CatalogPage() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const [rows, setRows] = useState<CatalogItem[]>([]);
  const [instanceCounts, setInstanceCounts] = useState<Record<string, number>>({});
  const [serviceTypes, setServiceTypes] = useState<string[]>([
    'container',
    'operator',
    'helm',
    'cluster',
  ]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const [formOpen, setFormOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [editItem, setEditItem] = useState<CatalogItem | null>(null);
  const [viewItem, setViewItem] = useState<CatalogItem | null>(null);
  const [viewOpen, setViewOpen] = useState(false);
  const [deleteItem, setDeleteItem] = useState<CatalogItem | null>(null);
  const [deleteItemOpen, setDeleteItemOpen] = useState(false);
  const [deletingItem, setDeletingItem] = useState(false);
  const [deleteItemError, setDeleteItemError] = useState<unknown>(null);
  const [serviceTypeFilter, setServiceTypeFilter] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [items, instances, stypes] = await Promise.all([
        listCatalogItems(),
        listCatalogItemInstances(),
        listServiceTypes().catch(() => []),
      ]);
      setRows(items);
      const counts: Record<string, number> = {};
      for (const inst of instances) {
        const cid = inst.spec?.catalog_item_id || '';
        if (cid) counts[cid] = (counts[cid] || 0) + 1;
      }
      setInstanceCounts(counts);
      const fromApi = stypes
        .map((s) => s.service_type || s.name || '')
        .filter(Boolean);
      setServiceTypes((prev) => {
        const merged = new Set([...prev, ...fromApi]);
        return Array.from(merged).sort();
      });
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const saveCatalogItem = async (body: CatalogItemCreate, editId: string | null) => {
    if (editId && editId !== '—') {
      await updateCatalogItem(editId, body);
    } else {
      await createCatalogItem(body);
    }
    await load();
  };

  const confirmDeleteItem = async () => {
    const id = deleteItem ? catalogItemId(deleteItem) : '';
    if (!id || id === '—') return;
    setDeletingItem(true);
    setDeleteItemError(null);
    try {
      await deleteCatalogItem(id);
      setDeleteItemOpen(false);
      setDeleteItem(null);
      await load();
    } catch (e) {
      setDeleteItemError(e);
    } finally {
      setDeletingItem(false);
    }
  };

  const visibleRows = useMemo(
    () =>
      serviceTypeFilter
        ? rows.filter((c) => catalogItemServiceType(c) === serviceTypeFilter)
        : rows,
    [rows, serviceTypeFilter],
  );

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1" size="2xl">
          Catalog
        </Title>
        <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
          <strong>Templates only</strong> — catalog items define service types and default fields.
          Deploy workloads from{' '}
          <Link to="/instances">Catalog instances</Link>.
        </p>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              {user?.dcAdmin ? (
                <Button
                  variant="primary"
                  icon={<PlusIcon />}
                  onClick={() => {
                    setEditItem(null);
                    setFormOpen(true);
                  }}
                  data-testid="catalog-create-btn"
                >
                  Create catalog item
                </Button>
              ) : null}
            </ToolbarItem>
            <ToolbarItem>
              {user?.dcAdmin ? (
                <Button
                  variant="secondary"
                  onClick={() => setImportOpen(true)}
                  data-testid="catalog-import-helm-btn"
                >
                  Import Helm chart
                </Button>
              ) : null}
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={load}>
                Refresh
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <ChipGroup>
                <Chip
                  onClick={() =>
                    setServiceTypeFilter((f) => (f === 'operator' ? null : 'operator'))
                  }
                  isReadOnly={false}
                >
                  Operators{serviceTypeFilter === 'operator' ? ' ✓' : ''}
                </Chip>
              </ChipGroup>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </PageSection>

      <PageSection>
        <ErrorAlert error={error} />
        <Title headingLevel="h2" size="lg" style={{ marginBottom: '0.75rem' }}>
          Catalog items
        </Title>
        <Table aria-label="Catalog items" variant="compact">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Service type</Th>
              <Th>Instances</Th>
              <Th>ID</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {visibleRows.length === 0 && !loading ? (
              <Tr>
                <Td colSpan={5}>
                  {serviceTypeFilter
                    ? `No catalog items with service_type=${serviceTypeFilter}`
                    : 'No catalog items'}
                </Td>
              </Tr>
            ) : (
              visibleRows.map((c) => {
                const id = catalogItemId(c);
                const count = instanceCounts[id] ?? 0;
                return (
                  <Tr
                    key={id}
                    data-testid={id !== '—' ? `catalog-row-${id}` : undefined}
                    style={{ cursor: 'pointer' }}
                    onClick={() => {
                      setViewItem(c);
                      setViewOpen(true);
                    }}
                  >
                    <Td>
                      <Button
                        variant="link"
                        isInline
                        onClick={(e) => {
                          e.stopPropagation();
                          setViewItem(c);
                          setViewOpen(true);
                        }}
                        style={{ fontWeight: 600, padding: 0 }}
                      >
                        {catalogItemDisplayName(c)}
                      </Button>
                    </Td>
                    <Td>{catalogItemServiceType(c)}</Td>
                    <Td>
                      {count > 0 ? (
                        <Link to="/instances" onClick={(e) => e.stopPropagation()}>
                          {count}
                        </Link>
                      ) : (
                        '0'
                      )}
                    </Td>
                    <Td onClick={(e) => e.stopPropagation()}>
                      <code style={{ fontSize: '0.75rem' }}>{id}</code>
                    </Td>
                    <Td onClick={(e) => e.stopPropagation()}>
                      <RowActionsMenu
                        actions={[
                          {
                            key: `create-instance-${id}`,
                            label: 'Create instance',
                            onClick: () => {
                              navigate(
                                `/instances?catalogItem=${encodeURIComponent(id)}&deploy=1`,
                              );
                            },
                          },
                          {
                            key: `view-${id}`,
                            label: 'View details',
                            onClick: () => {
                              setViewItem(c);
                              setViewOpen(true);
                            },
                          },
                          {
                            key: `edit-${id}`,
                            label: 'Edit catalog item',
                            onClick: () => {
                              setEditItem(c);
                              setFormOpen(true);
                            },
                          },
                          {
                            key: `delete-${id}`,
                            label: 'Delete catalog item',
                            onClick: () => {
                              setDeleteItem(c);
                              setDeleteItemOpen(true);
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

      <CatalogItemFormModal
        isOpen={formOpen}
        editItem={editItem}
        serviceTypes={serviceTypes}
        onClose={() => setFormOpen(false)}
        onSave={saveCatalogItem}
      />

      <HelmChartImportModal
        isOpen={importOpen}
        onClose={() => setImportOpen(false)}
        onImport={async (body) => {
          await createCatalogItem(body);
          await load();
        }}
      />

      <CatalogItemViewModal
        item={viewItem}
        isOpen={viewOpen}
        onClose={() => setViewOpen(false)}
        onEdit={(c) => {
          setViewOpen(false);
          setEditItem(c);
          setFormOpen(true);
        }}
        createInstanceHref={
          viewItem
            ? `/instances?catalogItem=${encodeURIComponent(catalogItemId(viewItem))}&deploy=1`
            : undefined
        }
      />

      <Modal
        variant={ModalVariant.small}
        title="Delete catalog item?"
        isOpen={deleteItemOpen}
        onClose={() => setDeleteItemOpen(false)}
        actions={[
          <Button
            key="del"
            variant="danger"
            onClick={confirmDeleteItem}
            isLoading={deletingItem}
          >
            Delete
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteItemOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={deleteItemError} />
        <p>
          Delete <strong>{deleteItem ? catalogItemDisplayName(deleteItem) : ''}</strong>?
          Existing instances may fail if they reference this item.
        </p>
      </Modal>
    </>
  );
}
