# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    --mount=type=bind,source=hack/install-deps.sh,target=/tmp/install-deps.sh \
    rm /etc/apt/apt.conf.d/docker-clean; \
    /tmp/install-deps.sh dev

WORKDIR /workspace
