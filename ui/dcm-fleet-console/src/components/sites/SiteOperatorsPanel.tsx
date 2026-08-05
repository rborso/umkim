import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Button,
  Modal,
  ModalVariant,
  Title,
} from '@patternfly/react-core';
import {
  Table,
  Thead,
  Tr,
  Th,
  Tbody,
  Td,
} from '@patternfly/react-table';
import { PlusIcon } from '@patternfly/react-icons';
import { RowActionsMenu } from '../catalog/RowActionsMenu';
import { replicationSummary } from '../catalog/PlacementTargetsPanel';
import {
  catalogItemId,
  deleteCatalogItemInstance,
  getInstancePlacementTargets,
  instanceMatchesCluster,
  isOperatorCatalogItem,
  listCatalogItemInstances,
  listCatalogItems,
  listProviders,
  providerHealth,
} from '../../api/client';
import type { CatalogItem, CatalogItemInstance, PlacementTargetsResponse } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';

type Props = {
  siteName: string;
  sitePhase: string;
};

export function SiteOperatorsPanel({ siteName, sitePhase }: Props) {
  const navigate = useNavigate();
  const [instances, setInstances] = useState<CatalogItemInstance[]>([]);
  const [operatorCatalog, setOperatorCatalog] = useState<CatalogItem[]>([]);
  const [olmReady, setOlmReady] = useState(false);
  const [replication, setReplication] = useState<Record<string, PlacementTargetsResponse>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const [deleteInstance, setDeleteInstance] = useState<CatalogItemInstance | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<unknown>(null);

  const ready = sitePhase === 'Ready';

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [deployed, items, providers] = await Promise.all([
        listCatalogItemInstances(),
        listCatalogItems(),
        listProviders(),
      ]);
      const opCatalog = items.filter(isOperatorCatalogItem);
      setOperatorCatalog(opCatalog);

      const providerName = `olm-${siteName}`;
      setOlmReady(
        providers.some(
          (p) =>
            p.name === providerName &&
            (providerHealth(p) === 'ready' || providerHealth(p) === 'Ready'),
        ),
      );

      const opIds = new Set(
        opCatalog.map((c) => catalogItemId(c)).filter((id) => id !== '—'),
      );
      const opInstances = deployed.filter((i) =>
        opIds.has(i.spec?.catalog_item_id || ''),
      );

      const withId = opInstances.filter((i) => i.id);
      const repEntries = await Promise.all(
        withId.map(async (inst) => {
          try {
            const data = await getInstancePlacementTargets(inst.id!);
            return [inst.id!, data] as const;
          } catch {
            return [inst.id!, null] as const;
          }
        }),
      );
      const repMap: Record<string, PlacementTargetsResponse> = {};
      for (const [id, data] of repEntries) {
        if (data) repMap[id] = data;
      }
      setReplication(repMap);
      setInstances(
        opInstances.filter((inst) =>
          instanceMatchesCluster(repMap[inst.id || ''], siteName),
        ),
      );
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [siteName]);

  useEffect(() => {
    void load();
  }, [load]);

  const openDeploy = () => {
    const first = operatorCatalog[0];
    const q = new URLSearchParams({ deploy: '1', zone: siteName });
    if (first) {
      q.set('catalogItem', catalogItemId(first));
    }
    navigate(`/instances?${q.toString()}`);
  };

  const confirmDelete = async (mode: 'strict' | 'deferred' | 'abandon' = 'strict') => {
    if (!deleteInstance?.id) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deleteCatalogItemInstance(deleteInstance.id, { deletionMode: mode });
      setDeleteOpen(false);
      setDeleteInstance(null);
      await load();
    } catch (e) {
      setDeleteError(e);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <>
      <Title headingLevel="h2" size="lg" style={{ marginTop: '1.5rem' }}>
        Operator instances
      </Title>
      <p style={{ fontSize: '0.875rem', maxWidth: '40rem' }}>
        OLM operators on this site via provider <code>olm-{siteName}</code>. Deploy from{' '}
        <strong>Catalog instances</strong> using the same wizard as the hub.
        {!olmReady && ready && (
          <span
            style={{
              display: 'block',
              marginTop: '0.25rem',
              color: 'var(--pf-v5-global--warning-color--100)',
            }}
          >
            OLM provider not ready — re-run site provision or <code>fleet-olm-sp.yml</code>.
          </span>
        )}
      </p>
      <ErrorAlert error={error} />
      <div style={{ marginBottom: '0.5rem' }}>
        <Button
          variant="secondary"
          icon={<PlusIcon />}
          isDisabled={!ready || operatorCatalog.length === 0}
          onClick={openDeploy}
          data-testid={`site-${siteName}-install-operator`}
        >
          Deploy catalog instance
        </Button>
      </div>
      {loading ? (
        <p>Loading operator instances…</p>
      ) : (
        <Table aria-label="Site operator instances" variant="compact">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Catalog</Th>
              <Th>Status</Th>
              <Th />
            </Tr>
          </Thead>
          <Tbody>
            {instances.length === 0 ? (
              <Tr>
                <Td colSpan={4}>No operator instances on this site.</Td>
              </Tr>
            ) : (
              instances.map((inst) => {
                const id = inst.id || '';
                return (
                  <Tr key={id}>
                    <Td>{inst.display_name || '—'}</Td>
                    <Td>
                      <code style={{ fontSize: '0.75rem' }}>
                        {inst.spec?.catalog_item_id}
                      </code>
                    </Td>
                    <Td>{replicationSummary(replication[id])}</Td>
                    <Td>
                      <RowActionsMenu
                        actions={[
                          {
                            key: 'del',
                            label: 'Delete',
                            isDanger: true,
                            onClick: () => {
                              setDeleteInstance(inst);
                              setDeleteOpen(true);
                            },
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
      )}

      <Modal
        variant={ModalVariant.small}
        title="Delete operator instance?"
        isOpen={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        actions={[
          <Button
            key="d"
            variant="danger"
            onClick={() => void confirmDelete('strict')}
            isLoading={deleting}
          >
            Delete
          </Button>,
          <Button key="c" variant="link" onClick={() => setDeleteOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={deleteError} />
        <p>Remove instance {deleteInstance?.display_name}?</p>
        {deleteError ? (
          <div style={{ marginTop: '1rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            <Button
              variant="secondary"
              onClick={() => void confirmDelete('deferred')}
              isDisabled={deleting}
            >
              Retry later (deferred)
            </Button>
            <Button
              variant="danger"
              onClick={() => void confirmDelete('abandon')}
              isDisabled={deleting}
            >
              Force remove record
            </Button>
          </div>
        ) : null}
      </Modal>
    </>
  );
}
