import { useState, type Ref } from 'react';
import {
  MenuToggle,
  type MenuToggleElement,
  Select,
  SelectList,
  SelectOption,
} from '@patternfly/react-core';

export type FleetModalSelectOption = {
  value: string;
  label: string;
  isDisabled?: boolean;
};

type Props = {
  id: string;
  value: string;
  options: FleetModalSelectOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  isDisabled?: boolean;
};

/** Select menu portaled to document.body so options are not clipped by modal overflow. */
export function FleetModalSelect({
  id,
  value,
  options,
  onChange,
  placeholder = 'Select…',
  isDisabled,
}: Props) {
  const [open, setOpen] = useState(false);
  const selectedLabel = options.find((o) => o.value === value)?.label ?? placeholder;

  return (
    <Select
      id={id}
      isOpen={open}
      selected={value}
      onSelect={(_event, v) => {
        onChange(v === undefined ? '' : String(v));
        setOpen(false);
      }}
      onOpenChange={setOpen}
      popperProps={{ appendTo: () => document.body, width: 'trigger' }}
      toggle={(toggleRef: Ref<MenuToggleElement>) => (
        <MenuToggle
          ref={toggleRef}
          id={id}
          onClick={() => setOpen((was) => !was)}
          isExpanded={open}
          isDisabled={isDisabled}
          style={{ width: '100%' }}
        >
          {selectedLabel}
        </MenuToggle>
      )}
    >
      <SelectList>
        {options.map((o) => (
          <SelectOption key={`${o.value}\0${o.label}`} value={o.value} isDisabled={o.isDisabled}>
            {o.label}
          </SelectOption>
        ))}
      </SelectList>
    </Select>
  );
}
