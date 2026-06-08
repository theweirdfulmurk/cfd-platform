# Experiment harness

Produces the tables in thesis Chapter 5 by running the **Scenario B**
benchmark on the Vast.ai EPYC 9654 cluster (see [../EXPERIMENT.md](../EXPERIMENT.md)):

* **Main**: 3 solvers × 3 schedulers × 6 reps = **54 runs**, N=16 ranks.
* **Scaling**: Yaris Coarse × 3 schedulers × 6 reps = **18 runs**, N=32 ranks.

The three schedulers all run under the `topology-aware-scheduler` profile
and differ only by the placement algorithm the extender applies, selected
per-MPIJob via the `scheduler.cfd-platform/algorithm` pod label:

| `scheduler` form value | algorithm label | role |
| --- | --- | --- |
| `random-scheduler` | `random` | baseline (no topology awareness) |
| `topology-aware`   | `greedy` | streaming greedy |
| `mueller-merbach`  | `mueller-merbach` | offline QAP heuristic (ours) |

## Pipeline

```
   smoke_mpip.sh               ─► validate the metric chain (run ONCE first)
   run_benchmark.py            ─► results/runs.csv
   parse_mpip.py               ─► used internally by run_benchmark
   analyze_results.py          ─► results/summary.csv
                                  results/paired_tests.csv
```

### 0. Smoke-test the metric chain first

The primary metric (MPI time) depends on mpiP emitting a report inside the
cluster and `parse_mpip.py` reading it — one link never tested on a real
cluster. Validate it with three real jobs before the full run:

```bash
./smoke_mpip.sh http://cfd-platform-backend.cfd-platform cases/ cfd-platform
```

PASS means `mpi_time_s` is populated (mpiP works) and `zones` is populated
(kubectl placement capture works). Do not start the 72 runs until this is green.

### 1. Main table + scaling cohort

```bash
# 54 main runs at N=16 (all three solvers)
python3 run_benchmark.py --backend http://cfd-platform-backend.cfd-platform --cases cases/
# 18 scaling runs at N=32 (Yaris only) — appends to the same runs.csv
python3 run_benchmark.py --backend ... --cases cases/ --np 32 --solvers openradioss
```

Both cohorts share `results/runs.csv`; the `np` column keeps them apart and
`analyze_results.py` groups by `(solver, scheduler, np)` so they are never pooled.

## Install

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Configuration

| Setting | Default | Override |
| --- | --- | --- |
| solver subset     | openfoam / openradioss / code_aster | `--solvers openradioss` |
| schedulers        | random-scheduler / topology-aware / mueller-merbach | (constant) |
| reps per cell     | 6 (incl. 1 warm-up → n=5) | `--reps 8` |
| ranks per job     | 16 | `--np 32` |
| namespace         | cfd-platform | `--namespace ...` |
| `ALPHA`           | 0.05 (95% CI) | `analyze_results.py` |
| `BOOTSTRAP_SEED`  | 42 (deterministic bootstrap CI) | `analyze_results.py` |

> Default `--reps 6` with `WARMUP_REPS=1` gives **n=5** per cell. At n=5 the
> one-sided Wilcoxon can *only* reach p<0.05 by hitting its floor 1/32 =
> 0.03125, and only if **all 5 pairs** move in the hypothesised direction — one
> reversed pair pushes it over 0.05. The paired t-test is more powerful but its
> Shapiro-Wilk normality check is near-blind at n=5. **If you want a safety
> margin on the dense (Code_Aster) cell, raise `--reps 8` (n=7).**

### runs.csv columns

`sim_id, solver, scheduler, np, rep, status, wall_time_s, app_time_s,
mpi_time_s, mpi_pct, zones, timestamp`

* `np` — MPI ranks; separates the N=16 and N=32 cohorts.
* `app_time_s` — total app time from mpiP (for compute-vs-comm breakdown).
* `zones` — zone histogram of the worker placement the extender chose, e.g.
  `a:13|b:3|c:0`. This is the *evidence* that mueller-merbach packs ranks into
  fewer zones than random — the thesis's "why it wins" picture. Captured via
  `kubectl` during the run (best-effort; empty if kubectl is unreachable).
* `timestamp` — wall-clock start of each run (ISO, for tracing).

## Statistical approach

Per supervisor requirement:

1. **Normality**: Shapiro-Wilk on each (solver, scheduler) cell.
2. **CI**:
   * normal → Student-t CI
   * non-normal → percentile bootstrap (5000 resamples, seed 42)
3. **Paired comparison** between algorithms (per solver), primary metric
   MPI time (`mpi_time_s` from the mpiP report):
   * `greedy_vs_random`, `mm_vs_random` (← hypothesis H1), `mm_vs_greedy`
   * paired t-test + Wilcoxon signed-rank backup, with `gain_pct`.
   * Tests are **one-sided** (directional: variant is faster / lower MPI time).

A monotonic increase of the `mm_vs_random` gain going
OpenFOAM → OpenRadioss → Code_Aster, with `p < 0.05` on Code_Aster,
confirms the thesis hypothesis.

## Durability & resuming

The 54+18 run is long ($50, ~hours) so the harness is built to never lose a
completed run:

* **Every run is a recorded row.** `run_one` never raises — submit failure,
  poll timeout, job `failed`, or an mpiP parse crash all still write a row with
  the run's coordinates and (once submitted) its `sim_id`, so any failure is
  traceable, not a black hole. Statuses you may see: `completed`, `failed`,
  `timeout`, `error`.
* **Flushed + fsync'd after every row**, so a killed VM / lost power loses at
  most the single in-flight run.
* **Resumable / idempotent.** Re-running `run_benchmark.py` *appends* to the
  existing `runs.csv` (header written once). `analyze_results.py` only counts
  `status==completed` rows with a non-empty metric, so partial/failed rows are
  carried for debugging but never pollute the statistics.
* **Capture stderr** for the failure reasons (the CSV has *what* failed, stderr
  has *why*):
  ```bash
  python3 run_benchmark.py --backend ... --cases cases/ 2>&1 | tee run.log
  ```

## Notes

* All scripts are idempotent — re-running `run_benchmark.py` appends to the
  existing CSV.
* The MPI time column comes from mpiP reports produced by
  `LD_PRELOAD=/opt/mpiP/lib/libmpiP.so` + `MPIP="-f /results/<id>"`, set in
  the MPIJob launcher command
  (see `backend/internal/infrastructure/k8s/simulation_manager.go`).
