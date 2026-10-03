import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Button,
  FormSelect,
  FormSelectOption,
  Modal,
  ModalVariant,
  PageSection,
  PageSectionVariants,
  Radio,
  Spinner,
  TextArea,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { PlusIcon, SyncIcon, TrashIcon } from '@patternfly/react-icons';
import {
  chatFleetLlamaProfileTest,
  createFleetLlamaProfile,
  deleteFleetLlamaProfile,
  endFleetLlamaProfileTest,
  getFleetLlamaProfileTest,
  listFleetLlamaProfiles,
  patchFleetLlamaProfile,
  previewFleetLlamaProfileValues,
  verifyFleetLlamaProfile,
} from '../api/client';
import type { FleetLlamaProfile, FleetLlamaProfileTest } from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';
import { useFleetClustersQuery } from '../hooks/useFleetQueries';

export function LlamaProfilesPage() {
  const [profiles, setProfiles] = useState<FleetLlamaProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [editProfile, setEditProfile] = useState<FleetLlamaProfile | null>(null);
  const [verifyProfile, setVerifyProfile] = useState<FleetLlamaProfile | null>(null);
  const [valuesPreview, setValuesPreview] = useState('');
  const [valuesOpen, setValuesOpen] = useState(false);
  const [activeTest, setActiveTest] = useState<FleetLlamaProfileTest | null>(null);
  const [chatPrompt, setChatPrompt] = useState('');
  const [chatReply, setChatReply] = useState('');
  const [chatBusy, setChatBusy] = useState(false);

  const clustersQ = useFleetClustersQuery();
  const gpuClusters = useMemo(() => {
    const items = clustersQ.data ?? [];
    return items.filter(
      (c) =>
        c.provisioning_phase === 'Ready' &&
        !c.management &&
        (c.capabilities?.gpu ||
          c.capability_discovery?.cluster_capabilities?.gpu ||
          c.capability_discovery?.cluster_capabilities?.['fleet.capability/gpu']),
    );
  }, [clustersQ.data]);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setProfiles(await listFleetLlamaProfiles());
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const pollTest = useCallback(async (id: string) => {
    const t = await getFleetLlamaProfileTest(id);
    setActiveTest(t);
    return t;
  }, []);

  useEffect(() => {
    if (!activeTest?.id) return;
    const phase = activeTest.phase ?? '';
    if (phase === 'Succeeded' || phase === 'Failed' || phase === 'Ready') return;
    const handle = window.setInterval(() => {
      void pollTest(activeTest.id).catch(() => undefined);
    }, 10000);
    return () => window.clearInterval(handle);
  }, [activeTest?.id, activeTest?.phase, pollTest]);

  return (
    <PageSection variant={PageSectionVariants.light}>
      <Title headingLevel="h1" size="2xl">
        Llama profiles
      </Title>
      <p>
        Manage <code>llama-cpp-helm</code> values profiles and verify them on GPU clusters.{' '}
        <Link to="/apps">Back to Apps</Link>
      </p>
      {error ? <ErrorAlert error={error} /> : null}
      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <Button variant="primary" icon={<PlusIcon />} onClick={() => setCreateOpen(true)}>
              Create profile
            </Button>
          </ToolbarItem>
          <ToolbarItem>
            <Button variant="secondary" icon={<SyncIcon />} onClick={() => void refresh()}>
              Refresh
            </Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>
      {loading ? <Spinner /> : null}
      <Table variant="compact">
        <Thead>
          <Tr>
            <Th>Name</Th>
            <Th>Description</Th>
            <Th>Chart</Th>
            <Th>System</Th>
            <Th>Actions</Th>
          </Tr>
        </Thead>
        <Tbody>
          {profiles.map((p) => (
            <Tr key={p.name}>
              <Td>{p.name}</Td>
              <Td>{p.description || '—'}</Td>
              <Td>{p.chartVersion || p.chart || '—'}</Td>
              <Td>{p.system ? 'yes' : 'no'}</Td>
              <Td>
                <Button
                  variant="link"
                  onClick={() => {
                    void previewFleetLlamaProfileValues(p.name).then((yaml) => {
                      setValuesPreview(yaml);
                      setValuesOpen(true);
                    });
                  }}
                >
                  Preview values
                </Button>
                <Button variant="link" onClick={() => setVerifyProfile(p)}>
                  Verify
                </Button>
                {!p.system ? (
                  <>
                    <Button variant="link" onClick={() => setEditProfile(p)}>Edit</Button>
                    <Button
                      variant="link"
                      icon={<TrashIcon />}
                      onClick={() =>
                        void deleteFleetLlamaProfile(p.name)
                          .then(refresh)
                          .catch((e) => setError(String(e)))
                      }
                    >
                      Delete
                    </Button>
                  </>
                ) : null}
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>

      <ProfileFormModal
        open={createOpen}
        title="Create llama profile"
        onClose={() => setCreateOpen(false)}
        onSubmit={async (body) => {
          await createFleetLlamaProfile(body);
          setCreateOpen(false);
          await refresh();
        }}
      />
      <ProfileFormModal
        open={!!editProfile}
        title={`Edit ${editProfile?.name ?? ''}`}
        initial={editProfile}
        onClose={() => setEditProfile(null)}
        onSubmit={async (body) => {
          if (!editProfile) return;
          await patchFleetLlamaProfile(editProfile.name, {
            description: body.description,
            profileYaml: body.profileYaml,
            chartVersion: body.chartVersion,
          });
          setEditProfile(null);
          await refresh();
        }}
      />

      <Modal
        variant={ModalVariant.medium}
        title="Merged values preview"
        isOpen={valuesOpen}
        onClose={() => setValuesOpen(false)}
      >
        <TextArea readOnly value={valuesPreview} rows={18} />
      </Modal>

      <VerifyModal
        profile={verifyProfile}
        clusters={gpuClusters}
        onClose={() => setVerifyProfile(null)}
        onStarted={(t) => {
          setVerifyProfile(null);
          setActiveTest(t);
          setChatReply('');
        }}
        onError={setError}
      />

      {activeTest ? (
        <PageSection>
          <Title headingLevel="h2" size="lg">
            Verify session {activeTest.id}
          </Title>
          <p>
            Profile <strong>{activeTest.profileName}</strong> on <strong>{activeTest.clusterName}</strong>{' '}
            — phase <strong>{activeTest.phase}</strong>
            {activeTest.message ? ` (${activeTest.message})` : ''}
          </p>
          {activeTest.mode === 'extended' && activeTest.phase === 'Ready' ? (
            <>
              <LabeledField label="Chat prompt" fieldId="llama-test-chat">
                <TextInput
                  id="llama-test-chat"
                  value={chatPrompt}
                  onChange={(_e, v) => setChatPrompt(v)}
                />
              </LabeledField>
              <Button
                className="pf-v5-u-mt-md"
                isDisabled={chatBusy || !chatPrompt.trim()}
                onClick={() => {
                  setChatBusy(true);
                  void chatFleetLlamaProfileTest(activeTest.id, {
                    messages: [{ role: 'user', content: chatPrompt.trim() }],
                    max_tokens: 128,
                  })
                    .then((r) => setChatReply(JSON.stringify(r, null, 2)))
                    .catch((e) => setError(String(e)))
                    .finally(() => setChatBusy(false));
                }}
              >
                Send
              </Button>
              {chatReply ? <TextArea readOnly className="pf-v5-u-mt-md" value={chatReply} rows={10} /> : null}
            </>
          ) : null}
          <Button
            className="pf-v5-u-mt-md"
            variant="danger"
            onClick={() =>
              void endFleetLlamaProfileTest(activeTest.id)
                .then(() => setActiveTest(null))
                .catch((e) => setError(String(e)))
            }
          >
            End test
          </Button>
        </PageSection>
      ) : null}
    </PageSection>
  );
}

function ProfileFormModal({
  open,
  title,
  initial,
  onClose,
  onSubmit,
}: {
  open: boolean;
  title: string;
  initial?: FleetLlamaProfile | null;
  onClose: () => void;
  onSubmit: (body: {
    id: string;
    description?: string;
    profileYaml?: string;
    chartVersion?: string;
  }) => Promise<void>;
}) {
  const [id, setId] = useState('');
  const [description, setDescription] = useState('');
  const [profileYaml, setProfileYaml] = useState('');
  const [chartVersion, setChartVersion] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setId(initial?.name ?? '');
    setDescription(initial?.description ?? '');
    setProfileYaml(initial?.profileYaml ?? '');
    setChartVersion(initial?.chartVersion ?? '');
    setErr(null);
  }, [open, initial]);

  return (
    <Modal variant={ModalVariant.medium} title={title} isOpen={open} onClose={onClose}>
      {err ? <ErrorAlert error={err} /> : null}
      {!initial ? (
        <LabeledField label="Name" fieldId="llama-prof-id">
          <TextInput id="llama-prof-id" value={id} onChange={(_e, v) => setId(v)} />
        </LabeledField>
      ) : null}
      <LabeledField label="Description" fieldId="llama-prof-desc">
        <TextInput id="llama-prof-desc" value={description} onChange={(_e, v) => setDescription(v)} />
      </LabeledField>
      <LabeledField
        label="Profile YAML overlay"
        fieldId="llama-prof-yaml"
        help={{ ...getFieldHelp('llamaProfiles.profileYaml')!, popoverMaxWidth: '32rem' }}
      >
        <TextArea id="llama-prof-yaml" value={profileYaml} onChange={(_e, v) => setProfileYaml(v)} rows={12} />
      </LabeledField>
      <LabeledField label="Chart version" fieldId="llama-prof-ver">
        <TextInput id="llama-prof-ver" value={chartVersion} onChange={(_e, v) => setChartVersion(v)} />
      </LabeledField>
      <Button
        className="pf-v5-u-mt-md"
        isDisabled={busy}
        onClick={() => {
          setBusy(true);
          void onSubmit({
            id: id.trim(),
            description: description.trim() || undefined,
            profileYaml,
            chartVersion: chartVersion.trim() || undefined,
          })
            .catch((e) => setErr(String(e)))
            .finally(() => setBusy(false));
        }}
      >
        Save
      </Button>
    </Modal>
  );
}

function VerifyModal({
  profile,
  clusters,
  onClose,
  onStarted,
  onError,
}: {
  profile: FleetLlamaProfile | null;
  clusters: { name: string }[];
  onClose: () => void;
  onStarted: (t: FleetLlamaProfileTest) => void;
  onError: (msg: string) => void;
}) {
  const [cluster, setCluster] = useState('');
  const [mode, setMode] = useState<'quick' | 'extended'>('quick');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (profile && clusters.length && !cluster) {
      setCluster(clusters[0].name);
    }
  }, [profile, clusters, cluster]);

  if (!profile) return null;

  return (
    <Modal
      variant={ModalVariant.medium}
      title={`Verify ${profile.name}`}
      isOpen={!!profile}
      onClose={onClose}
    >
      <LabeledField label="Cluster" fieldId="llama-verify-cluster">
        <FormSelect
          id="llama-verify-cluster"
          value={cluster}
          onChange={(_e, v) => setCluster(v)}
        >
          {clusters.map((c) => (
            <FormSelectOption key={c.name} value={c.name} label={c.name} />
          ))}
        </FormSelect>
      </LabeledField>
      <LabeledField label="Mode" fieldId="llama-verify-mode">
        <Radio
          id="llama-verify-quick"
          name="verify-mode"
          label="Quick (smoke test, remove app)"
          isChecked={mode === 'quick'}
          onChange={() => setMode('quick')}
        />
        <Radio
          id="llama-verify-ext"
          name="verify-mode"
          label="Extended (keep app, hub-proxied chat)"
          isChecked={mode === 'extended'}
          onChange={() => setMode('extended')}
        />
      </LabeledField>
      <Button
        className="pf-v5-u-mt-md"
        isDisabled={busy || !cluster}
        onClick={() => {
          setBusy(true);
          void verifyFleetLlamaProfile(profile.name, { clusterName: cluster, mode, wait: mode === 'quick' })
            .then(onStarted)
            .catch((e) => onError(String(e)))
            .finally(() => setBusy(false));
        }}
      >
        {busy ? <Spinner size="md" /> : 'Start verify'}
      </Button>
    </Modal>
  );
}
