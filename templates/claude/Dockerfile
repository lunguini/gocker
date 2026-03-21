FROM python:3-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    curl \
    git \
    ca-certificates \
    ripgrep \
    fd-find \
    openssh-client \
    jq \
    less \
    && rm -rf /var/lib/apt/lists/* \
    && ln -s $(which fdfind) /usr/local/bin/fd

# Create non-root user (Claude Code refuses bypass permissions as root)
RUN useradd -m -s /bin/bash -u 1000 sandbox \
    && mkdir -p /workspace \
    && chown sandbox:sandbox /workspace

# Install Claude Code to ~/.local/bin (where Claude Code expects its native binary)
RUN mkdir -p /home/sandbox/.local/bin \
    && GCS_BUCKET="https://storage.googleapis.com/claude-code-dist-86c565f3-f756-42ad-8dfa-d59b1c096819/claude-code-releases" \
    && ARCH=$(uname -m | sed 's/aarch64/arm64/' | sed 's/x86_64/x64/') \
    && VERSION=$(curl -fsSL "$GCS_BUCKET/latest") \
    && curl -fsSL -o /home/sandbox/.local/bin/claude "$GCS_BUCKET/$VERSION/linux-$ARCH/claude" \
    && chmod +x /home/sandbox/.local/bin/claude \
    && chown -R sandbox:sandbox /home/sandbox/.local

RUN curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Bake sandbox-required Claude settings into the image
RUN mkdir -p /home/sandbox/.claude && \
    echo '{"permissions":{"defaultMode":"bypassPermissions"},"skipDangerousModePermissionPrompt":true}' \
    > /home/sandbox/.claude/settings.json && \
    chown -R sandbox:sandbox /home/sandbox/.claude

USER sandbox
ENV HOME=/home/sandbox
ENV PATH="/home/sandbox/.local/bin:${PATH}"
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["claude", "--dangerously-skip-permissions"]
