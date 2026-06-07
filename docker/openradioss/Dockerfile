# Full OpenRadioss build with pinned OpenMPI 4.1.2 + mpiP for profiling.
#
# Based on the community gist by edp8489 (Rocky 9 + OpenMPI from source),
# extended to also produce libmpiP.so on the same MPI ABI so that
# LD_PRELOAD profiling works without rebuilding the solver.
#
# Build:  docker build -f docker/openradioss/Dockerfile -t openradioss .
# Run:    docker run --rm openradioss starter_linux64_gf -input some.rad
# Profile: docker run --rm -e LD_PRELOAD=/opt/mpiP/lib/libmpiP.so openradioss ...

ARG OMPI_VERSION=4.1.2
ARG OPENRADIOSS_REF=main

# -----------------------------------------------------------------------------
# Stage 0 — Go builder for extract-radioss-graph (consumes the METIS graph
# file written by the IDB_METIS=1 patched Starter + the gpmetis partition
# file, emits edge-list for the topology-aware scheduler).
# -----------------------------------------------------------------------------
FROM golang:1.21-alpine AS gobuilder
WORKDIR /src
COPY pkg/decomp /src/pkg/decomp
COPY scheduler /src/scheduler
WORKDIR /src/scheduler
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/extract-radioss-graph \
        ./cmd/extract-radioss-graph

# -----------------------------------------------------------------------------
# Stage 1 — builder: compile everything from source.
# -----------------------------------------------------------------------------
FROM rockylinux:9 AS builder

# EL9 / Rocky 9 split a number of -devel packages out of the base repo:
#   - binutils-devel, libstdc++-static -> CRB (CodeReady Builder)
#   - libunwind-devel                  -> EPEL
# Both repos must be enabled before installing the toolchain.
RUN dnf install -y dnf-plugins-core epel-release \
    && dnf config-manager --set-enabled crb \
    && dnf install -y \
        gcc gcc-c++ gcc-gfortran \
        make cmake \
        git git-lfs \
        perl wget tar bzip2 which \
        binutils-devel libunwind-devel \
        python3 \
    && dnf clean all

# --- OpenMPI (pinned to match OpenFOAM v2306 image ABI) ----------------------
ARG OMPI_VERSION
RUN wget -q https://download.open-mpi.org/release/open-mpi/v4.1/openmpi-${OMPI_VERSION}.tar.bz2 \
    && tar xjf openmpi-${OMPI_VERSION}.tar.bz2 \
    && cd openmpi-${OMPI_VERSION} \
    && ./configure --prefix=/opt/openmpi --enable-mpirun-prefix-by-default \
    && make -j"$(nproc)" install \
    && cd .. && rm -rf openmpi-${OMPI_VERSION}*

ENV PATH=/opt/openmpi/bin:$PATH
ENV LD_LIBRARY_PATH=/opt/openmpi/lib:$LD_LIBRARY_PATH

# --- OpenRadioss --------------------------------------------------------------
ARG OPENRADIOSS_REF
RUN git clone --depth 1 --branch "${OPENRADIOSS_REF}" \
        https://github.com/OpenRadioss/OpenRadioss.git /opt/OpenRadioss

# Patch: activate the built-in METIS graph dumper in the Starter.
#
# grid2mat.F has a debug-only branch guarded by IDB_METIS=0; flipping it to 1
# makes the Starter write `input.graph<level>` in standard METIS adjacency
# format (header `nelem nedges 010 ncon`) on every domain-decomposition pass.
# This is the file our scheduler reads via decomp.METIS() to build F(i,j).
RUN GRID2MAT=/opt/OpenRadioss/starter/source/spmd/domain_decomposition/grid2mat.F \
    && before=$(grep -cE '^[[:space:]]+IDB_METIS = 0[[:space:]]*$' "$GRID2MAT" || true) \
    && test "$before" = "1" || (echo "patch precondition failed: expected exactly 1 'IDB_METIS = 0' line, got $before" && exit 1) \
    && sed -i 's/^\([[:space:]]*\)IDB_METIS = 0[[:space:]]*$/\1IDB_METIS = 1/' "$GRID2MAT" \
    && after=$(grep -cE '^[[:space:]]+IDB_METIS = 0[[:space:]]*$' "$GRID2MAT" || true) \
    && test "$after" = "0" || (echo "patch postcondition failed: $after 'IDB_METIS = 0' lines remain" && exit 1) \
    && grep -nE '^[[:space:]]+IDB_METIS = 1[[:space:]]*$' "$GRID2MAT"

WORKDIR /opt/OpenRadioss/starter
RUN ./build_script.sh -arch=linux64_gf -nt="$(nproc)"

WORKDIR /opt/OpenRadioss/engine
RUN ./build_script.sh -arch=linux64_gf -mpi=ompi -mpi-root=/opt/openmpi -nt="$(nproc)"

# --- mpiP profiler (shared library, ABI-matched to our OpenMPI) ---------------
# libunwind is installed above, so we let mpiP use it for proper stack traces.
WORKDIR /tmp
# LOGNAME is read by mpiP's make-wrappers.py to stamp generated headers;
# Docker doesn't set it by default, leading to KeyError mid-build.
ENV LOGNAME=root
RUN git clone --depth 1 https://github.com/LLNL/mpiP.git \
    && cd mpiP \
    && ./configure --prefix=/opt/mpiP --with-cc=mpicc --with-f77=mpif90 \
    && make shared \
    && make install \
    && cd .. && rm -rf mpiP

# -----------------------------------------------------------------------------
# Stage 2 — runtime: copy only artefacts the solver actually needs at run time.
# -----------------------------------------------------------------------------
FROM rockylinux:9

RUN dnf install -y epel-release && dnf install -y \
        libgfortran libstdc++ \
        binutils libunwind \
        perl which procps-ng \
        metis \
        libxcrypt-compat \
        openssh-server openssh-clients \
    && dnf clean all
# libxcrypt-compat provides libcrypt.so.1 — the OpenRadioss starter/engine link
# against it, but rockylinux9 ships only libcrypt.so.2; without it the binaries
# fail at load ("libcrypt.so.1: cannot open shared object file").
# SSH for kubeflow mpi-operator: worker pods run `/usr/sbin/sshd -De` and the
# launcher ssh-es in. Generate host keys; disable client host-key prompt and
# server StrictModes (the operator mounts /root/.ssh world-readable). Without
# this the worker StartErrors and the launcher fails host-key verification.
RUN ssh-keygen -A \
    && printf 'Host *\n    StrictHostKeyChecking no\n    UserKnownHostsFile /dev/null\n    LogLevel ERROR\n' >> /etc/ssh/ssh_config \
    && sed -i '1i StrictModes no' /etc/ssh/sshd_config
# `metis` package provides gpmetis(1) — the Starter writes input.graph<L>
# (raw adjacency) and the entry-point script runs `gpmetis input.graph0 N`
# to get the .part.N file that decomp.METIS() expects.

COPY --from=builder   /opt/openmpi /opt/openmpi
COPY --from=builder   /opt/mpiP    /opt/mpiP
COPY --from=gobuilder /out/extract-radioss-graph /usr/local/bin/extract-radioss-graph

# Copy only the runtime parts of OpenRadioss — exec binaries, config files,
# and the extlib shared libraries the engine loads at run time. Sources, .git,
# CMake build dirs are dropped (they would add ~3 GB to the image).
COPY --from=builder /opt/OpenRadioss/exec           /opt/OpenRadioss/exec
COPY --from=builder /opt/OpenRadioss/hm_cfg_files   /opt/OpenRadioss/hm_cfg_files
COPY --from=builder /opt/OpenRadioss/extlib         /opt/OpenRadioss/extlib

ENV PATH=/opt/openmpi/bin:/opt/OpenRadioss/exec:$PATH
ENV LD_LIBRARY_PATH=/opt/openmpi/lib:/opt/OpenRadioss/extlib/hm_reader/linux64:$LD_LIBRARY_PATH
ENV RAD_CFG_PATH=/opt/OpenRadioss/hm_cfg_files
ENV OMP_STACKSIZE=400m

# mpiP is activated by setting LD_PRELOAD=/opt/mpiP/lib/libmpiP.so in the pod
# manifest — not here, so that unprofiled wall-clock baseline runs are clean.

WORKDIR /work
ENTRYPOINT ["/bin/bash"]
