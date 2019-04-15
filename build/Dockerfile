# Build the mattermost operator
ARG BUILD_IMAGE=golang:1.12.1
ARG BASE_IMAGE=alpine:3.9

FROM ${BUILD_IMAGE} AS build
WORKDIR /go/src/github.com/mattermost/mattermost-operator/
COPY . /go/src/github.com/mattermost/mattermost-operator/
RUN cp build/operator-sdk /usr/local/bin/
RUN make generate
RUN make build

# Final Image
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

ENV OPERATOR=/usr/local/bin/mattermost-operator \
    USER_UID=1001 \
    USER_NAME=mattermost-operator

RUN mkdir -p /licenses
COPY --from=build /go/src/github.com/mattermost/mattermost-operator/LICENSE /licenses/
COPY --from=build /go/src/github.com/mattermost/mattermost-operator/build/_output/bin/mattermost-operator /usr/local/bin/mattermost-operator
COPY --from=build /go/src/github.com/mattermost/mattermost-operator/build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
