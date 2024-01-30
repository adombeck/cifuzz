# This image contains the dependencies to run C/C++ fuzz tests (the only
# dependency is LLVM). It is built and used by the integration tests
# and can also be used for local development and testing.

FROM ubuntu:rolling

# Install APT packages with caching set up according to
# https://docs.docker.com/engine/reference/builder/#example-cache-apt-packages
RUN rm -f /etc/apt/apt.conf.d/docker-clean && \
    echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
  --mount=type=cache,target=/var/lib/apt,sharing=locked \
    export DEBIAN_FRONTEND=noninteractive && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    llvm \
    && rm -rf /var/lib/apt/lists/*
