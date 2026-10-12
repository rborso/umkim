import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Form,
  Modal,
  ModalVariant,
  PageSection,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Navigate } from 'react-router-dom';
import {
  activateLLMApiKey,
  createLLMApiKey,
  deleteLLMApiKey,
  listLLMApiKeys,
  revokeLLMApiKey,
  rotateLLMApiKey,
  type FleetLLMApiKey,
} from '../api/authClient';
import { useAuth } from '../auth/AuthContext';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';

export function LLMApiKeysPage() {
  const { user } = useAuth();
  const allowed = !!user?.dcAdmin;

  const [apiEndpoint, setApiEndpoint] = useState('');
  const [keys, setKeys] = useState<FleetLLMApiKey[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [tokenOpen, setTokenOpen] = useState(false);
  const [issuedToken, setIssuedToken] = useState('');
  const [keyName, setKeyName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [models, setModels] = useState('');
  const [saving, setSaving] = useState(false);

  const load = async () => {
    setError(null);
    try {
      const data = await listLLMApiKeys();
      setKeys(data.keys);
      setApiEndpoint(data.apiEndpoint ?? '');
    } catch (e) {
      setError(e);
    }
  };

  useEffect(() => {
    if (allowed) void load();
  }, [allowed]);

  if (!allowed) return <Navigate to="/" replace />;

  const handleCreate = async () => {
    if (!keyName.trim()) return;
    setSaving(true);
    setError(null);
    const modelList = models
      .split(',')
      .map((m) => m.trim())
      .filter(Boolean);
    try {
      const res = await createLLMApiKey({
        name: keyName.trim(),
        displayName: displayName.trim() || keyName.trim(),
        models: modelList.length ? modelList : undefined,
      });
      setCreateOpen(false);
      setKeyName('');
      setDisplayName('');
      setModels('');
      setIssuedToken(res.token);
      setTokenOpen(true);
      await load();
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageSection>
      <Title headingLevel="h1">LLM API keys</Title>
      <p style={{ marginTop: '0.5rem', maxWidth: '52rem' }}>
        Create Bearer tokens for the public OpenAI-compatible API.
        {apiEndpoint ? (
          <>
            {' '}
            Endpoint: <code>{apiEndpoint}</code>
          </>
        ) : null}
      </p>
      <ErrorAlert error={error} />
      <Button variant="primary" onClick={() => setCreateOpen(true)} style={{ marginBottom: '1rem' }}>
        Create API key
      </Button>
      <Table variant="compact">
        <Thead>
          <Tr>
            <Th>Name</Th>
            <Th>Display</Th>
            <Th>Prefix</Th>
            <Th>Active</Th>
            <Th>Models</Th>
            <Th>Actions</Th>
          </Tr>
        </Thead>
        <Tbody>
          {keys.map((k) => {
            const meta = k.metadata ?? {};
            const spec = k.spec ?? {};
            const status = k.status ?? {};
            return (
              <Tr key={meta.name}>
                <Td dataLabel="Name">{meta.name}</Td>
                <Td dataLabel="Display">{spec.displayName ?? ''}</Td>
                <Td dataLabel="Prefix">
                  <code>{status.keyPrefix ?? ''}</code>
                </Td>
                <Td dataLabel="Active">{status.active ? 'yes' : 'no'}</Td>
                <Td dataLabel="Models">{(spec.models ?? []).join(', ') || 'all'}</Td>
                <Td dataLabel="Actions">
                  <Button
                    variant="link"
                    onClick={() =>
                      void rotateLLMApiKey(meta.name!).then((res) => {
                        setIssuedToken(res.token);
                        setTokenOpen(true);
                        void load();
                      })
                    }
                  >
                    Rotate
                  </Button>
                  {status.active ? (
                    <Button variant="link" onClick={() => void revokeLLMApiKey(meta.name!).then(() => load())}>
                      Revoke
                    </Button>
                  ) : (
                    <Button variant="link" onClick={() => void activateLLMApiKey(meta.name!).then(() => load())}>
                      Activate
                    </Button>
                  )}
                  <Button
                    variant="link"
                    isDanger
                    onClick={() => void deleteLLMApiKey(meta.name!).then(() => load())}
                  >
                    Delete
                  </Button>
                </Td>
              </Tr>
            );
          })}
        </Tbody>
      </Table>

      <Modal variant={ModalVariant.medium} title="Create LLM API key" isOpen={createOpen} onClose={() => setCreateOpen(false)}>
        <Form>
          <LabeledField label="Key name" fieldId="llmKeyName" help={getFieldHelp('llmApiKey.name')}>
            <TextInput id="llmKeyName" value={keyName} onChange={(_e, v) => setKeyName(v)} />
          </LabeledField>
          <LabeledField label="Display name" fieldId="llmKeyDisplay" help={getFieldHelp('llmApiKey.displayName')}>
            <TextInput id="llmKeyDisplay" value={displayName} onChange={(_e, v) => setDisplayName(v)} />
          </LabeledField>
          <LabeledField
            label="Allowed models (optional)"
            fieldId="llmKeyModels"
            help={getFieldHelp('llmApiKey.models')}
            helperText="Comma-separated; leave empty for all models"
          >
            <TextInput
              id="llmKeyModels"
              value={models}
              onChange={(_e, v) => setModels(v)}
              placeholder="Qwen2.5-0.5B-Instruct"
            />
          </LabeledField>
        </Form>
        <Button variant="primary" isLoading={saving} onClick={() => void handleCreate()} style={{ marginTop: '1rem' }}>
          Create
        </Button>
      </Modal>

      <Modal variant={ModalVariant.medium} title="API key token" isOpen={tokenOpen} onClose={() => setTokenOpen(false)}>
        <Alert variant="warning" isInline title="Copy this token now" style={{ marginBottom: '1rem' }}>
          It is shown only once. Use <code>Authorization: Bearer &lt;token&gt;</code> against the public endpoint.
        </Alert>
        <TextInput readOnly value={issuedToken} />
      </Modal>
    </PageSection>
  );
}
