# Thin profiling layer over the community Code_Aster MPI image.
# Base image (quay.io/tianyikillua/code_aster, Ubuntu 18.04 + libopenmpi 2.1.x)
# is preserved as is to avoid rebuilding code_aster from source; mpiP is built
# against the same MPI inside this layer.
#
# Build:  docker build -f docker/codeaster/Dockerfile -t codeaster-mpip .
#
# NOTE: this image uses OpenMPI 2.1 (Ubuntu 18.04 default), NOT 4.1.2 like
# the OpenFOAM and OpenRadioss images. The libmpiP.so produced here is
# ABI-incompatible with the 4.1.2 builds, so we keep the build per-image.
# If migrating Code_Aster to a 22.04 base later (so OpenMPI 4.1 is available)
# the three images can share a single libmpiP.so artefact.

# -----------------------------------------------------------------------------
# Stage 1 — builder: build libmpiP.so on the same OpenMPI as the base image.
# -----------------------------------------------------------------------------
FROM quay.io/tianyikillua/code_aster:v13 AS builder

USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential gfortran \
        libopenmpi-dev openmpi-bin \
        binutils-dev libunwind-dev \
        git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tmp
RUN git clone --depth 1 https://github.com/LLNL/mpiP.git \
    && cd mpiP \
    && ./configure --prefix=/opt/mpiP --with-cc=mpicc --with-f77=mpif90 \
    && make shared \
    && make install

# -----------------------------------------------------------------------------
# Stage 2 — runtime: same base + libunwind + /opt/mpiP.
# -----------------------------------------------------------------------------
FROM quay.io/tianyikillua/code_aster:v13

USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
        libunwind8 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/mpiP /opt/mpiP
