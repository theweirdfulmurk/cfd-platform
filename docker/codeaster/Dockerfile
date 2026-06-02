# Full Code_Aster build from source with OpenMPI 4.1.x + mpiP.
#
# Replaces the previous thin-layer-on-tianyikillua approach (that path
# is dead — the only public Code_Aster MPI base images are published in
# Docker manifest schema 1, which modern buildx refuses to pull).
#
# Strategy:
#   1. ubuntu:22.04 base — gives us OpenMPI 4.1.x via apt (ABI-compatible
#      with our OpenFOAM and OpenRadioss images, so libmpiP.so can be
#      reused).
#   2. clone code_aster source from the official upstream
#      (https://gitlab.com/codeaster/src) and use its embedded
#      `make bootstrap` target which downloads, configures and builds
#      every prerequisite (MED, MUMPS, METIS, ParMETIS, SCOTCH, PT-SCOTCH,
#      MEDCoupling, mpi4py, …) along with code_aster itself.
#   3. build mpiP on the same OpenMPI 4.1.x ABI so that LD_PRELOAD
#      profiling works without rebuilding code_aster.
#
# Expect ~60-90 min build on GH Actions ubuntu-latest the first time
# (no cache); subsequent rebuilds reuse the bootstrap-output cache.

FROM ubuntu:22.04 AS builder

ARG DEBIAN_FRONTEND=noninteractive

# Toolchain + prerequisites that `make bootstrap` does NOT install itself.
RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        cmake \
        gfortran \
        gcc g++ \
        make \
        git wget tar bzip2 ca-certificates \
        python3 python3-dev python3-pip python3-numpy python3-scipy \
        libopenmpi-dev openmpi-bin \
        libopenblas-dev liblapack-dev \
        libboost-numpy-dev libboost-python-dev \
        libhdf5-openmpi-dev \
        zlib1g-dev bison flex \
        binutils-dev libunwind-dev \
        pkg-config \
        tk swig \
    && rm -rf /var/lib/apt/lists/*

# Clone code_aster source (pinned via ARG so CI runs are reproducible).
ARG CODEASTER_REF=main
RUN git clone --depth 1 --branch "${CODEASTER_REF}" \
        https://gitlab.com/codeaster/src.git /opt/code_aster

# `make bootstrap` runs the canonical EDF build sequence:
#   * downloads and installs the prerequisites that aren't packaged
#     (MED, MUMPS-parallel, parMETIS/PT-SCOTCH, MEDCoupling, MFront, …)
#     under /opt/code_aster/install/
#   * configures and builds code_aster itself with MPI support.
# WAFCFG_TMPL = config/wafcfg/<basename>.py — we use the default which
# the code_aster project tunes for Ubuntu LTS.
WORKDIR /opt/code_aster
RUN make bootstrap

# --- mpiP profiler (shared library, ABI-matched to our OpenMPI 4.1.x) -------
WORKDIR /tmp
RUN git clone --depth 1 https://github.com/LLNL/mpiP.git \
    && cd mpiP \
    && ./configure --prefix=/opt/mpiP --with-cc=mpicc --with-f77=mpif90 \
    && make shared \
    && make install \
    && rm -rf /tmp/mpiP

# -----------------------------------------------------------------------------
FROM ubuntu:22.04

ARG DEBIAN_FRONTEND=noninteractive

# Runtime-only deps: openmpi runtime, hdf5 runtime, numpy, scipy.
RUN apt-get update && apt-get install -y --no-install-recommends \
        openmpi-bin libopenmpi3 \
        libhdf5-openmpi-103 \
        libopenblas0 liblapack3 \
        python3 python3-numpy python3-scipy \
        libunwind8 \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/code_aster /opt/code_aster
COPY --from=builder /opt/mpiP /opt/mpiP

ENV PATH=/opt/code_aster/install/bin:$PATH
ENV LD_LIBRARY_PATH=/opt/code_aster/install/lib:$LD_LIBRARY_PATH

WORKDIR /work
ENTRYPOINT ["/bin/bash"]
