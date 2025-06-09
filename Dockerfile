FROM golang:1.23 AS build

# Set group-operator version
# Defaults to v2
ARG VERSION=v2
ARG RELEASE_VERSION

ADD . /go/src/github.com/coreweave/group-operator
WORKDIR /go/src/github.com/coreweave/group-operator
ENV CGO_ENABLED=0
RUN make RELEASE_VERSION=${RELEASE_VERSION} group-operator.$VERSION
RUN ln -s group-operator.${VERSION} _output/cmd/bin/group-operator

FROM gcr.io/distroless/base-debian12:latest

ENV CONTROLLER_VERSION=$VERSION
COPY --from=build /go/src/github.com/coreweave/group-operator/_output/cmd/bin/* /opt/
COPY third_party/library/license.txt /opt/license.txt

ENTRYPOINT ["/opt/group-operator"]
CMD ["--help"]
