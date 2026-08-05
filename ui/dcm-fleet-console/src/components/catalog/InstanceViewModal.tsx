import { useEffect, useState } from 'react';
import {
  Button,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  Modal,
  ModalVariant,
  Spinner,
} from '@patternfly/react-core';
import type { CatalogItem, CatalogItemInstance } from '../../api/types';
import { getCatalogItem, getCatalogItemInstance } from '../../api/client';
import { fieldsFromCatalogItem } from '../../catalog/fieldUtils';
import { redactUserValues } from '../../catalog/fieldSecrets';
import { valuesToDisplayString } from '../../catalog/valuesMerge';
import { ErrorAlert } from '../ErrorAlert';
import { HelmInstancePanel } from './HelmInstancePanel';
import { PlacementTargetsPanel } from './PlacementTargetsPanel';

type Props = {
  instance: CatalogItemInstance | null;
  isOpen: boolean;
  onClose: () => void;
  catalogItems?: CatalogItem[];
};

export function InstanceViewModal({ instance, isOpen, onClose, catalogItems }: Props) {
  const [full, setFull] = useState<CatalogItemInstance | null>(null);
  const [catalogItem, setCatalogItem] = useState<CatalogItem | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!isOpen || !instance?.id) {
      setFull(instance);
      return;
    }
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const data = await getCatalogItemInstance(instance.id!);
        if (!cancelled) {
          setFull(data);
          const cid = data.spec?.catalog_item_id;
          if (cid) {
            const fromList = catalogItems?.find((c) => c.id === cid || c.uid === cid);
            if (fromList) setCatalogItem(fromList);
            else {
              try {
                setCatalogItem(await getCatalogItem(cid));
              } catch {
                setCatalogItem(null);
              }
            }
          }
        }
      } catch (e) {
        if (!cancelled) {
          setFull(instance);
          setError(e);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [isOpen, instance, catalogItems]);

  const inst = full || instance;
  const serviceType =
    (catalogItem?.spec as { service_type?: string } | undefined)?.service_type ||
    (catalogItems?.find(
      (c: CatalogItem) =>
        c.id === inst?.spec?.catalog_item_id || c.uid === inst?.spec?.catalog_item_id,
    )?.spec as { service_type?: string } | undefined)?.service_type;
  if (!inst) return null;

  const catalogFields = catalogItem ? fieldsFromCatalogItem(catalogItem) : [];
  const userValues = redactUserValues(inst.spec?.user_values ?? [], catalogFields);

  return (
    <Modal
      variant={ModalVariant.medium}
      title={`Instance: ${inst.display_name || inst.id}`}
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        <Button key="close" variant="primary" onClick={onClose}>
          Close
        </Button>,
      ]}
    >
      <ErrorAlert error={error} />
      {loading ? <Spinner /> : null}
      <DescriptionList isCompact>
        <DescriptionListGroup>
          <DescriptionListTerm>Instance ID</DescriptionListTerm>
          <DescriptionListDescription>
            <code>{inst.id || inst.uid}</code>
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Catalog item</DescriptionListTerm>
          <DescriptionListDescription>
            <code>{inst.spec?.catalog_item_id || '—'}</code>
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Resource ID</DescriptionListTerm>
          <DescriptionListDescription>
            <code>{inst.resource_id || '—'}</code>
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Created</DescriptionListTerm>
          <DescriptionListDescription>
            {inst.create_time ? new Date(inst.create_time).toLocaleString() : '—'}
          </DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
      {inst.id ? <PlacementTargetsPanel instanceId={inst.id} /> : null}
      {userValues.length > 0 ? (
        <>
          <h3 style={{ marginTop: '1rem', fontSize: '1rem' }}>User values</h3>
          <table style={{ width: '100%', fontSize: '0.875rem' }}>
            <thead>
              <tr>
                <th align="left">Path</th>
                <th align="left">Value</th>
              </tr>
            </thead>
            <tbody>
              {userValues.map((uv) => (
                <tr key={uv.path}>
                  <td>
                    <code>{uv.path}</code>
                  </td>
                  <td>
                    {uv.path === 'values' &&
                    typeof uv.value === 'object' &&
                    uv.value !== null ? (
                      <pre
                        style={{
                          margin: 0,
                          maxHeight: '8rem',
                          overflow: 'auto',
                          fontSize: '0.75rem',
                          whiteSpace: 'pre-wrap',
                        }}
                      >
                        {valuesToDisplayString(uv.value as Record<string, unknown>)}
                      </pre>
                    ) : (
                      JSON.stringify(uv.value)
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      ) : null}
      {serviceType === 'helm' && inst.id ? (
        <HelmInstancePanel instance={inst} />
      ) : null}
      <h3 style={{ marginTop: '1rem', fontSize: '1rem' }}>Raw</h3>
      <pre
        style={{
          fontSize: '0.75rem',
          maxHeight: '10rem',
          overflow: 'auto',
          background: 'var(--pf-v5-global--BackgroundColor--200)',
          padding: '0.5rem',
        }}
      >
        {JSON.stringify(inst, null, 2)}
      </pre>
    </Modal>
  );
}
