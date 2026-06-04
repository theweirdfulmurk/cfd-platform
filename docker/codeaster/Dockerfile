# Code_Aster 15.5.2 MPI + mpiP profiler.
#
# Three-stage build:
#   1. prereqs   — compile HDF5, MED, METIS, PARMETIS, TFEL, HOMARD,
#                  SCOTCH-MPI, MUMPS-MPI, PETSc, MISS3D, ASRUN under /aster
#   2. aster     — clone gitlab.com/codeaster/src @ VERSION_ASTER, build
#                  with ./waf_mpi using scif_mpi wafcfg
#   3. runtime   — slim ubuntu:18.04 image with only the runtime libs +
#                  baked-in /aster + libmpiP.so for LD_PRELOAD profiling
#
# Build recipe is adapted from github.com/aethereng/docker-codeaster
# (no explicit licence; treated as GPL-3+ consistent with Code_Aster
# upstream). Their Dockerfile.common.default + Dockerfile.mpi.default
# are merged here into one file, retargeted for our CI matrix.

# =============================================================================
# Stage 1: build all code_aster prerequisites
# =============================================================================
FROM ubuntu:18.04 AS prereqs

ENV DEBIAN_FRONTEND=noninteractive
ENV DEVTOOLS_COMPUTER_ID=aster

ENV LANGUAGE=en_US.UTF-8
ENV LANG=en_US.UTF-8
ENV LC_MESSAGES=POSIX
ENV LC_CTYPE=en_US.UTF-8
ENV LC_ALL=en_US.UTF-8

RUN apt-get update -y && \
    apt-get upgrade -y --no-install-recommends \
        --with-new-pkgs -o Dpkg::Options::="--force-confold" && \
    apt-get install -y \
        apt-utils cmake g++ gcc gfortran locales make mercurial wget \
        autoconf automake libtool m4 \
        zlib1g-dev bison flex libopenblas-dev \
        python3-dev python3-numpy \
        libboost-numpy-dev libboost-python-dev \
        libopenmpi-dev libblacs-mpi-dev libscalapack-openmpi-dev \
        gdb tk xterm gmsh grace git ssh ca-certificates && \
    locale-gen en_GB.UTF-8 en_US.UTF-8 fr_FR.UTF-8 && \
    update-locale LANG=en_US.UTF-8 LC_MESSAGES=POSIX

ENV TEMP_DIRECTORY=/tmp
ENV ASTER_DIRECTORY=/aster
ENV LD_LIBRARY_PATH=$ASTER_DIRECTORY/tfel/lib
ENV PYTHONPATH=$ASTER_DIRECTORY/asrun/lib/python3.6/site-packages:/aster/tfel/lib/python3.6/site-packages:/aster/med/lib/python3.6/site-packages/med

# Prerequisite versions — pinned to the same set aethereng tested.
ENV VERSION_HDF5=1.10.3
ENV VERSION_MED=4.1.0
ENV VERSION_METIS=5.1.0_aster4
ENV VERSION_PARMETIS=4.0.3
ENV VERSION_TFEL=3.4.0
ENV VERSION_HOMARD=11.12_aster2
ENV VERSION_SCOTCH=6.0.4_aster7
ENV VERSION_MUMPS=5.2.1_aster2
ENV VERSION_PETSC=3.12.3_aster
ENV VERSION_MISS3D=6.7_aster6
ENV VERSION_ASRUN=2021.1.0

# -------- HDF5 --------
WORKDIR ${TEMP_DIRECTORY}/hdf5
RUN wget --no-check-certificate \
        https://support.hdfgroup.org/ftp/HDF5/releases/hdf5-1.10/hdf5-${VERSION_HDF5}/src/hdf5-${VERSION_HDF5}.tar.gz \
        -O hdf.tar.gz && \
    tar xf hdf.tar.gz -C . --strip-components 1 && rm hdf.tar.gz && \
    ./configure --prefix=${ASTER_DIRECTORY}/hdf5 && \
    make -j$(nproc) install

# -------- MED --------
# Upstream URL (files.salome-platform.org/Salome/medfile/) now returns 403
# for direct curl/wget — Salome restricted public hot-linking in 2024.
# Fall back to the Debian science-team mirror which packages the same
# 4.1.0 tarball as `med-fichier_4.1.0+repack.orig.tar.gz`. The `+repack`
# suffix is purely Debian metadata stripping of non-free binaries; the
# actual MED sources inside are identical.
WORKDIR ${TEMP_DIRECTORY}/med
RUN wget --no-check-certificate \
        https://deb.debian.org/debian/pool/main/m/med-fichier/med-fichier_4.1.0+repack.orig.tar.gz \
        -O med.tar.gz && \
    tar xf med.tar.gz -C . --strip-components 1 && rm med.tar.gz && \
    # Debian +repack touched configure.ac / Makefile.am so their mtime is
    # newer than the shipped Makefile.in → `make install` tries to re-run
    # `automake-1.15` (not available; Ubuntu 18.04 ships 1.15.1). Force
    # regeneration with the toolchain we DO have so timestamps line up.
    autoreconf -fi && \
    F77=gfortran CXXFLAGS='-std=gnu++98' PYTHON=/usr/bin/python3 \
        ./configure --disable-mesgerr \
                    --with-hdf5=${ASTER_DIRECTORY}/hdf5 \
                    --prefix=${ASTER_DIRECTORY}/med && \
    make -j$(nproc) install && \
    rm -rf ${ASTER_DIRECTORY}/med/share/doc

# -------- METIS / PARMETIS --------
WORKDIR ${TEMP_DIRECTORY}
RUN hg clone -r ${VERSION_METIS}    http://hg.code.sf.net/p/prereq/metis    metis    && \
    hg clone -r ${VERSION_PARMETIS} http://hg.code.sf.net/p/prereq/parmetis parmetis && \
    cd metis    && make -j$(nproc) config prefix=${ASTER_DIRECTORY}/metis    && make -j$(nproc) install && \
    cd ../parmetis && make -j$(nproc) config CFLAGS="-fPIC -DINTSIZE64" prefix=${ASTER_DIRECTORY}/parmetis && make -j$(nproc) install

# -------- TFEL / MFront --------
WORKDIR ${TEMP_DIRECTORY}/tfel
RUN wget --no-check-certificate \
        https://github.com/thelfer/tfel/archive/TFEL-${VERSION_TFEL}.tar.gz \
        -O tfel.tar.gz && \
    tar xf tfel.tar.gz -C . --strip-components 1 && rm tfel.tar.gz && \
    mkdir build && cd build && \
    cmake .. \
        -DCMAKE_BUILD_TYPE=Release \
        -DTFEL_SVN_REVISION=${VERSION_TFEL} -DTFEL_APPEND_VERSION=${VERSION_TFEL} \
        -DCMAKE_Fortran_COMPILER=/usr/bin/gfortran \
        -Dlocal-castem-header=ON -Denable-fortran=OFF \
        -Denable-broken-boost-python-module-visibility-handling=ON \
        -Denable-python-bindings=ON \
        -Denable-cyrano=ON -Denable-aster=ON \
        -Ddisable-reference-doc=ON -Ddisable-website=ON \
        -Denable-portable-build=ON \
        -DPython_ADDITIONAL_VERSIONS=3.6 -Denable-python=ON \
        -DPYTHON_EXECUTABLE:FILEPATH=/usr/bin/python3 \
        -DPYTHON_LIBRARY:FILEPATH=/usr/lib/python3.6/config-3.6m-x86_64-linux-gnu/libpython3.6.so \
        -DPYTHON_INCLUDE_DIR:PATH=/usr/include/python3.6 \
        -DCMAKE_INSTALL_PREFIX=${ASTER_DIRECTORY}/tfel && \
    make -j$(nproc) install && \
    ln -s ${ASTER_DIRECTORY}/tfel/bin/mfront-${VERSION_TFEL} /usr/local/bin/

# -------- HOMARD --------
WORKDIR ${TEMP_DIRECTORY}
RUN hg clone -r ${VERSION_HOMARD} http://hg.code.sf.net/p/prereq/homard homard && \
    cd homard && python3 setup_homard.py --prefix=${ASTER_DIRECTORY}/homard && \
    ln -s ${ASTER_DIRECTORY}/homard/homard /usr/local/bin/

# -------- SCOTCH (seq + MPI) --------
WORKDIR ${TEMP_DIRECTORY}
RUN hg clone -r ${VERSION_SCOTCH} http://hg.code.sf.net/p/prereq/scotch scotch_seq && \
    cp -r scotch_seq scotch_mpi && \
    cd scotch_seq/src && \
        make clean && \
        sed -i -e 's/CFLAGS\s*=/CFLAGS = -Wl,--no-as-needed/g' \
               -e 's/CCD\s*=.*$/CCD = cc/g' Makefile.inc && \
        make scotch esmumps CCD=cc prefix=${ASTER_DIRECTORY}/scotch_seq && \
        mkdir -p ${ASTER_DIRECTORY}/scotch_seq && \
        make -j$(nproc) install prefix=${ASTER_DIRECTORY}/scotch_seq && \
    cd ../../scotch_mpi/src && \
        make clean && \
        sed -i -e 's/CFLAGS\s*=/CFLAGS = -Wl,--no-as-needed/g' \
               -e 's/CCD\s*=.*$/CCD = mpicc/g' Makefile.inc && \
        make scotch esmumps ptscotch ptesmumps CCD=mpicc prefix=${ASTER_DIRECTORY}/scotch_mpi && \
        mkdir -p ${ASTER_DIRECTORY}/scotch_mpi && \
        make -j$(nproc) install prefix=${ASTER_DIRECTORY}/scotch_mpi

# -------- MUMPS (seq + MPI) --------
# `waf build` defaults to nproc parallel jobs; MUMPS 5.2.1_aster2 has
# undeclared module dependencies, so two jobs can race on the same
# <name>.mod0 → <name>.mod rename and one trips with
#   "Fatal Error: Can't rename module file 'idll.mod0' to 'idll.mod'"
# Force serial build with -j1 (slower but reliable).
WORKDIR ${TEMP_DIRECTORY}
RUN hg clone -r ${VERSION_MUMPS} http://hg.code.sf.net/p/prereq/mumps mumps_seq && \
    cp -r mumps_seq mumps_mpi && \
    cd mumps_seq && \
        LIBPATH="${ASTER_DIRECTORY}/scotch_seq/lib ${ASTER_DIRECTORY}/metis/lib ${ASTER_DIRECTORY}/parmetis/lib" \
        INCLUDES="${ASTER_DIRECTORY}/scotch_seq/include ${ASTER_DIRECTORY}/metis/include ${ASTER_DIRECTORY}/parmetis/include" \
        python3 ./waf configure --prefix=${ASTER_DIRECTORY}/mumps_seq && \
        python3 ./waf build -j1 && python3 ./waf install -j1 && \
    cd ../mumps_mpi && \
        LIBPATH="${ASTER_DIRECTORY}/scotch_mpi/lib ${ASTER_DIRECTORY}/metis/lib ${ASTER_DIRECTORY}/parmetis/lib" \
        INCLUDES="${ASTER_DIRECTORY}/scotch_mpi/include ${ASTER_DIRECTORY}/metis/include ${ASTER_DIRECTORY}/parmetis/include" \
        python3 ./waf configure --enable-mpi --enable-openmp --enable-metis --enable-parmetis --enable-scotch \
            --prefix=${ASTER_DIRECTORY}/mumps_mpi --install-tests && \
        python3 ./waf build -j1 && python3 ./waf install -j1

# -------- PETSc (with mumps + ML + hypre + superlu) --------
WORKDIR ${TEMP_DIRECTORY}
RUN hg clone -r ${VERSION_PETSC} http://hg.code.sf.net/p/prereq/petsc petsc && \
    cd petsc/petsc-src && \
        python3 ./configure \
            --with-debugging=0 --with-mpi=1 --with-openmp=0 \
            --with-ssl=0 --with-x=0 \
            --with-mumps-lib="-L${ASTER_DIRECTORY}/mumps_mpi/lib -lzmumps -ldmumps -lmumps_common -lpord -L${ASTER_DIRECTORY}/scotch_mpi/lib -lesmumps -lptscotch -lptscotcherr -lptscotcherrexit -lscotch -lscotcherr -lscotcherrexit -L${ASTER_DIRECTORY}/parmetis/lib -lparmetis -L${ASTER_DIRECTORY}/metis/lib -lmetis" \
            --with-mumps-include=${ASTER_DIRECTORY}/mumps_mpi/include \
            --with-blas-lapack-lib="-lopenblas -llapack -lblacsCinit-openmpi -lblacsF77init-openmpi -lblacs-openmpi" \
            --with-scalapack-lib="-lscalapack-openmpi" \
            --download-ml=${TEMP_DIRECTORY}/petsc/3rd/petsc-pkg-ml-e5040d11aa07.tar.gz \
            --download-hypre=${TEMP_DIRECTORY}/petsc/3rd/hypre-2.14.0.tar.gz \
            --download-superlu=${TEMP_DIRECTORY}/petsc/3rd/superlu-52fc55d0397e382f46bdc4fb77445d0e2f4181ea.tar.gz \
            --PETSC_ARCH=arch-linux2-codeaster \
            --LIBS="-lgomp" \
            --download-mpi4py=yes \
            --with-shared-libraries=1 \
            --prefix=${ASTER_DIRECTORY}/petsc && \
        make -j$(nproc) PETSC_DIR=${TEMP_DIRECTORY}/petsc/petsc-src PETSC_ARCH=arch-linux2-codeaster all && \
        make -j$(nproc) PETSC_DIR=${TEMP_DIRECTORY}/petsc/petsc-src PETSC_ARCH=arch-linux2-codeaster install

# -------- MISS3D --------
WORKDIR ${TEMP_DIRECTORY}
RUN hg clone -r ${VERSION_MISS3D} http://hg.code.sf.net/p/prereq/miss3d miss3d && \
    cd miss3d && make -j$(nproc) install prefix=${ASTER_DIRECTORY}/miss3d

# -------- ASRUN frontend --------
WORKDIR ${TEMP_DIRECTORY}
COPY asrun.external_configuration.py /tmp/asrun_external_configuration.py
RUN hg clone -r ${VERSION_ASRUN} http://hg.code.sf.net/p/prereq/frontend frontend && \
    cd frontend && \
        cp /tmp/asrun_external_configuration.py external_configuration.py && \
        python3 setup.py install --prefix=${ASTER_DIRECTORY}/asrun && \
    sed -i "s/mpi_get_procid_cmd : echo \$PMI_RANK/mpi_get_procid_cmd : echo \$OMPI_COMM_WORLD_RANK/" \
        ${ASTER_DIRECTORY}/asrun/etc/codeaster/asrun && \
    printf "\nvers : stable:${ASTER_DIRECTORY}/aster/share/aster\n" >> \
        ${ASTER_DIRECTORY}/asrun/etc/codeaster/aster && \
    ln -s ${ASTER_DIRECTORY}/asrun/bin/as_run /usr/local/bin/

# Stage wafcfgs in place; aster build stage consumes them.
RUN mkdir -p ${ASTER_DIRECTORY}/aster/lib
COPY dummy.env                      ${ASTER_DIRECTORY}/aster/lib/
COPY aster.wafcfg_scif_std.py       ${ASTER_DIRECTORY}/aster/lib/scif_std.py
COPY aster.wafcfg_scif_mpi.py       ${ASTER_DIRECTORY}/aster/lib/scif_mpi.py
COPY aster.wafcfg_scif_boost.py     ${ASTER_DIRECTORY}/aster/lib/scif_boost.py

RUN rm -rf ${TEMP_DIRECTORY}/*

# =============================================================================
# Stage 2: build code_aster against the prereqs
# =============================================================================
FROM prereqs AS aster_builder

ARG VERSION_ASTER=15.5.2
ENV DEVTOOLS_COMPUTER_ID=aster
ENV TEMP_DIRECTORY=/tmp
ENV ASTER_DIRECTORY=/aster
ENV PREREQ_PATH=$ASTER_DIRECTORY
ENV LINKFLAGS="-Wl,--no-as-needed"

# Re-export prereq paths (Docker doesn't carry ENV between FROM lines).
ENV LIBPATH_HDF5="${PREREQ_PATH}/hdf5/lib"          INCLUDES_HDF5="${PREREQ_PATH}/hdf5/include"
ENV LIBPATH_MED="${PREREQ_PATH}/med/lib"            INCLUDES_MED="${PREREQ_PATH}/med/include"
ENV PYPATH_MED="${PREREQ_PATH}/med/lib/python3.6/site-packages"
ENV LIBPATH_METIS="${PREREQ_PATH}/metis/lib"        INCLUDES_METIS="${PREREQ_PATH}/metis/include"
ENV LIBPATH_TFEL="${PREREQ_PATH}/tfel/lib"          INCLUDES_TFEL="${PREREQ_PATH}/tfel/include"
ENV LIBPATH_SCOTCH="${PREREQ_PATH}/scotch_mpi/lib"  INCLUDES_SCOTCH="${PREREQ_PATH}/scotch_mpi/include"
ENV LIBPATH_MUMPS="${PREREQ_PATH}/mumps_mpi/lib"    INCLUDES_MUMPS="${PREREQ_PATH}/mumps_mpi/include ${PREREQ_PATH}/mumps_mpi/include_seq"
ENV LIBPATH_PARMETIS="${PREREQ_PATH}/parmetis/lib"  INCLUDES_PARMETIS="${PREREQ_PATH}/parmetis/include"
ENV LIBPATH_PETSC="${PREREQ_PATH}/petsc/lib"        INCLUDES_PETSC="${PREREQ_PATH}/petsc/include"
ENV PYPATH_PETSC="${PREREQ_PATH}/petsc/lib"
ENV PYPATH_ASRUN="${PREREQ_PATH}/asrun/lib/python3.6/site-packages"
ENV LD_LIBRARY_PATH="${LIBPATH_HDF5}:${LIBPATH_MED}:${LIBPATH_METIS}:${LIBPATH_TFEL}:${LIBPATH_SCOTCH}:${LIBPATH_MUMPS}:${LIBPATH_PARMETIS}:${LIBPATH_PETSC}:${ASTER_DIRECTORY}/devtools/lib"
ENV PATH="${PREREQ_PATH}/med/bin:${PREREQ_PATH}/tfel/bin/mfront-3.4.0:${PREREQ_PATH}/homard/bin:${PREREQ_PATH}/miss3d:${PREREQ_PATH}/asrun/bin:${PREREQ_PATH}/devtools/bin:${PATH}"
ENV PYTHONPATH="${PYPATH_MED}:${PYPATH_PETSC}:${PYPATH_ASRUN}:${ASTER_DIRECTORY}/tfel/lib/python3.6/site-packages"
ENV LINKFLAGS="${LINKFLAGS} -Wl,-rpath=${LD_LIBRARY_PATH}"
ENV ENABLE_MPI=1

RUN ln -s /usr/lib/x86_64-linux-gnu/ /usr/lib64

WORKDIR ${TEMP_DIRECTORY}

# clone aster source pinned to VERSION_ASTER
RUN git clone --branch ${VERSION_ASTER} https://gitlab.com/codeaster/src.git aster

# clone devtools (development helpers, expected by `./waf_mpi configure`)
ENV VERSION_DEVTOOLS=default
RUN hg clone -r ${VERSION_DEVTOOLS} http://hg.code.sf.net/p/codeaster/devtools devtools

WORKDIR ${TEMP_DIRECTORY}/aster

# Patch out the MPI rank-check that wafcfg can't run inside a build container.
RUN sed -i 's/self.check_mpi_get_rank()/#self.check_mpi_get_rank()/g' data/wscript

ENV CONFIG_PARAMETERS_addmem=4096
ENV CONFIG_PARAMETERS_mpiexec="mpirun -np %(mpi_nbcpu)s --hostfile %(mpi_hostfile)s %(program)s --allow-run-as-root"
ENV CONFIG_PARAMETERS_mpi_get_rank="echo ${OMPI_COMM_WORLD_RANK}"
ENV CATALO_CMD="DUMMY="

# stamp version into pkginfo
COPY add_version.sh aster_pkginfo.pytmpl ./
RUN chmod +x add_version.sh && \
    ./add_version.sh ${VERSION_ASTER} && \
    cp pkginfo.py ${TEMP_DIRECTORY}/aster/code_aster/pkginfo.py

RUN ./waf_mpi distclean && \
    ./waf_mpi configure \
        --use-config=scif_mpi \
        --use-config-dir=${ASTER_DIRECTORY}/aster/lib \
        --prefix=${ASTER_DIRECTORY}/aster \
        --install-tests \
        --with-data=${TEMP_DIRECTORY}/aster/data \
        --without-hg && \
    ./waf_mpi install

# Trim test corpus (~500 MB) — keep only a handful of regression cases.
RUN mv ${ASTER_DIRECTORY}/aster/share/aster/tests ${TEMP_DIRECTORY}/tests && \
    mkdir ${ASTER_DIRECTORY}/aster/share/aster/tests && \
    for name in forma01a hsnv100a pynl01a sdll100a sslv155a ssnv128a zzzz100f; do \
        mv ${TEMP_DIRECTORY}/tests/${name}* ${ASTER_DIRECTORY}/aster/share/aster/tests/ || true; \
    done && \
    rm -rf ${TEMP_DIRECTORY}/*

# =============================================================================
# Stage 3: build mpiP against the same OpenMPI ABI as code_aster
# =============================================================================
FROM aster_builder AS mpip_builder

ENV LOGNAME=root
RUN apt-get update -y && \
    apt-get install -y --no-install-recommends binutils-dev libunwind-dev && \
    git clone --depth 1 https://github.com/LLNL/mpiP.git /tmp/mpiP && \
    cd /tmp/mpiP && \
    ./configure --prefix=/opt/mpiP --with-cc=mpicc --with-f77=mpif90 && \
    make shared && make install && \
    rm -rf /tmp/mpiP

# =============================================================================
# Stage 4: slim runtime image
# =============================================================================
FROM ubuntu:18.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update --fix-missing -y --no-install-recommends \
        -o Dpkg::Options::="--force-confold" && \
    apt-get install -y \
        g++ gcc gfortran locales \
        zlib1g-dev bison flex \
        libopenblas-dev python3-dev python3-numpy \
        libboost-numpy-dev libboost-python-dev \
        libopenmpi-dev libblacs-mpi-dev libscalapack-openmpi-dev \
        libunwind8 \
        gdb tk xterm gmsh grace ssh ca-certificates && \
    apt-get clean && rm -rf /var/lib/apt/lists/* /var/tmp/* /tmp/*

ENV LANGUAGE=en_US.UTF-8 LANG=en_US.UTF-8 LC_MESSAGES=POSIX \
    LC_CTYPE=en_US.UTF-8 LC_ALL=en_US.UTF-8
RUN locale-gen en_US.UTF-8 fr_FR.UTF-8 && update-locale

ENV ASTER_DIRECTORY=/aster
ENV PREREQ_PATH=$ASTER_DIRECTORY
ENV CONFIG_PARAMETERS_addmem=4800
ENV LINKFLAGS="-Wl,--no-as-needed"

ENV LIBPATH_HDF5="${PREREQ_PATH}/hdf5/lib"
ENV LIBPATH_MED="${PREREQ_PATH}/med/lib"
ENV LIBPATH_METIS="${PREREQ_PATH}/metis/lib"
ENV LIBPATH_TFEL="${PREREQ_PATH}/tfel/lib"
ENV LIBPATH_SCOTCH="${PREREQ_PATH}/scotch_mpi/lib"
ENV LIBPATH_MUMPS="${PREREQ_PATH}/mumps_mpi/lib"
ENV LIBPATH_PARMETIS="${PREREQ_PATH}/parmetis/lib"
ENV LIBPATH_PETSC="${PREREQ_PATH}/petsc/lib"
ENV LD_LIBRARY_PATH="${LIBPATH_HDF5}:${LIBPATH_MED}:${LIBPATH_METIS}:${LIBPATH_TFEL}:${LIBPATH_SCOTCH}:${LIBPATH_MUMPS}:${LIBPATH_PARMETIS}:${LIBPATH_PETSC}:${ASTER_DIRECTORY}/devtools/lib:/opt/mpiP/lib"
ENV PATH="${PREREQ_PATH}/med/bin:${PREREQ_PATH}/tfel/bin/mfront-3.4.0:${PREREQ_PATH}/homard/bin:${PREREQ_PATH}/miss3d:${PREREQ_PATH}/asrun/bin:${PREREQ_PATH}/devtools/bin:${PATH}"
ENV PYPATH_MED="${PREREQ_PATH}/med/lib/python3.6/site-packages"
ENV PYPATH_ASRUN="${PREREQ_PATH}/asrun/lib/python3.6/site-packages"
ENV PYPATH_PETSC="${PREREQ_PATH}/petsc/lib"
ENV PYTHONPATH="${PYPATH_MED}:${PYPATH_PETSC}:${PYPATH_ASRUN}:${ASTER_DIRECTORY}/tfel/lib/python3.6/site-packages"
ENV LINKFLAGS="${LINKFLAGS} -Wl,-rpath=${LD_LIBRARY_PATH}"

RUN mkdir /work && useradd -b /work -m -s /bin/bash aster

COPY --from=aster_builder /aster /aster
COPY --from=mpip_builder  /opt/mpiP /opt/mpiP
COPY run_testcases /usr/local/bin/run_testcases
COPY extract_codeaster_graph.py /opt/scripts/extract_codeaster_graph.py
RUN chmod +x /opt/scripts/extract_codeaster_graph.py

RUN ln -s /usr/bin/python3 /usr/local/bin/python && \
    ln -s /aster/tfel/bin/mfront-3.4.0 /usr/local/bin/ && \
    ln -s /aster/asrun/bin/as_run /usr/local/bin/ && \
    chmod +x /usr/local/bin/run_testcases

# SSH for kubeflow mpi-operator (sshd is already installed via the `ssh` package
# in the runtime apt step). Generate host keys; disable client host-key prompt
# and server StrictModes (the operator mounts /root/.ssh world-readable).
RUN ssh-keygen -A \
    && printf 'Host *\n    StrictHostKeyChecking no\n    UserKnownHostsFile /dev/null\n    LogLevel ERROR\n' >> /etc/ssh/ssh_config \
    && sed -i '1i StrictModes no' /etc/ssh/sshd_config

# Stay root: kubeflow mpi-operator runs `/usr/sbin/sshd -De` (needs root) and
# mounts SSH keys to /root/.ssh, so the worker must be root. mpirun already runs
# with --allow-run-as-root (CONFIG_PARAMETERS_mpiexec). The `aster` user remains
# available but we do not switch to it for the MPIJob worker.
WORKDIR /work/aster
ENTRYPOINT ["/bin/bash"]
