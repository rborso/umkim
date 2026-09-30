# YAIDAP and UMKIM — two homelab PoCs

Hello to everyone reading this. I want to introduce two proof-of-concept projects I have been building for my lab: **YAIDAP** (Ansible-first metal and VM provisioning) and **UMKIM** (a hub-centric manager for several independent K3s clusters). They are separate codebases, but UMKIM can call YAIDAP on a site jumphost when you do not want to install nodes by hand.

Both are experiments, not production platforms. They exist because I wanted to see whether Ansible and a small hub can carry real multi-site Kubernetes operations in a weird home network—not because every problem needs this shape of solution.

---

## YAIDAP

YAIDAP is the older project. It has been rewritten more than once and was originally my learning playground for Ansible, before I sat the older RHCE-style exam. The joke I tell myself is that Ansible is the hammer and everything is a nail. In practice it is an Ansible-driven infrastructure-as-code stack aimed at **building the lab that runs the lab**.

For a long time it also deployed services inside VMs. I narrowed the scope to what I still use every day:

- An **inventory schema** for hosts, networks, and disks
- **Host prepare** and **OS provisioning** (iPXE / kickstart)
- **Libvirt** host preparation and configuration
- **VM create / destroy** on those hypervisors

I never really got into Terraform, and I was curious whether bare-metal and libvirt lifecycle could live entirely in Ansible (whether it *should* is a different question).

The YAIDAP source tree lives at **`~/yaidap`** on my machines. The schema contract is documented in **`~/yaidap/docs/inventory-model.md`** and mirrored in sanitized form under `inventory/example/`. Real lab data is in trees like `inventory/lab/` (Ansible controller **`bck`**).

### Inventory directory layout

Each site or lab is one inventory root (for example `inventory/lab/` or `inventory/example/`):

```text
inventory/<site>/
  group_vars/all/all.yml    # shared networks, storages, TempXE defaults, SSH keys
  group_vars/<group>/       # optional group overrides (e.g. routers + vault)
  hypervisors.yml           # bare-metal / libvirt hypervisor hosts
  guests.yml                # VM guests (libvirt or Proxmox)
  routers.yml               # OpenWrt targets (optional)
  controllers.yml           # Ansible controller / TempXE anchor (optional)
  host_vars/<hostname>.yml  # per-host os, uplinks, disks, vm_location, …
  generated/                # discovery drafts (not hand-edited long-term)
    host_vars/<host>.yml
    host_vars/<host>.notes.json
```

Playbooks are tag-driven under `playbooks/` (`validate-all`, `configure-hypervisors`, `configure-tempxe`, `provision-vms`, `discover-hosts`, …). Almost all behavior is data in the YAML above, not hard-coded host names.

### Shared catalog (`group_vars/all/all.yml`)

These objects are **global names** every host references. Per-host files attach to them via `uplinks` and `disks`.

#### `networks`

Canonical L3 (and default L2) definitions. Uplinks refer to them by `network_name`.

| Field | Meaning |
| --- | --- |
| `network_name` | Unique id used from `uplinks[].networks[]` |
| `cidr` | Network CIDR |
| `gw` | Default gateway |
| `dns` | DNS server |
| `domain` | DNS search suffix |
| `tag` | Default VLAN id; `""` = untagged |
| `physical_network` | Default fabric / trunk name |
| `default_route` | Whether this L3 owns the host default route |
| `dhcp_start` / `dhcp_end` | Optional range hints (TempXE / lab DHCP) |

The same `network_name` can appear on **multiple NICs** with different effective VLAN tags—see uplink overrides below.

Example (from `inventory/example/`; names are fictional):

```yaml
networks:
  - network_name: mgmt_nw
    cidr: 192.168.1.0/24
    gw: 192.168.1.1
    tag: ""
    default_route: true
  - network_name: lab_nw
    cidr: 172.16.10.0/24
    gw: 172.16.10.1
    tag: "10"
    default_route: false
```

#### `storages`

Backend catalog for **hypervisor prepare** and **guest disks**. Filter rows with `os:` (family prefix) and/or `hosts:` so Rocky libvirt and Proxmox can share one file.

| Field | Meaning |
| --- | --- |
| `name` | Id referenced by guest `disks[].storage` |
| `type` | `local`/`dir`, `nfs`, `lvmthin`, `unused`, … (validated per hypervisor OS) |
| `path` | Directory or mountpoint |
| `disk` | Points at a host `disks[].name` for an extra bare-metal disk |
| `server` / `export` | NFS server and export |
| `builtin` | Skip create (e.g. installer-created PVE `local-lvm`) |
| `os` / `hosts` | Scope entry to OS family or hostname |

```yaml
storages:
  - name: local
    type: local
    path: /opt/local
    os: [rocky]
  - name: toshiba
    type: unused          # flip to lvmthin/dir when prepare should use the disk
    disk: disk1
    hosts: [virt03]
```

#### Site-wide defaults (same file)

Alongside `networks` and `storages`, `group_vars/all/all.yml` carries VM defaults (`memory`, `cores`, `domain`), **`libvirt_dir`**, **`tempxe_*`** (server IP, bind interface, discovery flags), **`ssh_pubkey`**, and optional **`cloudinit_*`** for guest create. My lab inventory sets `tempxe_server_ip` and `tempxe_bind_interface` on the controller so TempXE DHCP/TFTP binds on the internal lab bridge, not whatever address Ansible guessed from localhost facts.

### Per-host: `disks` and `uplinks`

**`disks`** describe boot/data layout on bare metal or sizing for VMs:

| Field | Meaning |
| --- | --- |
| `name` | Label (`bootdisk`, `disk1`, …) |
| `disk` | `sda`, `nvme0n1`, or stable `disk/by-id/…` on iron |
| `bootdisk` | OS install target |
| `size` | Size in GiB |
| `storage` | Guest: `storages[].name` on the target hypervisor |
| `format` | VM format (`qcow2`, `raw`) |

Prefer **`disk/by-id/…`** on bare metal so `sda`/`sdb` swaps do not break kickstart after reboot.

**`uplinks`** are the multi-NIC model: each uplink is a physical port or bond carrying one or more attached networks.

| Field | Meaning |
| --- | --- |
| `uplinks[].name` | Uplink id; bridge names derive from it |
| `mac_address` | Stable id; `prepare` resolves kernel ifname from MAC |
| `physical_network` | Fabric when attachment does not override |
| `networks[].network_name` | Must exist in shared `networks` |
| `networks[].tag` | Optional VLAN override for this attachment only |
| `networks[].ip` | Host address on that L3 |
| `networks[].vm_network` | Hypervisor should expose a guest bridge |
| `networks[].zone` / `tcp_open` / … | firewalld when enabled |

Effective VLAN = attachment `tag` if set, else `networks[name].tag`. That is how **one L3 (`lab_nw`) on tagged trunk vs untagged access** works (`inventory/example/host_vars/hv-dual-l1.yml`).

Truncated hypervisor example:

```yaml
uplinks:
  - name: n0p0
    physical_network: default_l2
    mac_address: "00:11:22:33:44:55"
    networks:
      - network_name: mgmt_nw
        vm_network: true
        ip: "{{ ansible_host }}"
      - network_name: lab_nw
        vm_network: true
        bridge_name: n0p0_lab_nw
        ip: "172.16.10.2"
```

Guests add **`vm_location`** (hypervisor hostname or `least-utilized`), **`os`**, and often **`hypervisor_uplink`** when the HV exposes the same `network_name` on more than one bridge.

### Ansible groups (root YAML files)

| File | Group | Purpose |
| --- | --- | --- |
| `hypervisors.yml` | `hypervisors` | Iron or nested HV; `ansible_host` is SSH target |
| `guests.yml` | `guests` | Libvirt/Proxmox VMs; includes `vm_location` in host_vars |
| `routers.yml` | `routers` | OpenWrt (`configure-routers.yml`) |
| `controllers.yml` | `controllers` | Machine that runs TempXE / discovery output paths |

Membership is plain Ansible: hostname under `hosts:` plus `host_vars/<hostname>.yml`. Cluster intent for UMKIM can also use root `{cluster}.yml` and `group_vars/{cluster}.yml` when the hub syncs inventory—but the YAIDAP-native core is still **hypervisors + guests + shared catalog**.

### How inventory is built (discovery → validate → prepare → create)

Most new bare-metal hosts follow the same pipeline documented in `docs/discovery-to-create.md`:

1. **Seed** — Put the hostname in `hypervisors.yml` with at least a **MAC** (and `ansible_host` once you know management IP).
2. **Discover** — Either SSH `discover-hosts.yml --tags check_host_details`, or PXE **`tempxe_discovery`** (Rocky Anaconda collector) and `configure-tempxe.yml --tags collect_discovery`. Output lands in **`generated/host_vars/<host>.yml`** plus **`.notes.json`** (hardware detail for humans, GPU PCI ids, NIC chipsets, …).
3. **Edit** — Replace `CHANGE_ME` placeholders: map uplinks to `network_name` entries in `group_vars/all`, set `vm_network`, disk `by-id`, TempXE flags (`install_os`, `pxe` on the right uplink).
4. **Promote** — Copy the draft to **`host_vars/<host>.yml`** (stop treating `generated/` as source of truth).
5. **Validate** — `validate-all.yml --tags validate`.
6. **Prepare** — `configure-hypervisors.yml --tags prepare` (bridges, pools, firewalld optional).
7. **Create guests** — Define `host_vars/my-vm.yml`, add to `guests.yml`, run `provision-vms.yml --tags create`.

PXE **OS install** and **discovery** share TempXE; after a successful install, post-install logic trims kickstart so the next boot chainloads disk. Discovery can run with **`reboot_after_discovery: false`** so a laptop without iLO stays in the live environment until you reboot manually.

Beyond OS install, the same inventory drives **libvirt VM create/destroy**, optional **Proxmox**, **OpenWrt ZTP**, and **GPU passthrough** fields (`gpu_passthrough_host` / `gpu_passthrough`) documented in the YAIDAP repo.

### Provisioning flow (TempXE / iPXE)

Many of my nodes are half-notebooks, so provisioning relies on **iPXE**. The internal lab LAN has **no DHCP** in steady state (install-time DHCP comes from the **TempXE Podman container** on the controller, not from OpenWrt).

When install is armed for a host:

1. TempXE serves DHCP (for the install window), TFTP, and kickstart/iPXE menus.
2. The machine PXE-boots, installs Rocky (or discovery-only, or Proxmox on supported paths).
3. When install is off, DHCP **chainloads local disk** so day-to-day reboots stay fast.

That lines up with the **`networks` / `uplinks` / `tempxe_*`** fields above: you declare *which* bridge and VLAN TempXE binds to, *which* MAC gets an install menu, and *which* disk id kickstart must wipe.

---

## UMKIM (v0.2)

UMKIM is a **management plane for multiple Kubernetes clusters**, not a single giant cluster with workers attached to one control plane. The design shifted a lot from v0.1 to **v0.2**; what follows is the current model.

### Hub

There is a **management hub**: a K3s cluster on a dedicated host (in my lab, often the external VPS). The hub runs, among other things:

- **Open Cluster Management (OCM)** — register and manage spoke clusters
- **Harbor** — images and Helm charts
- **WireGuard** — overlay between hub and sites
- **Fleet console and edge routing** — operator UI and optional hub-gateway HTTP(S)

Spoke clusters are **independent K3s control planes**. The hub does not run their workloads; it enrolls them, pushes platform pieces (OLM, cert-manager, ingress helpers), and installs **FleetApp** charts onto clusters you name explicitly.

### Sites

A **site** is a logical boundary: a datacenter corner, a room, or “everything behind the same jumphost.” Each site is a `FleetSite`. Sites can define **zero or more ingress methods** that describe how traffic from the Internet (or a WAN) reaches clusters in that site:

| Pattern | Idea |
| --- | --- |
| **Port forward** | Public IP ports 80/443 forwarded to a cluster VIP or single-node address |
| **Reverse proxy** | An external reverse proxy terminates TLS and routes subdomains to the cluster |
| **Hub gateway** | No site-local WAN ingress; HTTP(S) published on the hub and forwarded over WireGuard |
| **Private** | No fleet-managed WAN path; clusters still join over the overlay |

Each site can host **multiple clusters**.

### Clusters

Every cluster belongs to **exactly one site**. Supported shapes include **single-node** clusters and **three-node control-plane HA** (plus optional workers).

When you create a cluster in the console, the hub issues a **bootstrap bundle**: WireGuard client material, K3s install steps, and enrollment hooks so the spoke registers with the hub. After local validation, the hub **provisions** OCM registration and platform manifests until the cluster is **Ready**. Then you install applications with **FleetApp** (Helm from Harbor/OCI)—there is no automatic placement engine; you pick the cluster.

Lifecycle phases look like: Pending → ScriptIssued → Enrolled → Validated → Provisioning → **Ready** (with **Failed** and recovery paths documented in the UMKIM repo).

### YAIDAP integration

You do not have to PXE or libvirt-provision from UMKIM. The default site provisioner is **`noop`**: you SSH to already-installed nodes and run the bootstrap bundle.

If a site uses **`provider: yaidap`**, UMKIM stores **intent and inventory on the hub**, SSHs to a **site jumphost**, and runs YAIDAP playbooks there (via hub Jobs). YAIDAP still owns kickstart, bridges, disks, and libvirt details; UMKIM owns cluster enrollment once guests are SSH-ready. That is how “provision the hosts over a jumphost” fits together.

For the full connector contract, see [docs/components/yaidap-integration.md](docs/components/yaidap-integration.md) in the UMKIM repository.

---

## Test lab

Everything above is tested on a deliberately odd home setup: one rented VPS, a pile of used notebooks, one desktop, and a management router built from spare hardware. This section is **my** lab layout (IPs and topology), not the topology-neutral design docs in the UMKIM tree.

### Hardware

| Role | Machine | Notes |
| --- | --- | --- |
| **External hub / WAN-facing services** | Netcup VPS | 4 vCPU, 8 GiB RAM; runs UMKIM hub pieces that need a stable public footprint |
| **Compute node** | Notebook ×2 | Intel i5-8350U, 32 GiB RAM each |
| **Compute + GPU** | Notebook ×1 | Intel i7-7700HQ, 16 GiB RAM, NVIDIA GTX 1050 Ti (4 GiB) |
| **Compute + GPU** | Desktop | Intel i5-8400, 32 GiB RAM, NVIDIA GTX 1650 |
| **Management / routing (`bck`)** | Notebook | OpenWrt; PPPoE passthrough, internal routing, jumphost-style access |

The four compute nodes (two i5 notebooks, i7 notebook, desktop) hang off the same switch as **`bck`**. The VPS is outside this L2 domain and reaches the lab over VPN, SSH, and UMKIM’s WireGuard overlay.

### Network topology

Two logical networks share one physical unmanaged switch, with **`bck`** in the middle:

```text
                    ┌─────────────────┐
                    │ Provider router │
                    │ 192.168.1.0/24  │  DHCP on
                    └────────┬────────┘
                             │
              ┌──────────────┴──────────────┐
              │  VLAN 10 (on provider LAN) │
              │  Internal: 172.16.10.0/24  │  No DHCP
              └──────────────┬──────────────┘
                             │
                    ┌────────┴────────┐
                    │  bck (OpenWrt)  │
                    │  PPPoE passthrough
                    │  USB NIC ───────┼──► unmanaged switch ◄── onboard LAN
                    └─────────────────┘         │
                    unique public IP on USB       ├── 4× compute nodes
                    (separate from other WAN)     └── (iPXE / lab traffic)
```

**Provider network (`192.168.1.0/24`)**

- Behind the ISP router; **DHCP enabled**.
- **`bck`** participates on this network (including **VLAN 10** carrying the internal subnet).

**Internal lab network (`172.16.10.0/24`)**

- Routed/served by **`bck`** on **VLAN 10** of the provider LAN.
- **No DHCP** on this segment during normal operation—important for YAIDAP iPXE (install-time DHCP comes from the ephemeral Podman helper on the Ansible controller, not from the router).
- The four compute nodes use their **onboard Ethernet** on the unmanaged switch.

**Dual-WAN detail on `bck`**

- **`bck`** uses an **USB NIC** cabled to the same unmanaged switch (untagged).
- The **provider-facing** path and the **USB NIC** path each have their **own public IP** (PPPoE passthrough and separate addressing). That lets me expose or test WAN behavior without collapsing everything into one upstream profile.

### How the pieces map

- **YAIDAP** runs against inventory rooted on a jumphost (often reachable via **`bck`** and port forwards). PXE and kickstart target the **172.16.10.0/24** lab LAN; hypervisors and guests are the notebooks and desktop on the switch.
- **UMKIM hub** often lives on the **Netcup VPS**, with **sites** representing “homelab behind `bck`” and clusters enrolled over **WireGuard** once nodes (whether hand-installed or YAIDAP-provisioned) are up.
- **GPU clusters** use the i7 + 1050 Ti and the desktop + 1650 for device-plugin and workload experiments; the hub-side GPU enable playbooks assume the spoke already joined and validated storage/network.

If you are reproducing any of this, treat the numbers as one operator’s lab—not a required UMKIM topology. Greenfield install steps live in [docs/install.md](docs/install.md); inventory templates under `examples/inventory/` stay separate from this write-up.

---

## Where to read more

| Project | Starting point |
| --- | --- |
| **UMKIM** (this repo) | [README.md](README.md), [docs/design.md](docs/design.md), [docs/install.md](docs/install.md) |
| **YAIDAP** (`~/yaidap`) | `README.md`, `docs/inventory-model.md`, `docs/discovery-to-create.md`, `inventory/example/` |
| **YAIDAP + UMKIM connector** | [docs/components/yaidap-integration.md](docs/components/yaidap-integration.md), `~/yaidap/docs/integrations/umkim.md`, [docs/components/fleet-site.md](docs/components/fleet-site.md) |

Thanks for reading—both repos are PoCs, and I am still learning in public.
