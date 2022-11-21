# Build the mattermost operator
ARG BUILD_IMAGE=golang:1.19
ARG BASE_IMAGE=gcr.io/distroless/static:nonroot

FROM ${BUILD_IMAGE} as builder

WORKDIR /workspace
COPY . .

RUN mkdir -p licenses
COPY LICENSE /workspace/licenses

# Build
RUN make build

FROM ${BASE_IMAGE}

LABEL name="Mattermost Operator" \
  maintainer="dev-ops@mattermost.com" \
  vendor="Mattermost" \
  distribution-scope="public" \
  architecture="x86_64" \
  url="https://mattermost.dev" \
  io.k8s.description="Mattermost Operator creates, configures and helps manage Mattermost installations on Kubernetes" \
  io.k8s.display-name="Mattermost Operator" \
  io.openshift.tags="mattermost,collaboration,operator" \
  summary="Quick and easy Mattermost setup" \
  description="Mattermost operator deploys and configures Mattermost installations, and assists with maintenance/upgrade operations."

WORKDIR /
COPY --from=builder /workspace/licenses .
COPY --from=builder /workspace/build/_output/bin/mattermost-operator .

USER nonroot:nonroot

ENTRYPOINT ["/mattermost-operator"]
