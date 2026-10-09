import { useState } from 'react';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  PageSection,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { useSearchParams } from 'react-router-dom';
import { redeemInvite } from '../api/client';

export function InviteRedeemPage() {
  const [params] = useSearchParams();
  const token = params.get('token') ?? '';
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setError(null);
    if (!token) {
      setError('This link is missing its invite token.');
      return;
    }
    if (password.length < 8) {
      setError('Password must be at least 8 characters.');
      return;
    }
    if (password !== confirm) {
      setError('Passwords do not match.');
      return;
    }
    setBusy(true);
    try {
      const res = await redeemInvite({
        token,
        username,
        password,
        displayName: displayName || undefined,
        email: email || undefined,
      });
      setDone(res.username);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <PageSection>
      <Title headingLevel="h1">Accept invitation</Title>
      {done ? (
        <Alert variant="success" isInline title={`Account created: ${done}`}>
          You can now sign in with your new credentials.
        </Alert>
      ) : (
        <>
          <Alert variant="info" isInline title="Create your account" style={{ marginBottom: '1rem' }}>
            Choose a username and password. Your account is added to the directory and the tenant
            groups chosen by the administrator.
          </Alert>
          {error ? (
            <Alert variant="danger" isInline title={error} style={{ marginBottom: '1rem' }} />
          ) : null}
          <Form onSubmit={(e) => { e.preventDefault(); void submit(); }}>
            <FormGroup label="Username" isRequired fieldId="invite-username">
              <TextInput
                id="invite-username"
                value={username}
                onChange={(_e, v) => setUsername(v.toLowerCase())}
                placeholder="jdoe"
              />
            </FormGroup>
            <FormGroup label="Display name" fieldId="invite-display">
              <TextInput id="invite-display" value={displayName} onChange={(_e, v) => setDisplayName(v)} />
            </FormGroup>
            <FormGroup label="Email" fieldId="invite-email">
              <TextInput id="invite-email" type="email" value={email} onChange={(_e, v) => setEmail(v)} />
            </FormGroup>
            <FormGroup label="Password" isRequired fieldId="invite-password">
              <TextInput
                id="invite-password"
                type="password"
                value={password}
                onChange={(_e, v) => setPassword(v)}
              />
            </FormGroup>
            <FormGroup label="Confirm password" isRequired fieldId="invite-confirm">
              <TextInput
                id="invite-confirm"
                type="password"
                value={confirm}
                onChange={(_e, v) => setConfirm(v)}
              />
            </FormGroup>
            <Button type="submit" variant="primary" isLoading={busy} isDisabled={busy}>
              {busy ? 'Creating account…' : 'Create account'}
            </Button>
          </Form>
        </>
      )}
    </PageSection>
  );
}
