# Project Status

Snapshot as of 2026-06-04.

## Built and verified

| Component | State | Where |
| --- | --- | --- |
| Backend (Go + chi, MPIJob CRD + Job CRD via dynamic/typed K8s client) | compiles, tests green | `backend/` |
| Backend extraction Job orchestration (2-phase: extract → MPIJob) | implemented | `backend/internal/usecase/simulation.go` |
| Frontend (React + TypeScript, 3 solvers + scheduler choice) | compiles, np=16 default | `frontend/` |
| `pkg/decomp` (OpenFOAM / METIS / edge-list parsers) | 16/16 tests | `pkg/decomp/` |
| Scheduler extender + **random / greedy / Müller-Merbach** (3 профиля) | 4/4 algorithm tests, label-routing | `scheduler/` |
| **F-graph extraction binaries** (3 solvers) | implemented, smoke OK | `scheduler/cmd/extract-*` + `scripts/extract_codeaster_graph.py` |
| `docker/openradioss/Dockerfile` (Rocky 9 + OpenMPI 4.1.x + mpiP + extract-radioss-graph + gpmetis) | in GHCR | `docker/openradioss/` |
| `docker/openfoam/Dockerfile` (opencfd/openfoam-default 2306 + mpiP + extract-openfoam-graph) | in GHCR | `docker/openfoam/` |
| `docker/codeaster/Dockerfile` (Ubuntu 18.04 + 12 prereqs + code_aster 15.5.2 + mpiP + extract_codeaster_graph.py) | in GHCR | `docker/codeaster/` |
| `scheduler/Dockerfile` (distroless Go) | in GHCR | `scheduler/` |
| K8s manifests (namespace, RBAC for Jobs+MPIJobs, **scheduler-graphs PVC**, MPIJob example) | written, not deployed | `k8s/` |
| Python eval scripts (parse_mpip / run_benchmark / analyze_results) | written, NUM_PROCS=16, not run on real data | `experiment/` |

GHCR images:

```
ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest
ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest
ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest
ghcr.io/theweirdfulmurk/cfd-platform-scheduler:latest
```

## Smoke tests (local Docker via QEMU on Apple Silicon)

Single-node MPI Allreduce, 4 ranks, 64K doubles, 1000 iterations,
`LD_PRELOAD=/opt/mpiP/lib/libmpiP.so`.

| Image | OpenMPI | mpiP report written | Time per Allreduce | MPI% |
| --- | --- | --- | --- | --- |
| openfoam   | 4.1.x | yes | 92 µs  | 97.23 |
| openradioss| 4.1.x | yes | 243 µs | 99.21 |
| codeaster  | 2.1.1 | yes | 76 µs  | 97.74 |

All three `libmpiP.so` produce reports in the standard format
(`@--- MPI Time ---`, `@--- Callsites ---`, `@--- Aggregate Time ---`)
that `experiment/parse_mpip.py` already consumes.

Numbers are not representative — QEMU emulates x86_64 on Apple Silicon.
Real-cluster numbers will be different and stable.

### Reproduce the smoke

```bash
# pick any of:
IMG=ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest
IMG=ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest
IMG=ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest

docker pull --platform linux/amd64 "$IMG"

docker run --rm --platform linux/amd64 --user root \
  --entrypoint /bin/bash "$IMG" -lc '
set -e
cd /tmp
cat > pingpong.c <<EOF
#include <mpi.h>
#include <stdio.h>
#include <stdlib.h>
int main(int argc, char** argv) {
    MPI_Init(&argc, &argv);
    int rank, size;
    MPI_Comm_rank(MPI_COMM_WORLD, &rank);
    MPI_Comm_size(MPI_COMM_WORLD, &size);
    const int N = 1 << 16;
    double *buf = (double*)malloc(N * sizeof(double));
    double sum = 0.0;
    for (int i = 0; i < N; i++) buf[i] = rank + 0.1 * i;
    for (int it = 0; it < 1000; it++)
        MPI_Allreduce(MPI_IN_PLACE, buf, N, MPI_DOUBLE, MPI_SUM, MPI_COMM_WORLD);
    for (int i = 0; i < N; i++) sum += buf[i];
    if (rank == 0) printf("done; checksum=%.3e\n", sum);
    free(buf);
    MPI_Finalize();
    return 0;
}
EOF
mpicc -O2 -o pingpong pingpong.c
LD_PRELOAD=/opt/mpiP/lib/libmpiP.so \
  mpirun --allow-run-as-root --mca plm_rsh_agent /bin/true \
         -np 4 ./pingpong
find / -name "*.mpiP" 2>/dev/null | head -1 | xargs -r cat | head -40
'
```

## Outstanding before defence

| Item | State | ETA |
| --- | --- | --- |
| **Vast.ai instance m:42009** (EPYC 9654, 192 phys cores, 515 GB RAM, $1.103/hr) | waiting on Mark to RENT | minutes |
| Setup script `scripts/cluster-up.sh` (Docker + kind + tc qdisc + MPI Operator + scheduler + backend) | not written | ~1 day after VM live |
| End-to-end smoke through backend → MPIJob on Vast.ai kind cluster | not started | 1 day after setup |
| **Main benchmark — 54 jobs** (3 solvers × 3 schedulers × 6 reps, N=16) | not started | ~27 hours of compute |
| **Scaling demo — 18 jobs** (Yaris × 3 schedulers × 6 reps, N=32) | not started | ~12 hours of compute |
| `analyze_results.py` over real data → Table 4.1 + scaling chart | not started | 1-2 days after numbers land |
| Thesis text — chapters 1-5 (drafts из md уже есть) | partially mapped из md | ~2 weeks |
| Defence slides | not started | last week |

## Known caveats

* All four images are built linux/amd64 only — Apple Silicon Mac users
  must add `--platform linux/amd64`. Multi-arch would double CI time.
* The codeaster image uses OpenMPI 2.1 (Ubuntu 18.04 base) so its
  `libmpiP.so` is ABI-incompatible with the OpenMPI 4.1 used by
  openfoam / openradioss. This is fine for the experiment — each
  MPIJob runs inside one solver image and never mixes ABIs.
* `make bootstrap` from `gitlab.com/codeaster/src` does **not** work
  publicly (EDF-only network). We build code_aster from sources using
  the aethereng/docker-codeaster recipe (see `README.md` Credits).
* MED 4.1.0 tarball at salome-platform.org now returns HTTP 403 for
  curl/wget — we mirror via Debian's `med-fichier_4.1.0+repack`.
* OpenRadioss 2025.10 `Starter` sed patch turns on
  `IDB_METIS=1` (writes `input.graphL` for METIS partitioning).
* The codeaster build alone takes ~90 min cold on a GH ubuntu-latest
  runner. Subsequent rebuilds are ~5 min from GHA cache as long as
  `docker/codeaster/Dockerfile` is unchanged.

## Repo layout (terse)

```
backend/        Go REST API, creates MPIJobs via dynamic K8s client
frontend/       React UI
pkg/decomp/     F(i,j) parsers + 16 tests
scheduler/      Greedy + Müller-Merbach + extender + Dockerfile
docker/         3 solver images, each with mpiP baked in
scripts/        extract_codeaster_graph.py (runs inside codeaster image)
k8s/            Cluster manifests + example MPIJob
experiment/     Python: parse_mpip, run_benchmark, analyze_results
.github/        build-images matrix workflow → GHCR
```
