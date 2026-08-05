import { useEffect, useState } from 'react';
import {
  Button,
  Form,
  FormGroup,
  Modal,
  ModalVariant,
  TextInput,
  TextArea,
  FormSelect,
  FormSelectOption,
  Checkbox,
} from '@patternfly/react-core';
import { PlusIcon, TrashIcon } from '@patternfly/react-icons';
import type { CatalogFieldDef, CatalogFieldUiTier, CatalogFieldUiWidget, CatalogItem, CatalogItemCreate } from '../../api/types';
import { catalogItemDisplayName, catalogItemId } from '../../api/client';
import { fieldsFromCatalogItem } from '../../catalog/fieldUtils';
import { getFieldHelp } from '../../help/fieldHelp';
import { ErrorAlert } from '../ErrorAlert';
import { LabeledField } from '../forms/LabeledField';

type Props = {
  isOpen: boolean;
  editItem: CatalogItem | null;
  serviceTypes: string[];
  onClose: () => void;
  onSave: (body: CatalogItemCreate, editId: string | null) => Promise<void>;
};

const UI_TIERS: CatalogFieldUiTier[] = ['required', 'common', 'advanced', 'hidden'];
const UI_WIDGETS: CatalogFieldUiWidget[] = [
  'text',
  'select',
  'password',
  'boolean',
  'number',
  'domain',
  'hostname',
  'dns-label',
  'yaml',
];

function enumToText(field: CatalogFieldDef): string {
  const enums = field.validation_schema?.enum;
  if (!enums || enums.length === 0) return '';
  return enums.map(String).join(', ');
}

function textToEnum(text: string): string[] | undefined {
  const parts = text
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
  return parts.length > 0 ? parts : undefined;
}

const emptyField = (): CatalogFieldDef => ({
  path: '',
  display_name: '',
  description: '',
  example: '',
  editable: true,
  default: '',
  validation_schema: { type: 'string' },
});

export function CatalogItemFormModal({
  isOpen,
  editItem,
  serviceTypes,
  onClose,
  onSave,
}: Props) {
  const [displayName, setDisplayName] = useState('');
  const [serviceType, setServiceType] = useState('container');
  const [description, setDescription] = useState('');
  const [fields, setFields] = useState<CatalogFieldDef[]>([]);
  const [workloadReplicas, setWorkloadReplicas] = useState(1);
  const [advancedJson, setAdvancedJson] = useState('');
  const [useAdvanced, setUseAdvanced] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);

  useEffect(() => {
    if (!isOpen) return;
    if (editItem) {
      setDisplayName(catalogItemDisplayName(editItem));
      setServiceType(
        (editItem.spec as { service_type?: string })?.service_type ||
          editItem.service_type ||
          'container',
      );
      setDescription(editItem.description || '');
      setFields(fieldsFromCatalogItem(editItem));
      setWorkloadReplicas(
        Number((editItem.spec as { workload?: { replicas?: number } })?.workload?.replicas) || 1,
      );
      setAdvancedJson(JSON.stringify(editItem.spec ?? {}, null, 2));
      setUseAdvanced(false);
    } else {
      setDisplayName('');
      setServiceType('container');
      setDescription('');
      setFields([]);
      setWorkloadReplicas(1);
      setAdvancedJson('{\n  "service_type": "container",\n  "fields": [],\n  "workload": { "replicas": 1 }\n}');
      setUseAdvanced(false);
    }
    setSaveError(null);
  }, [isOpen, editItem]);

  const save = async () => {
    setSaving(true);
    setSaveError(null);
    try {
      if (!displayName.trim()) throw new Error('Display name is required');
      let spec: Record<string, unknown>;
      if (useAdvanced) {
        spec = JSON.parse(advancedJson) as Record<string, unknown>;
      } else {
        spec = {
          service_type: serviceType,
          fields: fields.filter((f) => f.path.trim()),
          workload: { replicas: workloadReplicas > 0 ? workloadReplicas : 1 },
        };
      }
      const body: CatalogItemCreate = {
        display_name: displayName.trim(),
        description: description.trim() || undefined,
        spec,
      };
      await onSave(body, editItem ? catalogItemId(editItem) : null);
      onClose();
    } catch (e) {
      setSaveError(e);
    } finally {
      setSaving(false);
    }
  };

  const updateField = (idx: number, patch: Partial<CatalogFieldDef>) => {
    const next = [...fields];
    next[idx] = { ...next[idx], ...patch };
    setFields(next);
  };

  const updateFieldEnum = (idx: number, text: string) => {
    const field = fields[idx];
    const schema = { ...(field.validation_schema || {}) };
    const enums = textToEnum(text);
    if (enums) {
      schema.enum = enums;
      schema.type = schema.type || 'string';
    } else {
      delete schema.enum;
    }
    updateField(idx, { validation_schema: schema });
  };

  const updateFieldUiMeta = (
    idx: number,
    meta: { tier?: CatalogFieldUiTier | ''; widget?: CatalogFieldUiWidget | '' },
  ) => {
    const field = fields[idx];
    const schema = { ...(field.validation_schema || {}) };
    if (meta.tier !== undefined) {
      if (meta.tier) schema['x-ui-tier'] = meta.tier;
      else delete schema['x-ui-tier'];
    }
    if (meta.widget !== undefined) {
      if (meta.widget) schema['x-ui-widget'] = meta.widget;
      else delete schema['x-ui-widget'];
    }
    updateField(idx, {
      validation_schema: schema,
      ui_tier: meta.tier ? meta.tier : undefined,
      ui_widget: meta.widget ? meta.widget : undefined,
    });
  };

  return (
    <Modal
      variant={ModalVariant.large}
      title={editItem ? 'Edit catalog item' : 'Create catalog item'}
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        <Button key="save" variant="primary" onClick={save} isLoading={saving}>
          Save
        </Button>,
        <Button key="cancel" variant="link" onClick={onClose}>
          Cancel
        </Button>,
      ]}
    >
      <ErrorAlert error={saveError} />
      <Form>
        <LabeledField
          label="Display name"
          isRequired
          fieldId="ci-display"
          help={getFieldHelp('catalog.displayName')}
        >
          <TextInput
            id="ci-display"
            value={displayName}
            onChange={(_e, v) => setDisplayName(v)}
            isDisabled={!!editItem}
          />
        </LabeledField>
        <LabeledField
          label="Service type"
          isRequired
          fieldId="ci-stype"
          help={getFieldHelp('catalog.serviceType')}
        >
          <FormSelect
            id="ci-stype"
            value={serviceType}
            onChange={(_e, v) => setServiceType(v)}
            isDisabled={useAdvanced}
          >
            {serviceTypes.map((st) => (
              <FormSelectOption key={st} value={st} label={st} />
            ))}
          </FormSelect>
        </LabeledField>
        <LabeledField
          label="Description"
          fieldId="ci-desc"
          help={getFieldHelp('catalog.description')}
        >
          <TextInput
            id="ci-desc"
            value={description}
            onChange={(_e, v) => setDescription(v)}
          />
        </LabeledField>
        {!useAdvanced ? (
          <LabeledField
            label="Replicas per placed cluster"
            fieldId="ci-replicas"
            help={getFieldHelp('catalog.replicas')}
            helperText="Pod count on each cluster after placement (not total fleet-wide)"
          >
            <TextInput
              id="ci-replicas"
              type="number"
              value={String(workloadReplicas)}
              onChange={(_e, v) => setWorkloadReplicas(v === '' ? 1 : Number(v))}
              data-testid="catalog-workload-replicas"
            />
          </LabeledField>
        ) : null}
        <FormGroup fieldId="ci-adv">
          <Checkbox
            id="ci-adv"
            label="Edit raw spec JSON (advanced)"
            isChecked={useAdvanced}
            onChange={(_e, checked) => setUseAdvanced(checked)}
          />
        </FormGroup>
        {useAdvanced ? (
          <LabeledField label="Spec (JSON)" fieldId="ci-spec">
            <TextArea
              id="ci-spec"
              value={advancedJson}
              onChange={(_e, v) => setAdvancedJson(v)}
              rows={12}
              resizeOrientation="vertical"
            />
          </LabeledField>
        ) : (
          <>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <strong>Catalog fields</strong>
              <Button
                variant="secondary"
                size="sm"
                icon={<PlusIcon />}
                onClick={() => setFields([...fields, emptyField()])}
              >
                Add field
              </Button>
            </div>
            {fields.map((f, idx) => (
              <div
                key={idx}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 1fr 1fr 1fr 1fr 1fr 1fr 1fr auto',
                  gap: '0.5rem',
                  marginTop: '0.5rem',
                  alignItems: 'end',
                }}
              >
                <LabeledField label="Path" fieldId={`cf-path-${idx}`} help={getFieldHelp('catalog.fieldPath')}>
                  <TextInput
                    id={`cf-path-${idx}`}
                    value={f.path}
                    onChange={(_e, v) => updateField(idx, { path: v })}
                  />
                </LabeledField>
                <LabeledField label="Display name" fieldId={`cf-dn-${idx}`} help={getFieldHelp('catalog.fieldDisplayName')}>
                  <TextInput
                    id={`cf-dn-${idx}`}
                    value={f.display_name || ''}
                    onChange={(_e, v) => updateField(idx, { display_name: v })}
                  />
                </LabeledField>
                <LabeledField label="Description" fieldId={`cf-desc-${idx}`} help={getFieldHelp('catalog.fieldDescription')}>
                  <TextInput
                    id={`cf-desc-${idx}`}
                    value={f.description || ''}
                    onChange={(_e, v) => updateField(idx, { description: v })}
                  />
                </LabeledField>
                <LabeledField label="Example" fieldId={`cf-ex-${idx}`} help={getFieldHelp('catalog.fieldExample')}>
                  <TextInput
                    id={`cf-ex-${idx}`}
                    value={f.example || ''}
                    onChange={(_e, v) => updateField(idx, { example: v })}
                  />
                </LabeledField>
                <LabeledField label="Default" fieldId={`cf-def-${idx}`} help={getFieldHelp('catalog.fieldDefault')}>
                  <TextInput
                    id={`cf-def-${idx}`}
                    value={f.default !== undefined ? String(f.default) : ''}
                    onChange={(_e, v) => updateField(idx, { default: v })}
                  />
                </LabeledField>
                <LabeledField label="Allowed values" fieldId={`cf-enum-${idx}`} help={getFieldHelp('catalog.fieldAllowedValues')}>
                  <TextInput
                    id={`cf-enum-${idx}`}
                    value={enumToText(f)}
                    onChange={(_e, v) => updateFieldEnum(idx, v)}
                    placeholder="hub, site, none"
                  />
                </LabeledField>
                <LabeledField label="UI tier" fieldId={`cf-tier-${idx}`} help={getFieldHelp('catalog.fieldUiTier')}>
                  <FormSelect
                    id={`cf-tier-${idx}`}
                    value={
                      f.ui_tier ||
                      (f.validation_schema?.['x-ui-tier'] as string | undefined) ||
                      ''
                    }
                    onChange={(_e, v) =>
                      updateFieldUiMeta(idx, { tier: v as CatalogFieldUiTier | '' })
                    }
                  >
                    <FormSelectOption value="" label="(infer)" />
                    {UI_TIERS.map((t) => (
                      <FormSelectOption key={t} value={t} label={t} />
                    ))}
                  </FormSelect>
                </LabeledField>
                <LabeledField label="UI widget" fieldId={`cf-widget-${idx}`} help={getFieldHelp('catalog.fieldUiWidget')}>
                  <FormSelect
                    id={`cf-widget-${idx}`}
                    value={
                      f.ui_widget ||
                      (f.validation_schema?.['x-ui-widget'] as string | undefined) ||
                      ''
                    }
                    onChange={(_e, v) =>
                      updateFieldUiMeta(idx, { widget: v as CatalogFieldUiWidget | '' })
                    }
                  >
                    <FormSelectOption value="" label="(infer)" />
                    {UI_WIDGETS.map((w) => (
                      <FormSelectOption key={w} value={w} label={w} />
                    ))}
                  </FormSelect>
                </LabeledField>
                <Button
                  variant="plain"
                  icon={<TrashIcon />}
                  aria-label="Remove field"
                  onClick={() => setFields(fields.filter((_, i) => i !== idx))}
                />
              </div>
            ))}
          </>
        )}
      </Form>
    </Modal>
  );
}
