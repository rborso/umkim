import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const CATALOG_COMMON_FIELD_HELP: Record<string, FieldHelpContent> = {
  'metadata.name': {
    description:
      'Kubernetes resource name for the workload (DNS label). Used as container name, Helm release name, or subscription name depending on service type.',
    example: 'demo-webapp',
    examples: ['whoami-helm', 'pet-clinic'],
  },
  'metadata.labels.app': {
    description: 'App label applied to the deployed workload for identification and routing.',
    example: 'demo-webapp',
  },
  'image.reference': {
    description: 'Full container image reference pulled by the container service provider on each placed cluster.',
    example: 'harbor.example.com:30443/fleet/whoami:0.1.0',
  },
  'release.namespace': {
    description: 'Kubernetes namespace where the Helm release is installed on the target cluster.',
    example: 'default',
  },
  'values': {
    description:
      'Helm values YAML merged into the chart defaults. Use for app-specific config, ingress hosts, persistence, and bootstrap secrets.',
    example: 'replicaCount: 1\nservice:\n  port: 80',
    whenToUse: 'Override chart defaults not exposed as separate catalog fields.',
    learnMoreUrl: DOCS.deployApp,
    commonMistakes: ['Invalid YAML — use Validate on the Configure step before deploying.'],
  },
  'chart.repository': {
    description: 'OCI registry URL hosting the Helm chart (Harbor charts project on the hub).',
    example: 'oci://10.10.0.1:30443/charts',
  },
  'chart.name': {
    description: 'Chart name within the OCI repository.',
    example: 'whoami',
  },
  'chart.version': {
    description: 'Semver chart version to install. Pin for reproducible deploys.',
    example: '0.1.0',
  },
  'install.wait': {
    description: 'When true, Helm waits for resources to become ready before marking the install successful.',
    example: 'true',
  },
  'install.timeout': {
    description: 'Maximum seconds Helm waits during install/upgrade before failing.',
    example: '1800',
  },
  'install.atomic': {
    description: 'When true, Helm rolls back automatically if the install fails partway through.',
    example: 'true',
  },
  'routing.mode': {
    description:
      'How fleet edge exposes the app: hub (hub Traefik), site (cluster Traefik), or none (cluster ingress only).',
    example: 'hub',
    examples: ['hub', 'site', 'none'],
    whenToUse:
      'hub for fleet-wide URLs; site when the cluster Traefik serves the app; none when the chart owns ingress.',
    relatedFields: ['routing.site_host', 'routing.cluster', 'routing.require_auth'],
    commonMistakes: ['Setting site without routing.site_host when DNS is not auto-derived.'],
    learnMoreUrl: DOCS.fleetEdge,
  },
  'routing.require_auth': {
    description:
      'Opt in to Authelia forward-auth for this instance. Hub mode protects the hub URL; site mode protects the site hostname on spoke Traefik. Default is off.',
    example: 'false',
    examples: ['true', 'false'],
    whenToUse:
      'Enable when the app should require login. Hub mode needs Infrastructure ingress auth Ready; site mode needs cluster ingress forward-auth Ready.',
    relatedFields: ['routing.mode', 'routing.cluster'],
    commonMistakes: [
      'Enabling without hub or cluster ingress auth provisioned first.',
    ],
    learnMoreUrl: DOCS.ingressAuth,
  },
  'routing.cluster': {
    description:
      'Optional OCM ManagedCluster pin. When empty, policies select a cluster from fleet.site and capability labels.',
    example: 'spoke-a',
    examples: ['spoke-a', 'spoke-b'],
  },
  'routing.health_path': {
    description: 'HTTP path used for hub edge health checks on the routed service.',
    example: '/',
  },
  'routing.failover_mode': {
    description:
      'Optional override for instance failover behavior: manual, relocatable, or standby. Leave empty to use catalog defaults.',
    examples: ['manual', 'relocatable', 'standby'],
    learnMoreUrl: DOCS.failover,
  },
  'fleet.ingress.baseDomain': {
    description:
      'Base domain for per-app HTTPS hostnames when using chart-managed cluster ingress (e.g. git.homelab.example.com).',
    example: 'site.example.com',
  },
  'values.fleet.ingress.baseDomain': {
    description:
      'DNS zone for chart-managed ingress. Pick a hub or site zone, or enter a custom domain.',
    example: 'site.example.com',
    relatedFields: ['values.fleet.ingress.host', 'values.__preset.exposure'],
  },
  'values.fleet.ingress.host': {
    description: 'Hostname prefix before the base domain (e.g. git → git.example.com).',
    example: 'git',
  },
  'values.__preset.exposure': {
    description:
      'How the app is exposed: hub FleetAppRoute, site spoke Traefik, or chart-managed ingress on the cluster.',
    examples: ['hub', 'site', 'chart_ingress'],
  },
  'routing.hub_host': {
    description:
      'Optional full hub URL hostname. Leave empty to use {release}.{hub-zone} from the fleet edge registry.',
    example: 'gitea.apps.example.com',
  },
  'routing.site_host': {
    description:
      'Optional FQDN for site-local routing when mode is site. Leave empty to derive from hostname prefix + site zone.',
    example: 'whoami.apps.mgmt.umkim.com',
  },
  'subscription.package': {
    description: 'OLM package name to subscribe to (operator bundle). Must exist in the CatalogSource.',
    example: 'grafana-operator',
    whenToUse: 'Match the package name from operator catalog metadata.',
    relatedFields: ['subscription.channel', 'subscription.source'],
  },
  'subscription.channel': {
    description: 'OLM channel for the operator subscription (e.g. stable, fast).',
    example: 'stable',
    relatedFields: ['subscription.package'],
  },
  'subscription.source': {
    description: 'CatalogSource name providing the operator package.',
    example: 'operatorhubio-catalog',
  },
  'subscription.sourceNamespace': {
    description: 'Namespace where the CatalogSource is installed.',
    example: 'olm',
  },
  'operatorGroup.targetNamespace': {
    description: 'Namespace where the operator watches and installs operands.',
    example: 'operators',
  },
  'installPlanApproval': {
    description: 'Automatic approves OLM InstallPlans immediately; Manual requires admin approval in the cluster.',
    example: 'Automatic',
    examples: ['Automatic', 'Manual'],
    whenToUse: 'Use Manual in production clusters that require change control.',
  },
  'resources.cpu.min': {
    description:
      'Minimum CPU (Kubernetes quantity). Use millicores for sub-core requests (e.g. 200m = 0.2 core).',
    example: '200m',
    examples: ['200m', '500m', '1'],
  },
  'resources.cpu.max': {
    description: 'Maximum CPU (Kubernetes quantity). Must be >= CPU min.',
    example: '500m',
    examples: ['500m', '1', '2'],
  },
  'resources.memory.min': {
    description:
      'Minimum memory (DCM spec: MB/GB/TB suffix, e.g. 64MB). The k8s provider translates to Kubernetes Mi/Gi on the cluster.',
    example: '64MB',
    examples: ['64MB', '256MB', '1GB'],
  },
  'resources.memory.max': {
    description: 'Maximum memory (DCM spec: MB/GB/TB suffix).',
    example: '256MB',
    examples: ['256MB', '512MB', '1GB'],
  },
  'workload.replicas': {
    description: 'Pod replicas on each cluster after placement (not fleet-wide total).',
    example: '1',
  },
  'workload.gpu.count': {
    description:
      'GPU-backed vLLM instances on the single placed cluster (Phase 1). Maps to Helm replicaCount; not fleet-wide bin-pack yet.',
    example: '2',
  },
};
