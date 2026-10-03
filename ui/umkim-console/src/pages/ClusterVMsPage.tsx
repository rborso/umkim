import { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import {
  Button,
  ButtonVariant,
  Modal,
  PageSection,
  PageSectionVariants,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  getFleetCluster,
  listFleetClusterVMs,
  powerFleetClusterVM,
} from '../api/client';
import type { FleetCluster, FleetVM, FleetVMPowerAction } from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { VMSerialConsole } from '../components/clusters/VMSerialConsole';
import { VMVncConsole } from '../components/clusters/VMVncConsole';
import { useAuth } from '../auth/AuthContext';
import { canMutate } from '../auth/roles';

export function ClusterVMsPage() {
  const { name: clusterName } = useParams<{ name: string }>();
  const { user } = useAuth();
  const canEdit = canMutate(user);
  const [cluster, setCluster] = useState<FleetCluster | null>(null);
  const [vms, setVms] = useState<FleetVM[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [selected, setSelected] = useState<FleetVM | null>(null);
  const [vncOpen, setVncOpen] = useState(false);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!clusterName) return;
    setError(null);
    try {
      const [c, list] = await Promise.all([
        getFleetCluster(clusterName),
        listFleetClusterVMs(clusterName),
      ]);
      setCluster(c);
      setVms(list);
      setSelected((prev) => {
        if (!prev) return list[0] || null;
        return list.find((v) => v.namespace === prev.namespace && v.name === prev.name) || list[0] || null;
      });
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [clusterName]);

  useEffect(() => {
    void load();
    const t = window.setInterval(() => void load(), 10000);
    return () => window.clearInterval(t);
  }, [load]);

  async function power(vm: FleetVM, action: FleetVMPowerAction, force = false) {
    if (!clusterName) return;
    const key = `${action}:${force}:${vm.namespace}/${vm.name}`;
    setBusy(key);
    setError(null);
    try {
      await powerFleetClusterVM(clusterName, vm.namespace, vm.name, action, force);
      await load();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  }

  if (!clusterName) {
    return null;
  }

  const kvReady =
    !!cluster?.capabilities?.kubevirt || cluster?.labels?.['fleet.capability/kubevirt'] === 'true';

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1">Virtual machines</Title>
        <p style={{ marginTop: '0.5rem' }}>
          Cluster <Link to={`/clusters/${clusterName}`}>{clusterName}</Link>
          {kvReady ? '' : ' — KubeVirt capability not reported; listing may be empty.'}
        </p>
      </PageSection>
      <PageSection>
        {error ? <ErrorAlert error={error} /> : null}
        {loading && vms.length === 0 ? <p>Loading…</p> : null}
        <Table aria-label="Virtual machines" variant="compact">
          <Thead>
            <Tr>
              <Th>Namespace</Th>
              <Th>Name</Th>
              <Th>Phase</Th>
              <Th>Ready</Th>
              <Th>Node</Th>
              <Th>IP</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {vms.length === 0 ? (
              <Tr>
                <Td colSpan={7}>No VirtualMachines found.</Td>
              </Tr>
            ) : (
              vms.map((vm) => {
                const id = `${vm.namespace}/${vm.name}`;
                const selectedRow = selected?.namespace === vm.namespace && selected?.name === vm.name;
                return (
                  <Tr key={id}>
                    <Td>
                      <Button
                        variant={ButtonVariant.link}
                        isInline
                        onClick={() => {
                          setSelected(vm);
                          setVncOpen(false);
                        }}
                      >
                        {vm.namespace}
                      </Button>
                    </Td>
                    <Td>
                      <Button
                        variant={ButtonVariant.link}
                        isInline
                        onClick={() => {
                          setSelected(vm);
                          setVncOpen(false);
                        }}
                      >
                        {vm.name}
                      </Button>
                      {selectedRow ? ' · selected' : ''}
                    </Td>
                    <Td>{vm.phase || vm.printableStatus || '—'}</Td>
                    <Td>{vm.ready ? 'yes' : 'no'}</Td>
                    <Td>{vm.nodeName || '—'}</Td>
                    <Td>{vm.ip || '—'}</Td>
                    <Td>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.35rem' }}>
                        <Button
                          variant={ButtonVariant.secondary}
                          isDisabled={!canEdit || busy !== null}
                          isLoading={busy === `start:false:${id}`}
                          onClick={() => void power(vm, 'start')}
                        >
                          Power on
                        </Button>
                        <Button
                          variant={ButtonVariant.secondary}
                          isDisabled={!canEdit || busy !== null}
                          isLoading={busy === `stop:false:${id}`}
                          onClick={() => void power(vm, 'stop')}
                        >
                          Soft off
                        </Button>
                        <Button
                          variant={ButtonVariant.danger}
                          isDisabled={!canEdit || busy !== null}
                          isLoading={busy === `stop:true:${id}`}
                          onClick={() => void power(vm, 'stop', true)}
                        >
                          Hard off
                        </Button>
                        <Button
                          variant={ButtonVariant.secondary}
                          isDisabled={!canEdit || busy !== null}
                          isLoading={busy === `restart:false:${id}`}
                          onClick={() => void power(vm, 'restart')}
                        >
                          Soft reboot
                        </Button>
                        <Button
                          variant={ButtonVariant.danger}
                          isDisabled={!canEdit || busy !== null}
                          isLoading={busy === `restart:true:${id}`}
                          onClick={() => void power(vm, 'restart', true)}
                        >
                          Hard reboot
                        </Button>
                      </div>
                    </Td>
                  </Tr>
                );
              })
            )}
          </Tbody>
        </Table>

        {selected ? (
          <div style={{ marginTop: '1.5rem' }}>
            <Title headingLevel="h2" size="lg">
              Console — {selected.namespace}/{selected.name}
            </Title>
            <p style={{ fontSize: '0.9rem', maxWidth: '48rem' }}>
              Serial console is the default session. Open VNC only when needed; it is not kept
              connected in the background.
            </p>
            <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '0.75rem' }}>
              <Button
                variant={ButtonVariant.primary}
                onClick={() => setVncOpen(true)}
                isDisabled={!selected.running && selected.phase !== 'Running'}
              >
                Open VNC
              </Button>
              <Button variant={ButtonVariant.link} onClick={() => void load()}>
                Refresh
              </Button>
            </div>
            <VMSerialConsole
              clusterName={clusterName}
              namespace={selected.namespace}
              name={selected.name}
              active={!!selected.running || selected.phase === 'Running'}
            />
            {!(selected.running || selected.phase === 'Running') ? (
              <p style={{ color: '#6a6e73' }}>Power on the VM to attach serial console.</p>
            ) : null}
          </div>
        ) : null}
      </PageSection>

      <Modal
        title={`VNC — ${selected?.namespace}/${selected?.name}`}
        isOpen={vncOpen && !!selected}
        onClose={() => setVncOpen(false)}
        variant="large"
      >
        {selected && vncOpen ? (
          <VMVncConsole
            clusterName={clusterName}
            namespace={selected.namespace}
            name={selected.name}
            open={vncOpen}
          />
        ) : null}
      </Modal>
    </>
  );
}
