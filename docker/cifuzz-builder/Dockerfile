FROM ubuntu:20.04 AS build

# Install APT packages with caching set up according to
# https://docs.docker.com/engine/reference/builder/#example-cache-apt-packages
RUN rm -f /etc/apt/apt.conf.d/docker-clean && \
    echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
  --mount=type=cache,target=/var/lib/apt,sharing=locked \
    export DEBIAN_FRONTEND=noninteractive && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    clang-12 \
    make \
    libcap-dev \
    && rm -rf /var/lib/apt/lists/*

ENV PATH="${PATH}:/usr/lib/llvm-12/bin"
ENV CC=/usr/lib/llvm-12/bin/clang
ENV CXX=/usr/lib/llvm-12/bin/clang++

# Install Go
# We cache the go source tarball in /var/cache/go-src.
# We could look into ways to always install the latest Go release if we
# find that we frequently have to update the Go version manually here
# (which should only be necessary when we use new Go features).
RUN --mount=type=cache,target=/var/cache/go-src \
    SHA256=$(sha256sum /var/cache/go-src/go.tar.gz | cut -d ' ' -f 1); \
    if [ "${SHA256:-}" != "b3075ae1ce5dab85f89bc7905d1632de23ca196bd8336afd93fa97434cfa55ae" ]; then \
      curl --fail --silent --show-error --location -o /var/cache/go-src/go.tar.gz https://go.dev/dl/go1.21.1.linux-amd64.tar.gz; \
    fi && \
    tar -C /usr/local -xzf /var/cache/go-src/go.tar.gz

ENV PATH="${PATH}:/usr/local/go/bin"

WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    make installer

FROM scratch AS bin
COPY --from=build /src/build/bin/cifuzz_installer_linux /
