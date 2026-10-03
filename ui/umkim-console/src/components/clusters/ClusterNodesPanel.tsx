import { useState } from 'react';
import { Button, Label, LabelGroup, TextInput } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import type { FleetNode } from '../../api/types';
import { formatResourceUsagePair } from '../../utils/k8sQuantity';
import { FieldHelp } from '../forms/FieldHelp';
import { getFieldHelp } from '../../help/fieldHelp';

function nodeInternalIp(n: FleetNode): string {
  const internal = n.addresses?.find((a) => a.type === 'InternalIP');
  if (internal) return internal.address;
  const hostname = n.addresses?.find((a) => a.type === 'Hostname');
  return hostname?.address || '—';
}

function conditionsSummary(n: FleetNode): string {
  const bad = (n.conditions || []).filter(
    (c) => c.status !== 'False' && c.type !== 'Ready' && c.type !== 'NodeReady',
  );
  if (bad.length === 0) return 'OK';
  return bad.map((c) => c.type).join(', ');
}

function NodeLabelsCell({ labels }: { labels: Record<string, string> }) {
  const entries = Object.entries(labels || {});
  if (entries.length === 0) {
    return <span style={{ color: '#6a6e73' }}>—</span>;
  }
  return (
    <LabelGroup numLabels={5} isEditable={false}>
      {entries.map(([k, v]) => (
        <Label key={k} color="blue" isCompact>
          <span style={{ wordBreak: 'break-word' }}>
            {k}={v}
          </span>
        </Label>
      ))}
    </LabelGroup>
  );
}

export function ClusterNodesPanel({
  nodes,
  clusterPhase,
  clusterAvailable,
  nodesFetchFailed,
  showLabelEditor,
  onAddLabel,
}: {
  nodes: FleetNode[];
  clusterPhase?: string;
  clusterAvailable?: boolean;
  nodesFetchFailed?: boolean;
  showLabelEditor?: boolean;
  onAddLabel?: (nodeName: string, key: string, value: string) => void;
}) {
  if (nodes.length === 0) {
    if (clusterPhase === 'Ready' && !clusterAvailable) {
      return (
        <p>
          Waiting for OCM klusterlet to connect. Hub provision is complete; node list appears once
          the cluster is available.
        </p>
      );
    }
    if (nodesFetchFailed) {
      return <p>No nodes (cluster proxy failed; klusterlet may still be starting).</p>;
    }
    return <p>No nodes (cluster proxy unavailable or klusterlet not ready).</p>;
  }

  return (
    <>
      {!clusterAvailable && nodes.length > 0 && (
        <p style={{ marginBottom: '0.75rem' }}>
          OCM klusterlet is not available; nodes are shown via direct cluster API access.
        </p>
      )}
      <div style={{ overflowX: 'auto' }}>
        <Table aria-label="Cluster nodes">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Internal IP</Th>
              <Th>Roles</Th>
              <Th>Ready</Th>
              <Th>CPU</Th>
              <Th>Memory</Th>
              <Th>Disk</Th>
              <Th>Conditions</Th>
              {showLabelEditor && <Th>Labels</Th>}
            </Tr>
          </Thead>
          <Tbody>
            {nodes.map((n) => (
              <Tr key={n.name}>
                <Td>{n.name}</Td>
                <Td>
                  <code>{nodeInternalIp(n)}</code>
                </Td>
                <Td>{(n.roles || []).join(', ') || '—'}</Td>
                <Td>
                  <Label color={n.ready ? 'green' : 'red'}>{n.ready ? 'Yes' : 'No'}</Label>
                </Td>
                <Td>{formatResourceUsagePair(n.usage?.cpu, n.capacity?.cpu, 'cpu')}</Td>
                <Td>{formatResourceUsagePair(n.usage?.memory, n.capacity?.memory, 'memory')}</Td>
                <Td>{n.capacity?.ephemeral_storage || '—'}</Td>
                <Td>{conditionsSummary(n)}</Td>
                {showLabelEditor && onAddLabel && (
                  <Td style={{ maxWidth: '20rem', verticalAlign: 'top' }}>
                    <NodeLabelsCell labels={n.labels || {}} />
                    <NodeLabelAdd onAdd={(key, value) => onAddLabel(n.name, key, value)} />
                  </Td>
                )}
              </Tr>
            ))}
          </Tbody>
        </Table>
      </div>
    </>
  );
}

function NodeLabelAdd({ onAdd }: { onAdd: (key: string, value: string) => void }) {
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  return (
    <div style={{ marginTop: '0.5rem', display: 'flex', gap: '0.25rem', flexWrap: 'wrap', alignItems: 'center' }}>
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '0.15rem' }}>
        Label key
        {getFieldHelp('clusterNodes.labelKey') ? (
          <FieldHelp {...getFieldHelp('clusterNodes.labelKey')!} fieldId="clusterNodes-labelKey" />
        ) : null}
      </span>
      <TextInput
        placeholder="fleet.label/key"
        value={key}
        onChange={(_e, v) => setKey(v)}
        style={{ maxWidth: '12rem' }}
      />
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '0.15rem' }}>
        Value
        {getFieldHelp('clusterNodes.labelValue') ? (
          <FieldHelp {...getFieldHelp('clusterNodes.labelValue')!} fieldId="clusterNodes-labelValue" />
        ) : null}
      </span>
      <TextInput
        placeholder="value"
        value={value}
        onChange={(_e, v) => setValue(v)}
        style={{ maxWidth: '8rem' }}
      />
      <Button variant="secondary" onClick={() => onAdd(key, value)}>
        Add label
      </Button>
    </div>
  );
}
