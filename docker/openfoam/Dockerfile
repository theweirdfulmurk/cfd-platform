# Thin profiling layer over the official OpenFOAM v2306 image.
# The base image ships OpenMPI 4.1.2, so libmpiP.so built on the same ABI
# is dropped in via LD_PRELOAD at run time.
#
# Build:  docker build -f docker/openfoam/Dockerfile -t openfoam-mpip .

# -----------------------------------------------------------------------------
# Stage 1 — builder: build libmpiP.so using the OpenMPI shipped in the base.
# -----------------------------------------------------------------------------
FROM opencfd/openfoam-default:2306 AS builder

USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential gfortran \
        binutils-dev libunwind-dev \
        git ca-certificates \
        wget \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tmp
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

COPY --from=builder /opt/mpiP /opt/mpiP

# We do NOT set LD_PRELOAD here. The MPIJob template adds it only when
# profiling is requested — running without LD_PRELOAD gives the unprofiled
# baseline used in the wall-clock metric.
#
# Do not switch USER — base image controls that. If base sets a non-root
# user we inherit it; if it leaves root, we stay root.
