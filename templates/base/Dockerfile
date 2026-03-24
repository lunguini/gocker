FROM debian:bookworm-slim

ARG NERDCTL_VERSION=2.0.4
ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    git \
    iptables \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Install containerd
RUN curl -fsSL "https://github.com/containerd/containerd/releases/download/v2.0.4/containerd-2.0.4-linux-${TARGETARCH}.tar.gz" \
    | tar -xz -C /usr/local

# Install runc
RUN curl -fsSL -o /usr/local/sbin/runc \
    "https://github.com/opencontainers/runc/releases/download/v1.2.5/runc.${TARGETARCH}" \
    && chmod +x /usr/local/sbin/runc

# Install nerdctl
RUN curl -fsSL "https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-${TARGETARCH}.tar.gz" \
    | tar -xz -C /usr/local/bin

# Install CNI plugins (required for container networking)
RUN mkdir -p /opt/cni/bin \
    && curl -fsSL "https://github.com/containernetworking/plugins/releases/download/v1.6.2/cni-plugins-linux-${TARGETARCH}-v1.6.2.tgz" \
    | tar -xz -C /opt/cni/bin

# Install gocker (Linux build) — replaced at build time with current version
COPY gocker /usr/local/bin/gocker
RUN chmod +x /usr/local/bin/gocker

# Containerd config
RUN mkdir -p /etc/containerd \
    && containerd config default > /etc/containerd/config.toml

COPY gocker-init.sh /usr/local/bin/gocker-init.sh
RUN chmod +x /usr/local/bin/gocker-init.sh

ENTRYPOINT ["/usr/local/bin/gocker-init.sh"]
