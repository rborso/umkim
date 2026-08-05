import {
  Checkbox,
  FormSelect,
  FormSelectOption,
  NumberInput,
  TextInput,
  TextArea,
} from '@patternfly/react-core';
import type { CatalogFieldDef } from '../../api/types';
import { isCatalogFieldEditable, isFieldRequired, isYamlLikeField, resolveFieldWidget } from '../../catalog/fieldUtils';
import { filteredEnumOptions } from '../../catalog/dependsOnOptions';
import { previewIngressFqdn, type SiteDomainFieldContext } from '../../catalog/siteDomains';
import { valuesToDisplayString } from '../../catalog/valuesMerge';
import { resolveCatalogFieldHelp } from '../../help/fieldHelp';
import { LabeledField } from '../forms/LabeledField';
import { SiteDomainField } from '../forms/SiteDomainField';
import { CpuQuantityInput } from '../forms/CpuQuantityInput';

type Props = {
  fields: CatalogFieldDef[];
  values: Record<string, string>;
  onChange: (path: string, value: string) => void;
  idPrefix?: string;
  readOnly?: boolean;
  errors?: Record<string, string>;
  siteDomainContext?: SiteDomainFieldContext;
};

function boolFromString(raw: string): boolean {
  const lower = raw.toLowerCase();
  return lower === 'true' || lower === '1';
}

function enumOptionLabel(opt: unknown): string {
  const s = String(opt);
  if (s === '') return '(inherit / default)';
  return s;
}

function formatFieldDefault(field: CatalogFieldDef): string | undefined {
  const raw = field.default;
  if (raw === undefined || raw === null || raw === '') return undefined;
  if (typeof raw === 'object') {
    return valuesToDisplayString(raw as Record<string, unknown>);
  }
  return String(raw);
}

export function DynamicFieldForm({
  fields,
  values,
  onChange,
  idPrefix = 'df',
  readOnly = false,
  errors = {},
  siteDomainContext,
}: Props) {
  if (fields.length === 0) {
    return null;
  }

  return (
    <>
      {fields.map((field) => {
        const id = `${idPrefix}-${field.path.replace(/\./g, '-')}`;
        const label = field.display_name || field.path;
        const value = values[field.path] ?? '';
        const enumSource = filteredEnumOptions(field, values) ?? field.validation_schema?.enum;
        const enums = enumSource;
        const isRequired = isFieldRequired(field);
        const fieldReadOnly = readOnly || !isCatalogFieldEditable(field);
        const widget = resolveFieldWidget(field);
        const yamlLike = isYamlLikeField(field);
        const help = resolveCatalogFieldHelp(field);
        const fieldId = field.path.replace(/\./g, '-');
        const fieldError = errors[field.path];
        const validated = fieldError ? 'error' : 'default';
        const showSelect =
          !fieldReadOnly &&
          (widget === 'select' || (enums && enums.length > 0));

        if (showSelect && enums && enums.length > 0) {
          const selected = value !== '' ? value : String(field.default ?? '');
          return (
            <LabeledField
              key={field.path}
              label={label}
              fieldId={id}
              isRequired={isRequired}
              help={help}
              validated={validated}
              errorMessage={fieldError}
            >
              <FormSelect
                id={id}
                value={selected}
                onChange={(_e, v) => onChange(field.path, v)}
                data-testid={`${idPrefix}-enum-${fieldId}`}
              >
                {enums.map((opt) => (
                  <FormSelectOption
                    key={String(opt)}
                    value={String(opt)}
                    label={enumOptionLabel(opt)}
                  />
                ))}
              </FormSelect>
            </LabeledField>
          );
        }

        if (widget === 'boolean' && !fieldReadOnly) {
          return (
            <LabeledField
              key={field.path}
              label={label}
              fieldId={id}
              isRequired={isRequired}
              help={help}
              validated={validated}
              errorMessage={fieldError}
            >
              <Checkbox
                id={id}
                isChecked={boolFromString(value || String(field.default ?? 'false'))}
                onChange={(_e, checked) => onChange(field.path, checked ? 'true' : 'false')}
                label={label}
              />
            </LabeledField>
          );
        }

        if (widget === 'cpu-quantity' && !fieldReadOnly) {
          return (
            <LabeledField
              key={field.path}
              label={label}
              fieldId={id}
              isRequired={isRequired}
              help={help}
              validated={validated}
              errorMessage={fieldError}
            >
              <CpuQuantityInput
                id={id}
                value={value || String(field.default ?? '500m')}
                onChange={(v) => onChange(field.path, v)}
                ariaLabel={label}
              />
            </LabeledField>
          );
        }

        if (widget === 'number' && !fieldReadOnly) {
          const num = value === '' ? NaN : Number(value);
          return (
            <LabeledField
              key={field.path}
              label={label}
              fieldId={id}
              isRequired={isRequired}
              help={help}
              validated={validated}
              errorMessage={fieldError}
            >
              <NumberInput
                inputName={id}
                inputAriaLabel={label}
                value={Number.isNaN(num) ? 0 : num}
                onChange={(event) => {
                  const v = (event.target as HTMLInputElement).value;
                  onChange(field.path, v);
                }}
                onMinus={() => {
                  const n = Number.isNaN(num) ? 0 : num;
                  onChange(field.path, String(Math.max(0, n - 1)));
                }}
                onPlus={() => {
                  const n = Number.isNaN(num) ? 0 : num;
                  onChange(field.path, String(n + 1));
                }}
              />
            </LabeledField>
          );
        }

        if ((yamlLike || widget === 'yaml') && !fieldReadOnly) {
          return (
            <LabeledField
              key={field.path}
              label={label}
              fieldId={id}
              isRequired={isRequired}
              help={help}
              validated={validated}
              errorMessage={fieldError}
            >
              <TextArea
                id={id}
                value={value}
                onChange={(_e, v) => onChange(field.path, v)}
                rows={field.path === 'values' ? 10 : 6}
                resizeOrientation="vertical"
                style={field.path === 'values' ? { fontFamily: 'monospace' } : undefined}
                placeholder={formatFieldDefault(field)}
              />
            </LabeledField>
          );
        }

        if (widget === 'site-domain' && !fieldReadOnly && siteDomainContext) {
          const optionId =
            siteDomainContext.selectedOptionByField[field.path] ??
            siteDomainContext.options[0]?.id ??
            '';
          const zoneValue = values[field.path] ?? '';
          const preview = previewIngressFqdn(
            values,
            siteDomainContext.hostPrefixPath,
            field.path,
          );
          return (
            <SiteDomainField
              key={field.path}
              fieldId={id}
              label={label}
              isRequired={isRequired}
              help={help}
              validated={validated}
              errorMessage={fieldError}
              options={siteDomainContext.options}
              selectedOptionId={optionId}
              zoneValue={zoneValue}
              onOptionChange={(optId, zone) =>
                siteDomainContext.onDomainChange(field.path, optId, zone)
              }
              onCustomZoneChange={(zone) => onChange(field.path, zone)}
              previewFqdn={preview || undefined}
            />
          );
        }

        const inputType = widget === 'password' ? 'password' : 'text';

        return (
          <LabeledField
            key={field.path}
            label={label}
            fieldId={fieldId}
            isRequired={isRequired}
            help={help}
            validated={validated}
            errorMessage={fieldError}
          >
            <TextInput
              id={id}
              type={inputType}
              value={value}
              onChange={(_e, v) => onChange(field.path, v)}
              isDisabled={fieldReadOnly}
              readOnlyVariant={fieldReadOnly ? 'default' : undefined}
              placeholder={!fieldReadOnly ? formatFieldDefault(field) : undefined}
            />
          </LabeledField>
        );
      })}
    </>
  );
}
