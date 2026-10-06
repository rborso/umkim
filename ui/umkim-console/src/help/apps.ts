import type { FieldHelpContent } from './fieldHelp';

export const APPS_FIELD_HELP: Record<string, FieldHelpContent> = {
  'apps.clusterName': {
    description:
      'Ready workload cluster that will run the Helm release. You must select it; the hub does not pick a cluster.',
    example: 'lab-single-0',
  },
  'apps.chart': {
    description:
      'OCI chart URL, or a short name under the hub Harbor charts project (for example whoami).',
    example: 'oci://10.10.0.1:30443/charts/whoami',
  },
  'apps.version': {
    description: 'Chart version from Harbor. Leave empty only if the registry has a default.',
    example: '0.1.0',
  },
  'apps.releaseName': {
    description: 'Helm release name on the selected cluster. Also used as the default Service name for hub routes.',
    example: 'whoami',
  },
  'apps.namespace': {
    description: 'Spoke Kubernetes namespace. Created if missing.',
    example: 'default',
  },
  'apps.routeMode': {
    description:
      'none leaves ingress to the chart. hub publishes through the hub Traefik gateway to this cluster. site uses the cluster’s own ingress. Only applies to charts that create a Service (web apps and custom charts); KubeVirt and LLM charts manage their own networking.',
    examples: ['none', 'hub', 'site'],
  },
  'apps.hubHost': {
    description:
      'Public hostname on the hub gateway. Leave empty to use <release>.<hub edge domain>. Must resolve to the hub WAN for ACME.',
    example: 'whoami.demo.example.test',
  },
  'apps.siteHost': {
    description:
      'Public hostname on the cluster site ingress (port-forward or reverse proxy). Leave empty to use <release>.<site edge domain>.',
    example: 'whoami.homelab.example.com',
  },
  'apps.routeServiceName': {
    description:
      'Kubernetes Service the route targets in the release namespace. Defaults to the release name when empty. Custom charts must create this Service.',
    example: 'my-app',
  },
  'apps.routeServicePort': {
    description: 'Service port the route forwards to. Empty uses the controller default.',
    example: '80',
  },
  'apps.routeServiceNamespace': {
    description: 'Namespace of the routed Service. Defaults to the release namespace.',
    example: 'default',
  },
  'apps.routeHealthPath': {
    description: 'HTTP path probed to mark the route healthy. Defaults to /.',
    example: '/healthz',
  },
  'apps.valuesYaml': {
    description: 'Optional Helm values YAML merged at install time.',
    example: 'replicaCount: 1',
  },
  'apps.llamaProfile': {
    description:
      'Built-in llama-cpp-helm values profile from the hub API (runtime + model + registry image). You can edit the generated YAML before install. Open the help on Values (YAML) for extraArgs, top-p, and temp examples.',
    example: 'qwen35-2b-ud-4g',
  },
  'apps.llmBackend': {
    description:
      'When enabled, a Ready install is discovered as a hub LiteLLM backend (spoke WireGuard upstream, not the FleetApp route). Requires Infrastructure → LLM platform enabled. Uncheck for private or test installs.',
    example: 'enabled after qwen35 profile on lab-gpu-0',
  },
  'apps.llamaRoute': {
    description:
      'Fleet Llama profiles use hostNetwork on the GPU node. Hub/site routes target the chart Service and usually do not reach llama-server; hub LiteLLM uses the spoke upstream address instead. Use route only if you customized networking.',
  },
  'apps.editInstall': {
    description:
      'Patch chart version and Helm values on an existing FleetApp. Triggers helm upgrade --install on the spoke when the spec changes.',
    example: 'ctxSize: 8192',
  },
  'apps.rockyCpu': {
    description: 'vCPU cores for the Rocky Linux 10 guest (writes cpu in values).',
    example: '2',
  },
  'apps.rockyMemory': {
    description: 'Guest memory request (Kubernetes quantity).',
    example: '2Gi',
  },
  'apps.rockyDisk': {
    description: 'Root disk PVC size for the CDI import. Must cover the Rocky cloud image (~10Gi+).',
    example: '20Gi',
  },
  'apps.rockySshKeys': {
    description:
      'One SSH public key per line, added to the guest via cloud-init ssh_authorized_keys for the cloudInit.user account. Leave empty to keep password-only access (default user/password rocky/rocky on the console).',
    example: 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... you@host',
    commonMistakes: [
      'Pasting a private key instead of the .pub public key',
      'Expecting keys added to an already-booted VM to apply without a reboot / cloud-init re-run',
    ],
  },
  'apps.rockyNetMode': {
    description:
      'pod uses the cluster pod network with masquerade/NAT (guest IP is not routable from the LAN). lan attaches a second NIC through Multus to a NetworkAttachmentDefinition, giving the guest an address on the site L2 (DHCP or static).',
    examples: ['pod', 'lan'],
  },
  'apps.rockyMultusNetwork': {
    description:
      'Multus NetworkAttachmentDefinition as namespace/name. The default default/mgmt-l2 is created by Enable LAN Multus on the KubeVirt platform. Its bridge CNI has no IPAM, so the guest does DHCP or static addressing.',
    example: 'default/mgmt-l2',
  },
  'apps.rockyStaticIP': {
    description:
      'Static IPv4 in CIDR form, written into the guest as a NetworkManager keyfile (Rocky GenericCloud ignores cloud-init network v2). Leave empty to use DHCP on the L2. Requires Network = lan.',
    example: '172.16.10.150/24',
  },
  'apps.rockyGateway': {
    description: 'Default gateway for the static LAN address (usually the L2 router).',
    example: '172.16.10.1',
  },
  'apps.rockyDns': {
    description: 'DNS servers for the static LAN address (comma or space separated).',
    example: '172.16.10.1, 1.1.1.1',
  },
  'apps.rockyInterface': {
    description:
      'Guest NIC name that receives the static address. Must match the VM interface (enp1s0 on q35 virtio).',
    example: 'enp1s0',
  },
  'apps.libraryChart': {
    description:
      'Ready library chart on the hub. Short names still work; they expand to Harbor charts.',
    example: 'whoami',
  },
  'charts.origin': {
    description: 'Upstream oci:// URL or an https:// link to a .tgz. Private charts should be uploaded instead.',
    example: 'oci://ghcr.io/example/app',
  },
  'charts.imagePolicy': {
    description:
      'external leaves image pulls on upstream registries. proxy uses Harbor pull-through. copy mirrors discovered images into Harbor.',
    examples: ['external', 'proxy', 'copy'],
  },
  'charts.upload': {
    description: 'Helm package (.tgz). Chart.yaml name and version become the library identity.',
    example: 'whoami-0.1.0.tgz',
  },
};
