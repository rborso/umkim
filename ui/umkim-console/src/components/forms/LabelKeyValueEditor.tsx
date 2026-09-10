import { Button, TextInput } from '@patternfly/react-core';
import { PlusIcon, TrashIcon } from '@patternfly/react-icons';
import { useState } from 'react';

type Props = {
  labels: Record<string, string>;
  onChange: (labels: Record<string, string>) => void;
  keyPlaceholder?: string;
  valuePlaceholder?: string;
  idPrefix?: string;
};

export function LabelKeyValueEditor({
  labels,
  onChange,
  keyPlaceholder = 'fleet.label/zone',
  valuePlaceholder = 'homelab',
  idPrefix = 'label',
}: Props) {
  const entries = Object.entries(labels);
  const [draftKey, setDraftKey] = useState('');
  const [draftValue, setDraftValue] = useState('');

  const add = () => {
    const k = draftKey.trim();
    if (!k) return;
    onChange({ ...labels, [k]: draftValue.trim() });
    setDraftKey('');
    setDraftValue('');
  };

  const remove = (key: string) => {
    const next = { ...labels };
    delete next[key];
    onChange(next);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '0.35rem' }}>
      {entries.map(([k, v]) => (
        <div
          key={k}
          style={{
            display: 'flex',
            gap: '0.35rem',
            alignItems: 'center',
            flexWrap: 'wrap',
            wordBreak: 'break-word',
          }}
        >
          <code style={{ fontSize: '0.85rem', minWidth: '8rem' }}>{k}</code>
          <span>=</span>
          <code style={{ fontSize: '0.85rem', wordBreak: 'break-word' }}>{v}</code>
          <Button variant="plain" icon={<TrashIcon />} aria-label={`Remove ${k}`} onClick={() => remove(k)} />
        </div>
      ))}
      <div style={{ display: 'flex', gap: '0.35rem', flexWrap: 'wrap', alignItems: 'center' }}>
        <TextInput
          id={`${idPrefix}-key`}
          placeholder={keyPlaceholder}
          value={draftKey}
          onChange={(_e, val) => setDraftKey(val)}
          style={{ maxWidth: '14rem' }}
        />
        <TextInput
          id={`${idPrefix}-value`}
          placeholder={valuePlaceholder}
          value={draftValue}
          onChange={(_e, val) => setDraftValue(val)}
          style={{ maxWidth: '10rem' }}
        />
        <Button variant="secondary" icon={<PlusIcon />} onClick={add}>
          Add label
        </Button>
      </div>
    </div>
  );
}
