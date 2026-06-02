# Experiment harness

Reproduces the table in thesis Chapter 4 by running 135 simulations
(3 solvers × 3 schedulers × 15 reps) on a 9-node, 3-AZ cluster.

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

| Constant | Meaning | Source |
| --- | --- | --- |
| `SOLVERS`            | three solvers tested | `run_benchmark.py` |
| `SCHEDULERS`         | default vs topology-aware | `run_benchmark.py` |
| `REPS_PER_CONFIG`    | 15 reps per cell | `run_benchmark.py` |
| `WARMUP_REPS`        | first 2 reps dropped | `run_benchmark.py`, `analyze_results.py` |
| `NUM_PROCS`          | 8 MPI ranks per job | `run_benchmark.py` |
| `ALPHA`              | 0.05 (95% CI) | `analyze_results.py` |

## Statistical approach

Per supervisor requirement:

1. **Normality**: Shapiro-Wilk on each (solver, scheduler) cell.
2. **CI**:
   * normal → Student-t CI
   * non-normal → percentile bootstrap (5000 resamples)
3. **Paired comparison** between schedulers (per solver):
   * paired t-test
   * Wilcoxon signed-rank as a robust backup

A monotonic increase of the topology-aware improvement when going from
OpenFOAM → OpenRadioss → Code_Aster, with `p < 0.05` on Code_Aster,
confirms the thesis hypothesis.

## Notes

* All scripts are idempotent — re-running appends to existing CSVs.
* The MPI time column is captured from mpiP reports produced by
  `LD_PRELOAD=/opt/mpiP/lib/libmpiP.so` set in the MPIJob template
  (see backend/internal/infrastructure/k8s/simulation_manager.go).
