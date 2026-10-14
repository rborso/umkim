import { Button, Checkbox, FormGroup, TextInput } from '@patternfly/react-core';
import { isPlaceholderAddress } from '../../utils/clusterHosts';
import type { FleetClusterNodeVMInput } from '../../api/types';

type Props = {
  /** Unique prefix for the input ids (control-0, worker-1, single). */
  idPrefix: string;
  /** The guest name the pipeline will use: the node hostname. */
  hostname: string;
  address: string;
  vm?: FleetClusterNodeVMInput;
  /** True when the slot is bound to an existing inventory host. */
  hostSelected: boolean;
  longhornEnabled: boolean;
  disabled?: boolean;
  /** Label for the bulk-apply button, e.g. "Copy to all control nodes". */
  copyLabel?: string;
  onToggle: (enabled: boolean) => void;
  onChange: (patch: FleetClusterNodeVMInput) => void;
  onCopyToSiblings?: () => void;
};

/**
 * Per-node "create this VM" controls for the cluster create wizard. Enabling the
 * box makes the bring-up pipeline declare the YAIDAP guest (named after the node)
 * and then create it, which removes the manual "add host" step.
 */
export function NodeVMEditor({
  idPrefix,
  hostname,
  address,
  vm,
  hostSelected,
  longhornEnabled,
  disabled,
  copyLabel,
  onToggle,
  onChange,
  onCopyToSiblings,
}: Props) {
  const enabled = !!vm;
  const num = (key: keyof FleetClusterNodeVMInput) => {
    const v = vm?.[key];
    return v === undefined || v === null ? '' : String(v);
  };
  const setNum = (key: keyof FleetClusterNodeVMInput, raw: string) => {
    const n = parseInt(raw, 10);
    onChange({ [key]: Number.isFinite(n) ? n : 0 });
  };
  const placeholderAddress = isPlaceholderAddress(address);

  return (
    <div data-testid={`${idPrefix}-vm`}>
      <Checkbox
        id={`${idPrefix}-vm-create`}
        label="Create this VM"
        isChecked={enabled}
        isDisabled={disabled || hostSelected}
        onChange={(_e, checked) => onToggle(checked)}
        data-testid={`${idPrefix}-vm-create`}
      />
      <p
        style={{
          fontSize: '0.8rem',
          marginTop: '0.25rem',
          color: 'var(--pf-v5-global--Color--200)',
        }}
      >
        {hostSelected
          ? 'An inventory host is selected for this slot, so nothing is created.'
          : enabled
            ? `The pipeline will declare and create the guest ${hostname || '(node name)'}.`
            : 'Declare and create a VM guest for this node during bring-up.'}
      </p>
      {enabled && placeholderAddress && (
        <p style={{ fontSize: '0.8rem', color: 'var(--pf-v5-global--danger-color--100)' }}>
          Set the real site address first: {address || '(empty)'} is a placeholder.
        </p>
      )}
      {enabled && (
        <>
          <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', marginTop: '0.5rem' }}>
            <TextInput
              id={`${idPrefix}-vm-cores`}
              type="number"
              aria-label="VM vCPU cores"
              placeholder="2"
              value={num('cores')}
              onChange={(_e, v) => setNum('cores', v)}
            />
            <TextInput
              id={`${idPrefix}-vm-memory`}
              type="number"
              aria-label="VM memory MiB"
              placeholder="4096"
              value={num('memory')}
              onChange={(_e, v) => setNum('memory', v)}
            />
            <TextInput
              id={`${idPrefix}-vm-disk`}
              type="number"
              aria-label="VM boot disk GiB"
              placeholder="40"
              value={num('diskGi')}
              onChange={(_e, v) => setNum('diskGi', v)}
            />
            {longhornEnabled && (
              <TextInput
                id={`${idPrefix}-vm-datadisk`}
                type="number"
                aria-label="VM data disk GiB"
                placeholder="10"
                value={num('dataDiskGi')}
                onChange={(_e, v) => setNum('dataDiskGi', v)}
              />
            )}
            <TextInput
              id={`${idPrefix}-vm-hypervisor`}
              aria-label="VM hypervisor"
              placeholder="least-utilized"
              value={vm?.hypervisor || ''}
              onChange={(_e, v) => onChange({ hypervisor: v })}
            />
          </div>
          <p style={{ fontSize: '0.75rem', marginTop: '0.25rem', color: 'var(--pf-v5-global--Color--200)' }}>
            vCPU · RAM (MiB) · boot disk (GiB)
            {longhornEnabled ? ' · data disk (GiB)' : ''} · hypervisor
          </p>
          {onCopyToSiblings && copyLabel && (
            <FormGroup fieldId={`${idPrefix}-vm-copy`}>
              <Button variant="link" isInline onClick={onCopyToSiblings}>
                {copyLabel}
              </Button>
            </FormGroup>
          )}
        </>
      )}
    </div>
  );
}
