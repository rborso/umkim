import { useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import {
  Button,
  Form,
  LoginPage as PFLoginPage,
  TextInput,
  Alert,
} from '@patternfly/react-core';
import { useAuth } from '../auth/AuthContext';
import { getOIDCConfig } from '../api/authClient';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';
import { useEffect } from 'react';
import type { OIDCConfig } from '../auth/types';

export function LoginPage() {
  const { login, user } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [oidc, setOidc] = useState<OIDCConfig | null>(null);

  useEffect(() => {
    if (user) {
      const from = (location.state as { from?: string })?.from ?? '/';
      navigate(from, { replace: true });
    }
  }, [user, navigate, location.state]);

  useEffect(() => {
    getOIDCConfig().then(setOidc).catch(() => setOidc({ enabled: false }));
  }, []);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setBusy(true);
    try {
      await login(username, password);
      const from = (location.state as { from?: string })?.from ?? '/';
      navigate(from, { replace: true });
    } catch {
      setError('Invalid username or password');
    } finally {
      setBusy(false);
    }
  }

  const loginForm = (
    <Form onSubmit={onSubmit}>
      {error && (
        <Alert variant="danger" title={error} isInline style={{ marginBottom: '1rem' }} />
      )}
      <LabeledField label="Username" fieldId="username" isRequired help={getFieldHelp('login.username')}>
        <TextInput
          id="username"
          data-testid="login-username"
          value={username}
          onChange={(_e, v) => setUsername(v)}
          autoComplete="username"
        />
      </LabeledField>
      <LabeledField label="Password" fieldId="password" isRequired help={getFieldHelp('login.password')}>
        <TextInput
          id="password"
          data-testid="login-password"
          type="password"
          value={password}
          onChange={(_e, v) => setPassword(v)}
          autoComplete="current-password"
        />
      </LabeledField>
      <Button type="submit" variant="primary" isBlock isLoading={busy} data-testid="login-submit">
        Log in
      </Button>
      {oidc?.enabled && oidc.authorizeUrl && (
        <Button
          variant="secondary"
          isBlock
          style={{ marginTop: '0.75rem' }}
          isDisabled
          title="OIDC SSO (Keycloak) — configure FLEET_AUTH_OIDC_* on hub"
        >
          Sign in with SSO
        </Button>
      )}
    </Form>
  );

  return (
    <PFLoginPage
      brandImgSrc=""
      brandImgAlt="DCM Fleet"
      textContent="DCM Fleet Console"
      loginTitle="Log in to your account"
    >
      {loginForm}
    </PFLoginPage>
  );
}
