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
   run_benchmark.py            ─► results/runs.csv
   parse_mpip.py               ─► used internally by run_benchmark
   analyze_results.py          ─► results/summary.csv
                                  results/paired_tests.csv
```

## Install

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Configuration

| Constant | Value | Source |
| --- | --- | --- |
| `SOLVERS`         | openfoam / openradioss / code_aster | `run_benchmark.py` |
| `SCHEDULERS`      | random-scheduler / topology-aware / mueller-merbach | `run_benchmark.py` |
| `REPS_PER_CONFIG` | 6 reps per cell | `run_benchmark.py` |
| `WARMUP_REPS`     | first 1 rep dropped from analysis | `run_benchmark.py`, `analyze_results.py` |
| `NUM_PROCS`       | 16 MPI ranks per job | `run_benchmark.py` |
| `ALPHA`           | 0.05 (95% CI) | `analyze_results.py` |
| `BOOTSTRAP_SEED`  | 42 (deterministic bootstrap CI) | `analyze_results.py` |

> The design uses `REPS_PER_CONFIG=6` with `WARMUP_REPS=1`, so **n=5** samples
> per cell reach the statistics. Tests are one-sided, so both the paired t-test
> and the one-sided Wilcoxon work at n=5 (one-sided Wilcoxon can reach
> p<0.05 since 1/32 = 0.03125).

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

## Notes

* All scripts are idempotent — re-running `run_benchmark.py` appends to the
  existing CSV.
* The MPI time column comes from mpiP reports produced by
  `LD_PRELOAD=/opt/mpiP/lib/libmpiP.so` + `MPIP="-f /results/<id>"`, set in
  the MPIJob launcher command
  (see `backend/internal/infrastructure/k8s/simulation_manager.go`).
