import {
  FormSelect,
  FormSelectOption,
  TextInput,
} from '@patternfly/react-core';
import type { DomainZoneOption } from '../../catalog/siteDomains';
import { isCustomDomainOption } from '../../catalog/siteDomains';
import { LabeledField } from './LabeledField';
import type { FieldHelpContent } from '../../help/fieldHelp';

type Props = {
  fieldId: string;
  label: string;
  isRequired?: boolean;
  help?: FieldHelpContent;
  validated?: 'error' | 'default';
  errorMessage?: string;
  options: DomainZoneOption[];
  selectedOptionId: string;
  zoneValue: string;
  onOptionChange: (optionId: string, zone: string) => void;
  onCustomZoneChange: (zone: string) => void;
  previewFqdn?: string;
};

export function SiteDomainField({
  fieldId,
  label,
  isRequired,
  help,
  validated,
  errorMessage,
  options,
  selectedOptionId,
  zoneValue,
  onOptionChange,
  onCustomZoneChange,
  previewFqdn,
}: Props) {
  const custom = isCustomDomainOption(selectedOptionId);

  return (
    <>
      <LabeledField
        label={label}
        fieldId={`${fieldId}-zone-select`}
        isRequired={isRequired}
        help={help}
        validated={validated}
        errorMessage={errorMessage}
      >
        <FormSelect
          id={`${fieldId}-zone-select`}
          value={selectedOptionId}
          onChange={(_e, v) => {
            const opt = options.find((o) => o.id === v);
            onOptionChange(v, opt?.zone ?? '');
          }}
          data-testid="deploy-domain-zone-select"
        >
          {options.map((opt) => (
            <FormSelectOption key={opt.id} value={opt.id} label={opt.label} />
          ))}
        </FormSelect>
      </LabeledField>
      {custom ? (
        <LabeledField
          label="Custom base domain"
          fieldId={`${fieldId}-custom`}
          isRequired={isRequired}
          validated={validated}
          errorMessage={errorMessage}
        >
          <TextInput
            id={`${fieldId}-custom`}
            value={zoneValue}
            onChange={(_e, v) => onCustomZoneChange(v)}
            placeholder="apps.example.com"
            data-testid="deploy-domain-custom"
          />
        </LabeledField>
      ) : null}
      {previewFqdn ? (
        <p style={{ fontSize: '0.875rem', color: 'var(--pf-v5-global--Color--200)', marginTop: '0.25rem' }}>
          URL preview: <code>{previewFqdn}</code>
        </p>
      ) : null}
    </>
  );
}
