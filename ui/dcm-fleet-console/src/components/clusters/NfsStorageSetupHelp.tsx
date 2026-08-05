import { Alert, ExpandableSection } from '@patternfly/react-core';

type Props = {
  clusterName: string;
  exportBase?: string;
};

export function NfsStorageSetupHelp({ clusterName, exportBase = '/srv/fleet-nfs' }: Props) {
  const clusterDir = `${exportBase}/${clusterName}`;
  const exportLine = `${exportBase} *(rw,sync,no_subtree_check,no_root_squash)`;

  return (
    <ExpandableSection
      toggleText="NFS server setup (export + directory)"
      displaySize="default"
      style={{ marginTop: '0.75rem', maxWidth: '42rem' }}
    >
      <Alert variant="info" isInline title="Before enabling NFS on the cluster" style={{ marginBottom: '0.75rem' }}>
        The fleet installs only the in-cluster nfs-subdir-external-provisioner. You must provide a
        reachable NFS server and per-cluster export subdirectory.
      </Alert>
      <p style={{ marginTop: 0 }}>
        On the <strong>NFS server</strong> (or delegate host via SSH), create the cluster directory
        and export the base path to cluster node IPs (WireGuard or LAN):
      </p>
      <pre
        style={{
          fontSize: '0.8rem',
          padding: '0.75rem',
          overflowX: 'auto',
          background: 'var(--pf-v6-global--BackgroundColor--200)',
        }}
      >
        {`# On NFS server (${exportBase} example)
sudo mkdir -p ${clusterDir}
sudo chmod 1777 ${clusterDir}

# /etc/exports (restrict to your cluster node subnet)
${exportLine}

sudo exportfs -ra
exportfs -v | grep fleet-nfs`}
      </pre>
      <p style={{ marginBottom: '0.35rem' }}>
        <strong>Console fields</strong>
      </p>
      <ul style={{ marginTop: 0, paddingLeft: '1.25rem' }}>
        <li>
          <strong>NFS server</strong> — IP/hostname reachable from cluster nodes (e.g.{' '}
          <code>198.51.100.40</code> or WireGuard <code>10.10.0.3</code>)
        </li>
        <li>
          <strong>Export base</strong> — server filesystem path shared in exports (e.g.{' '}
          <code>{exportBase}</code>), not the per-cluster subdir
        </li>
        <li>
          <strong>Cluster subdirectory</strong> — <code>{clusterName}</code> → full path{' '}
          <code>{clusterDir}</code>
        </li>
        <li>
          <strong>Client mount path</strong> — what nodes mount (usually{' '}
          <code>{clusterDir}</code>). Use NFSv4 pseudo-path <code>/{clusterName}</code> only when
          the server has an NFSv4 root export (<code>fsid=0</code>) and you know that layout.
        </li>
        <li>
          <strong>Delegate host</strong> — SSH target to run <code>mkdir -p {clusterDir}</code>{' '}
          when the NFS server is not a cluster node
        </li>
      </ul>
      <p style={{ marginBottom: 0, fontSize: '0.875rem' }}>
        Click <strong>Validate NFS</strong> before enable — it mounts the client path and creates a
        provisioner-style subdirectory (same check the dynamic provisioner needs).
      </p>
    </ExpandableSection>
  );
}
