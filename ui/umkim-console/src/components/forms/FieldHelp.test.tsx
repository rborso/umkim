import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { FieldHelp } from './FieldHelp';

describe('FieldHelp', () => {
  it('renders help trigger with test id', () => {
    render(
      <FieldHelp
        fieldId="routing-cluster"
        description="Target OCM cluster for placement."
        example="virt02"
      />,
    );
    expect(screen.getByTestId('field-help-routing-cluster')).toBeInTheDocument();
    expect(screen.getByLabelText('Show field help')).toBeInTheDocument();
  });
});
