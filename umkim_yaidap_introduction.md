Hello y'all who's reading this, I would like to introduce two of my experimental PoC projects, yaidap (Infrastructure as code) and umkim (kubernetes fleet management). 

Umkim

Umkim is meant to be a management cluster for multiple kubernetes clusters. The idea here changed a lot from v0.1 to v0.2, I'll talk about the current, v0.2.
There is a management host, which is a k3s cluster, that manages multiple, independent k3s clusters that are connected via wireguard, and belongs to logical units, called sites, and operations are mostly run via ansible, inside pods. For now, everything is tested and developed only on rocky linux 10.
Umkim hub
The hub is a k3s cluster with wireguard, multiple services for different purposes. The hub can not run workloads, it is for management only, running open cluster manager to manage the workload clusters, and harbor for images and charts also for both the hub and the workload clusters. The idea is a helm chart is uploaded, which can be reused for multiple clusters.
For routing, the base idea is that some clusters might not have exposed ingress from internet, so there’s an option for the umkim hub to be used as a reverse proxy. For that, letsencrypt certs are being used, and it is possible to protect the resources with authelia middleware, so the sites can be reached only after authenticating to authelia. Authelia uses lldap running on the umkim hub. For workload clusters that has nvidia gpu, llm inference can be configured via llama.cpp, and added these to the litellm proxy as a provider. On the console, an api key can be generated to access the litellm api. For models deployed with the same name, load balancing with sticky session is the default. 
Sites are logical separations for clusters, each site can have zero or more ingresses, like port forward, reverse proxy, these represent the connectivity from the internet to the site and the site’s clusters.
Each cluster needs to be part of one site, and can be a single node cluster, or a 3node ha (with additional workers). A cluster can have routing via port forward, where a public IP's port 80 and 443 is pointing to the shared VIP of the cluster (or for single node the node's internal IP), it can have external reverse proxy, where an external reverse proxy is configured with one or more subdomains, or it can have no external routing. For those with no external routing, the hub-gateway can be used.
On each managed cluster traefik reverse proxy, letsencrypt, lldap and authelia can be enabled.
When a cluster entry is created on the console, it generates a bootstrap bundle. This bootstrap bundle contains everything to create the k3s cluster, and connect it to the umkim-hub, it needs to be downloaded and moved to the preinstalled rocky-10 host. There is also an option to integrate yaidap, create VMs, and provision hosts over a jumphost. As yaidap is optional, and is a project on it’s own, it has it’s own github repo.
For each operation after the umkim hub is installed, the console, or the umkim cli can be used.
Yaidap
So, this project is older, and had seen many changes, rewrites, and was basically my learning project for ansible, before I did the old-system RHCE exam. Basically here ansible is the hammer, and everything is a nail. The project itself is something like an ansible driven infrastructure as code thingy. There were many changes on this, but it was meant to be my homelab-building project that builds itself. For a long time it contained services in VMs as well, but I have narrowed it to inventory schema, host prepare and provisioning, libvirt prepare and configure, and vm creation. I needed that, because somehow I never really got into terraform, and also because I wanted to know if it can be done (not if it should). I have the following lab:
4x machines for testing (2 of them with gpu), and 1x for management, running the openwrt VM and acting as the jumphost for the umkim yaidap provisioning.
Provider network is 192.168.1.0/24, with the provider router, dhcp enabled, PPPoE PT. Own external IP.
Internal network is 172.16.10.0/24, served by the openwrt VM. Own external IP, separate from the provider network’s one.
As IPXE needs untagged VLANs, and I cannot turn off dhcp on provider network, on provider L1 network VLAN tag 10 is the internal network, and with an usb nic there is a separate L1 network that is untagged, and connected to a switch, which is connected to the rest of the machines (L2 + L3 managed switch already arrived, free time did not). 

How this project actually is built up, is there is an inventory schema for hosts, networks, disk, which I tried to make to cover most of the possible setups, the docs explain these schemas.

It supports multiple networks, on multiple vlans, and also multiple nics. 
Provision works like the following: as my machines do not have out-of-band management, I had to rely on ipxe, and the network can not have dhcp (it could if it does not answer pxe, not tested). The host needs to be defined, and the network install runs inside a podman container, with a post-install hook to prevent looping reinstalls. A discovery system has been also created, where the host entry needs to be created with only the mac address, and on a reboot, similarly to the pxe install it starts a rocky-10 OS, and runs the discovery (this discovery can be run against a linux host as well). This will create the correct structure for the host, where only minimal changes are needed (like specifying the L1/L3 network for the interfaces). 
The project supports preparing a host for virtualization with libvirt also, and creating/destroying these VMs.

Umkim and yaidap together
Umkim does not ship yaidap, integration is optional. The default site provisioner is noop, which expects an already installed rocky 10 machine. When the site is configured with provisioner yaidap, the hub treats yaidap as the remote executor on the site’s jumphost. For Yaidap backed sites, native ansible YAMLs are stored on the hub, and synced to the jumphost. The available groups are bare_metals, hypervisors (for libvirt VMs), and guests (for the VMs). It is possible to configure multiple hypervisors, and use “least-utilized” as the vm location to place the VM on one of the hypervisors.
Links to projects/docs:
https://umkim.com/
https://docs.umkim.com/
https://github.com/rborso/umkim
https://github.com/rborso/yaidap


