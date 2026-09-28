FROM python:3.12-bookworm@sha256:9bed8554e926c07c6f908841d5ee88c33e8df9236b191526bbce81a9062ab43a
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.source="https://github.com/rborso/umkim"
LABEL org.opencontainers.image.vendor="Robert Borso"

ARG HELM_VERSION=v3.16.3
ARG HELM_SHA256=f5355c79190951eed23c5432a3b920e071f4c00a64f75e077de0dd4cb7b294ea
ARG KUBECTL_VERSION=v1.30.1
ARG KUBECTL_SHA256=5b86f0b06e1a5ba6f8f00e2b01e8ed39407729c4990aeda961f83a586f975e8a
ARG ANSIBLE_CORE_VERSION=2.15.13
ARG NETADDR_VERSION=1.3.0

RUN apt-get update && apt-get install -y --no-install-recommends \
    openssh-client=1:9.2p1-2+deb12u10 \
    sshpass=1.09-1+b1 \
    git=1:2.39.5-0+deb12u3 \
    curl=7.88.1-10+deb12u15 \
    iputils-ping=3:20221126-1+deb12u1 \
    skopeo \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" -o /tmp/kubectl \
    && echo "${KUBECTL_SHA256}  /tmp/kubectl" | sha256sum -c - \
    && install -m 0755 /tmp/kubectl /usr/local/bin/kubectl \
    && rm -f /tmp/kubectl \
    && curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz" -o /tmp/helm.tgz \
    && echo "${HELM_SHA256}  /tmp/helm.tgz" | sha256sum -c - \
    && tar -xzf /tmp/helm.tgz -C /tmp \
    && install -m 0755 /tmp/linux-amd64/helm /usr/local/bin/helm \
    && rm -rf /tmp/helm.tgz /tmp/linux-amd64

WORKDIR /ansible
COPY LICENSE NOTICE /licenses/
COPY requirements.yml /ansible/requirements.yml
RUN pip install --no-cache-dir \
      "ansible-core==${ANSIBLE_CORE_VERSION}" \
      "netaddr==${NETADDR_VERSION}" && \
    ansible-galaxy collection install -r requirements.yml

COPY ansible.cfg /ansible/ansible.cfg
COPY site-validate.yml site-provision.yml site-provision-connector.yml cluster-validate.yml cluster-provision.yml \
  cluster-provision-local.yml cluster-provision-hub.yml cluster-discover-capabilities.yml cluster-delete-hub.yml \
  cluster-configure-storage.yml cluster-configure-kubevirt.yml \
  cluster-gpu-host-prep.yml cluster-gpu-enable.yml cluster-gpu-firewall.yml cluster-validate-storage.yml \
  cluster-add-worker.yml cluster-remove-worker.yml cluster-replace-control.yml cluster-kubeconfig-sync.yml \
  cluster-overlay-recover.yml \
  ingress-auth-reconcile.yml acme-reconcile.yml llm-reconcile.yml wireguard-install.yml /ansible/
COPY cleanup_node_local.sh /ansible/cleanup_node_local.sh
COPY roles /ansible/roles/
COPY charts /ansible/charts/
COPY deploy /ansible/deploy/
COPY inventory /ansible/inventory/
COPY data /ansible/data/

ENV ANSIBLE_CONFIG=/ansible/ansible.cfg
ENTRYPOINT ["ansible-playbook"]
