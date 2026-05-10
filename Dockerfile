# build
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    rm /etc/apt/apt.conf.d/docker-clean; \
    apt-get update && apt-get install -y --no-install-recommends bash

WORKDIR /out
WORKDIR /src

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    --mount=type=bind,source=go.sum,destination=go.sum \
    --mount=type=bind,source=go.mod,destination=go.mod \
    --mount=type=bind,source=./hack,destination=/hack \
    /hack/install-deps.sh dev

ARG TARGETOS
ARG TARGETARCH
ARG SOURCE_DATE_EPOCH=0
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,source=. \
    GOOS=$TARGETOS GOARCH=$TARGETARCH OUTDIR=/out ./hack/binary.sh

# binary
FROM scratch AS binary
COPY --from=build /out/* /

# runtime
FROM debian:trixie-20260421 AS runtime

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    --mount=type=bind,source=./hack,destination=/hack \
    /hack/install-deps.sh runtime

COPY --from=build /out/opendivine /usr/bin/opendivine

CMD ["opendivine"]
