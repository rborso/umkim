import { FormGroup, FormHelperText } from '@patternfly/react-core';
import type { ReactNode } from 'react';
import { FieldHelp, type FieldHelpProps } from './FieldHelp';

type Props = {
  label: string;
  fieldId: string;
  isRequired?: boolean;
  help?: FieldHelpProps;
  helperText?: ReactNode;
  validated?: 'default' | 'error';
  errorMessage?: string;
  children: ReactNode;
};

export function LabeledField({
  label,
  fieldId,
  isRequired,
  help,
  helperText,
  validated,
  errorMessage,
  children,
}: Props) {
  return (
    <FormGroup
      label={label}
      labelIcon={help ? <FieldHelp {...help} fieldId={fieldId} /> : undefined}
      fieldId={fieldId}
      isRequired={isRequired}
    >
      {children}
      {validated === 'error' && errorMessage ? (
        <FormHelperText>
          <span style={{ color: 'var(--pf-v5-global--danger-color--100)' }}>{errorMessage}</span>
        </FormHelperText>
      ) : helperText ? (
        <FormHelperText>{helperText}</FormHelperText>
      ) : null}
    </FormGroup>
  );
}
