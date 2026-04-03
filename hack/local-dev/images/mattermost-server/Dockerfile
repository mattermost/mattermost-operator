FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy and extract the pre-built tarball
COPY mattermost-server.tar.gz /tmp/
RUN cd /opt && tar -xzf /tmp/mattermost-server.tar.gz && rm /tmp/mattermost-server.tar.gz

# Mattermost expects data directories
RUN mkdir -p /opt/mattermost/data /opt/mattermost/logs /opt/mattermost/config \
    /opt/mattermost/plugins /opt/mattermost/client/plugins

# Non-root user
RUN useradd --system --uid 2000 --gid 0 --home-dir /opt/mattermost mattermost && \
    chown -R mattermost:0 /opt/mattermost

# Operator sets command: ["mattermost"], so the binary must be in PATH.
# Also symlink mmctl so scripts/admin-secret.sh can use it if needed.
RUN ln -s /opt/mattermost/bin/mattermost /usr/local/bin/mattermost && \
    ln -s /opt/mattermost/bin/mmctl /usr/local/bin/mmctl

WORKDIR /opt/mattermost
USER 2000

EXPOSE 8065

ENTRYPOINT ["./bin/mattermost"]
