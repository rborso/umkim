import { Link } from 'react-router-dom';
import { Button, DescriptionList, DescriptionListGroup, DescriptionListTerm, DescriptionListDescription, Modal, ModalVariant } from '@patternfly/react-core';
import type { CatalogItem } from '../../api/types';
import { catalogItemDisplayName, catalogItemId, catalogItemServiceType } from '../../api/client';
import { fieldsFromCatalogItem } from '../../catalog/fieldUtils';

type Props = {
  item: CatalogItem | null;
  isOpen: boolean;
  onClose: () => void;
  onEdit?: (item: CatalogItem) => void;
  /** Link to catalog instances deploy wizard (templates-only catalog). */
  createInstanceHref?: string;
};

export function CatalogItemViewModal({
  item,
  isOpen,
  onClose,
  onEdit,
  createInstanceHref,
}: Props) {
  if (!item) return null;
  const fields = fieldsFromCatalogItem(item);

  return (
    <Modal
      variant={ModalVariant.medium}
      title={`Catalog item: ${catalogItemDisplayName(item)}`}
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        createInstanceHref ? (
          <Link key="deploy" to={createInstanceHref}>
            <Button variant="primary">Create instance</Button>
          </Link>
        ) : null,
        onEdit ? (
          <Button key="edit" variant="secondary" onClick={() => onEdit(item)}>
            Edit
          </Button>
        ) : null,
        <Button key="close" variant="link" onClick={onClose}>
          Close
        </Button>,
      ]}
    >
      <DescriptionList isCompact>
        <DescriptionListGroup>
          <DescriptionListTerm>ID</DescriptionListTerm>
          <DescriptionListDescription>
            <code>{catalogItemId(item)}</code>
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Service type</DescriptionListTerm>
          <DescriptionListDescription>{catalogItemServiceType(item)}</DescriptionListDescription>
        </DescriptionListGroup>
        {item.description ? (
          <DescriptionListGroup>
            <DescriptionListTerm>Description</DescriptionListTerm>
            <DescriptionListDescription>{item.description}</DescriptionListDescription>
          </DescriptionListGroup>
        ) : null}
      </DescriptionList>
      {fields.length > 0 ? (
        <>
          <h3 style={{ marginTop: '1rem', fontSize: '1rem' }}>Fields</h3>
          <table style={{ width: '100%', fontSize: '0.875rem' }}>
            <thead>
              <tr>
                <th align="left">Path</th>
                <th align="left">Default</th>
                <th align="left">Editable</th>
              </tr>
            </thead>
            <tbody>
              {fields.map((f) => (
                <tr key={f.path}>
                  <td>
                    <code>{f.path}</code>
                  </td>
                  <td>{f.default !== undefined ? String(f.default) : '—'}</td>
                  <td>{f.editable === false ? 'no' : 'yes'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      ) : null}
      <h3 style={{ marginTop: '1rem', fontSize: '1rem' }}>Spec (JSON)</h3>
      <pre
        style={{
          fontSize: '0.75rem',
          maxHeight: '12rem',
          overflow: 'auto',
          background: 'var(--pf-v5-global--BackgroundColor--200)',
          padding: '0.5rem',
        }}
      >
        {JSON.stringify(item.spec ?? {}, null, 2)}
      </pre>
    </Modal>
  );
}
