# cfd-platform — Topology-Aware MPI Scheduler

Web-based platform for running parallel engineering simulations
(OpenFOAM, OpenRadioss, Code_Aster) on Kubernetes, with a custom
topology-aware scheduler that minimises MPI communication latency.

This is the implementation for Mark Egorov's BMSTU diploma
(supervisor: A. A. Kuzmina), defended June 2026.

## Architecture

```
                    ┌──────────────┐
              user →│   frontend   │     React + TypeScript
                    └──────┬───────┘
                           │ HTTP
                    ┌──────▼───────┐
                    │   backend    │     Go + chi
                    │ (REST API)   │     creates MPIJob CRDs
                    └──────┬───────┘
                           │ kubectl apply
                    ┌──────▼───────┐
                    │  MPI Operator│     kubeflow/mpi-operator
                    │   (CRD)      │     resolves MPIJob → Pods
                    └──────┬───────┘
                           │ pods created
                    ┌──────▼───────┐    ┌─────────────────────────┐
                    │kube-scheduler│←→  │topology-aware-scheduler │
                    │              │    │ (extender, /prioritize) │
                    └──────┬───────┘    └─────────────────────────┘
                           │                         │
                           │                         │ uses F(i,j) from
                           │                         │ pkg/decomp parsers
                           │                         │
                    ┌──────▼─────────────────────────▼──────┐
                    │     OpenFOAM / OpenRadioss /          │
                    │     Code_Aster pods                   │
                    │  + libmpiP.so (LD_PRELOAD profiling)  │
                    └───────────────────────────────────────┘
```

## Layout

| Path | What |
| --- | --- |
| `backend/`     | Go REST API, creates MPIJobs via `dynamic` client |
| `frontend/`    | React UI for submitting and watching simulations |
| `pkg/decomp/`  | Communication graph parsers (OpenFOAM / METIS / edge-list); 16 passing tests |
| `scheduler/`   | Topology-aware scheduler extender (Go HTTP service) |
| `docker/`      | Solver Docker images with mpiP baked in |
| `scripts/`     | Preprocessors: extract F(i,j) from solver output |
| `k8s/`         | Cluster manifests + example MPIJob |
| `experiment/`  | Python eval scripts (Shapiro/CI/t-test/bootstrap) |
| `.github/workflows/` | Multi-stage matrix build → GHCR |

## Three solvers, three communication-graph shapes

The thesis hypothesis is that the Müller-Merbach offline placement
outperforms the streaming greedy as the F(i,j) graph density grows:

| Solver | Method | Graph density | Hypothesis |
| --- | --- | --- | --- |
| OpenFOAM   | FVM, iterative GAMG          | sparse (geom. neighbours)        | MM ≈ greedy |
| OpenRadioss| explicit FEM dynamics        | sparse (boundary nodes only)     | MM slightly better |
| Code_Aster | implicit FEM, direct MUMPS   | dense (LU fill-in)               | MM clearly better |

## Building the images

GitHub Actions builds all four images on push to `main` and on demand
via `workflow_dispatch`. Images land at:

```
ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest
ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest
ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest
ghcr.io/theweirdfulmurk/cfd-platform-scheduler:latest
```

Local build:

```bash
docker build -f docker/openradioss/Dockerfile -t openradioss .
docker build -f docker/openfoam/Dockerfile    -t openfoam-mpip .
docker build -f docker/codeaster/Dockerfile   -t codeaster-mpip .
docker build -f scheduler/Dockerfile          -t topology-aware-scheduler .
```

## Deploying to a cluster

The cluster must be ≥ 9 nodes spread over ≥ 3 availability zones for
the experiment to demonstrate meaningful latency differences. See the
thesis Chapter 4 for the cluster topology.

```bash
# 1. MPI Operator (one-off install).
kubectl apply -f https://raw.githubusercontent.com/kubeflow/mpi-operator/v0.4.0/deploy/v2beta1/mpi-operator.yaml

# 2. cfd-platform namespace + resources.
kubectl apply -f k8s/

# 3. Patch kube-scheduler ConfigMap with our extender entry.
#    (cluster-dependent — see comments in k8s/50-scheduler-config.yaml)
```

## Extracting F(i,j) per solver

| Solver | Workflow |
| --- | --- |
| OpenFOAM | `decomp.OpenFOAM("/pvc/simulations/<id>")` — reads `processor*/constant/polyMesh/boundary` |
| OpenRadioss | sed-patched Starter writes `input.graph<L>` (METIS format) → `gpmetis input.graph0 N` → `decomp.METIS()` |
| Code_Aster | `scripts/extract_codeaster_graph.py` runs `medpartitioner --create-boundary-faces`, reads `joints` via MEDLoader → edge-list → `decomp.EdgeList()` |

Full upstream-source references (which file in OpenRadioss/medcoupling)
are in `pkg/decomp/openradioss.go` and `pkg/decomp/codeaster.go`.

## Running the experiment

```bash
cd experiment/
python3 run_benchmark.py    # orchestrates 3 solvers × 3 schedulers × 6 reps = 54 runs (+18 N=32 scaling)
python3 analyze_results.py  # Shapiro-Wilk, CI, paired t-test, bootstrap
```

Output goes to `experiment/results/` (gitignored). Final summary table
becomes Table 4.1 of the thesis.

## Development

```bash
cd backend && go test ./...     # backend unit tests
cd pkg/decomp && go test ./...  # parser tests (16/16)
cd scheduler && go test ./...   # algorithm tests
```

## Credits

* The Code_Aster MPI Docker recipe under `docker/codeaster/` is adapted
  from [aethereng/docker-codeaster](https://github.com/aethereng/docker-codeaster)
  (no explicit licence; treated as GPL-3+ consistent with Code_Aster
  upstream). The `Dockerfile.common.default` + `Dockerfile.mpi.default`
  are merged into a single multi-stage `Dockerfile`; sidecar files
  (`aster.wafcfg_scif_*.py`, `asrun.external_configuration.py`,
  `dummy.env`, `add_version.sh`, `aster_pkginfo.pytmpl`, `run_testcases`)
  are vendored verbatim.
* Code_Aster itself is © EDF, released under GPL-3.
