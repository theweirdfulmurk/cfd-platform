# Thin profiling layer over the official OpenFOAM v2306 image.
# The base image ships OpenMPI 4.1.2, so libmpiP.so built on the same ABI
# is dropped in via LD_PRELOAD at run time.
#
# Build context = repo root (so pkg/decomp and scheduler/ are visible).
# Build:  docker build -f docker/openfoam/Dockerfile -t openfoam-mpip .

# -----------------------------------------------------------------------------
# Stage 0 — Go builder for extract-openfoam-graph (reads processor*/boundary,
# emits edge-list for our topology-aware scheduler).
# -----------------------------------------------------------------------------
FROM golang:1.21-alpine AS gobuilder
WORKDIR /src
COPY pkg/decomp /src/pkg/decomp
COPY scheduler /src/scheduler
WORKDIR /src/scheduler
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/extract-openfoam-graph \
        ./cmd/extract-openfoam-graph

# -----------------------------------------------------------------------------
# Stage 1 — builder: build libmpiP.so using the OpenMPI shipped in the base.
# -----------------------------------------------------------------------------
FROM opencfd/openfoam-default:2306 AS builder

USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential gfortran \
        binutils-dev libunwind-dev \
        git ca-certificates \
        wget python3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tmp
# LOGNAME is read by mpiP's make-wrappers.py to stamp generated headers;
# Docker doesn't set it by default, leading to KeyError mid-build.
ENV LOGNAME=root
RUN git clone --depth 1 https://github.com/LLNL/mpiP.git \
    && cd mpiP \
    && ./configure --prefix=/opt/mpiP --with-cc=mpicc --with-f77=mpif90 \
    && make shared \
    && make install

# -----------------------------------------------------------------------------
# Stage 2 — runtime: keep the original openfoam image untouched, just add
# /opt/mpiP and libunwind so libmpiP.so resolves at run time.
# -----------------------------------------------------------------------------
FROM opencfd/openfoam-default:2306

USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
        libunwind8 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder   /opt/mpiP /opt/mpiP
COPY --from=gobuilder /out/extract-openfoam-graph /usr/local/bin/extract-openfoam-graph

# We do NOT set LD_PRELOAD here. The MPIJob template adds it only when
# profiling is requested — running without LD_PRELOAD gives the unprofiled
# baseline used in the wall-clock metric.
#
# Do not switch USER — base image controls that. If base sets a non-root
# user we inherit it; if it leaves root, we stay root.
