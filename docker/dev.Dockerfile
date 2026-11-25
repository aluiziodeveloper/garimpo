# Development container: the Go toolchain at a fixed version, running as your user.
FROM golang:1.27.4

ARG UID=1000
ARG GID=1000

# A user with the same UID/GID as the host, so files created in /src belong to you.
# The cache directories are created here so the named volumes inherit their owner.
RUN groupadd --gid "${GID}" dev \
 && useradd --uid "${UID}" --gid "${GID}" --create-home dev \
 && mkdir -p /go/pkg/mod /home/dev/.cache/go-build \
 && chown -R dev:dev /go /home/dev/.cache

USER dev
WORKDIR /src
