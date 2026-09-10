import { FormSelect, FormSelectOption, NumberInput } from '@patternfly/react-core';
import { useMemo } from 'react';
import {
  formatCpuQuantityValue,
  parseCpuQuantityParts,
  type CpuQuantityUnit,
} from '../../utils/k8sQuantity';

type Props = {
  id: string;
  value: string;
  onChange: (value: string) => void;
  ariaLabel?: string;
};

export function CpuQuantityInput({ id, value, onChange, ariaLabel }: Props) {
  const parts = useMemo(() => parseCpuQuantityParts(value), [value]);

  const emit = (amount: number, unit: CpuQuantityUnit) => {
    onChange(formatCpuQuantityValue({ amount, unit }));
  };

  return (
    <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', flexWrap: 'wrap' }}>
      <NumberInput
        inputName={id}
        inputAriaLabel={ariaLabel || 'CPU amount'}
        value={parts.amount}
        min={parts.unit === 'millicores' ? 1 : 0.001}
        step={parts.unit === 'millicores' ? 1 : 0.1}
        onChange={(event) => {
          const raw = (event.target as HTMLInputElement).value;
          const n = Number(raw);
          if (Number.isNaN(n)) return;
          emit(n, parts.unit);
        }}
        onMinus={() => {
          const step = parts.unit === 'millicores' ? 1 : 0.1;
          const min = parts.unit === 'millicores' ? 1 : 0.001;
          emit(Math.max(min, parts.amount - step), parts.unit);
        }}
        onPlus={() => {
          const step = parts.unit === 'millicores' ? 1 : 0.1;
          emit(parts.amount + step, parts.unit);
        }}
        style={{ maxWidth: '8rem' }}
      />
      <FormSelect
        id={`${id}-unit`}
        aria-label="CPU unit"
        value={parts.unit}
        onChange={(_e, v) => emit(parts.amount, v as CpuQuantityUnit)}
        style={{ maxWidth: '9rem' }}
      >
        <FormSelectOption value="millicores" label="millicores (m)" />
        <FormSelectOption value="cores" label="cores" />
      </FormSelect>
    </div>
  );
}
