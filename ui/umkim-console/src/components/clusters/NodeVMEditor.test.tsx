import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { NodeVMEditor } from './NodeVMEditor';
import { isPlaceholderAddress } from '../../utils/clusterHosts';

// The shared setup file does not register RTL auto-cleanup, so unmount between
// cases here (otherwise the queries see the previous render too).
afterEach(cleanup);

describe('isPlaceholderAddress', () => {
  it('flags the wizard placeholders and empties', () => {
    expect(isPlaceholderAddress('198.51.100.21')).toBe(true);
    expect(isPlaceholderAddress('203.0.113.9')).toBe(true);
    expect(isPlaceholderAddress('192.0.2.1')).toBe(true);
    expect(isPlaceholderAddress('')).toBe(true);
  });

  it('accepts a real site address', () => {
    expect(isPlaceholderAddress('172.16.10.101')).toBe(false);
  });
});

describe('NodeVMEditor', () => {
  const baseProps = {
    idPrefix: 'control-0',
    hostname: 'lab-1',
    address: '172.16.10.101',
    hostSelected: false,
    longhornEnabled: false,
    onToggle: vi.fn(),
    onChange: vi.fn(),
  };

  it('is off by default and explains what enabling does', () => {
    render(<NodeVMEditor {...baseProps} />);
    const box = screen.getByTestId('control-0-vm-create') as HTMLInputElement;
    expect(box.checked).toBe(false);
    expect(screen.queryByLabelText('VM vCPU cores')).toBeNull();
    expect(screen.getByText(/Declare and create a VM guest/)).toBeTruthy();
  });

  it('reveals the resource fields when enabled and names the guest', () => {
    render(<NodeVMEditor {...baseProps} vm={{}} />);
    expect(screen.getByText(/will declare and create the guest lab-1/)).toBeTruthy();
    expect(screen.getByLabelText('VM vCPU cores')).toBeTruthy();
    expect(screen.getByLabelText('VM memory MiB')).toBeTruthy();
    expect(screen.getByLabelText('VM boot disk GiB')).toBeTruthy();
    // Longhorn is off, so no data disk field.
    expect(screen.queryByLabelText('VM data disk GiB')).toBeNull();
  });

  it('shows the data disk field when Longhorn is enabled', () => {
    render(<NodeVMEditor {...baseProps} vm={{}} longhornEnabled />);
    expect(screen.getByLabelText('VM data disk GiB')).toBeTruthy();
  });

  it('disables the box when an existing inventory host fills the slot', () => {
    render(<NodeVMEditor {...baseProps} hostSelected />);
    const box = screen.getByTestId('control-0-vm-create') as HTMLInputElement;
    expect(box.disabled).toBe(true);
    expect(screen.getByText(/An inventory host is selected/)).toBeTruthy();
  });

  it('warns when the address is still a placeholder', () => {
    render(<NodeVMEditor {...baseProps} address="198.51.100.21" vm={{}} />);
    expect(screen.getByText(/is a placeholder/)).toBeTruthy();
  });

  it('reports toggles and field edits', () => {
    const onToggle = vi.fn();
    const onChange = vi.fn();
    render(<NodeVMEditor {...baseProps} vm={{}} onToggle={onToggle} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText('VM vCPU cores'), { target: { value: '6' } });
    expect(onChange).toHaveBeenCalledWith({ cores: 6 });
    fireEvent.click(screen.getByTestId('control-0-vm-create'));
    expect(onToggle).toHaveBeenCalledWith(false);
  });
});
