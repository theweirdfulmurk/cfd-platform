# NOTE 2026-06-02 — currently UNBUILDABLE.
#
# The only publicly available Code_Aster MPI base image
# (quay.io/tianyikillua/code_aster, tags v13/latest/master) is published
# with Docker manifest schema 1 (vnd.docker.distribution.manifest.v1+prettyjws),
# which buildx 0.12+ refuses to pull as of 2025. There is no maintained
# upstream alternative that ships MPI-enabled code_aster as a schema-2
# image at the time of writing (codeastersolver/codeaster-mpi was last
# updated in 2019 and uses the same schema 1 format).
#
# Two ways forward (decide after OpenFOAM/OpenRadioss are green):
#   (a) follow the OpenRadioss pattern — clone the code_aster source,
#       compile it from scratch on top of debian:bookworm with
#       OpenMPI 4.1.x and MUMPS, then bake mpiP in. ~1 day of work.
#   (b) wait for upstream to republish with schema 2 (issue tracked at
#       https://github.com/tianyikillua/code_aster_on_docker).
#
# The Dockerfile below is the OLD thin-layer approach kept for reference.
# It will not build in CI; the codeaster job is disabled in the GH Actions
# matrix in .github/workflows/build-images.yml.

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
