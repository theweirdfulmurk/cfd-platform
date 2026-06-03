#!/usr/bin/env python3
"""Statistical analysis of bench results from run_benchmark.py.

Per the head-of-department's requirement we report:
    * Shapiro-Wilk normality check on each cell
    * 95% confidence interval — Student-t when normal, bootstrap otherwise
    * Paired tests between the three placement algorithms per solver,
      one-sided (variant faster than baseline) per the directional H1:
        - greedy (topology-aware) vs random  (baseline)
        - mueller-merbach        vs random   ← primary hypothesis H1
        - mueller-merbach        vs greedy
    * Wilcoxon signed-rank as a non-parametric backup

Sampling: run_benchmark.py does 6 reps per cell; the first (cold-start)
rep is dropped here, leaving n=5 for every test.

The scheduler names match what run_benchmark.py writes into the `scheduler`
column: random-scheduler / topology-aware (= greedy) / mueller-merbach.

A monotonic increase of the mm_vs_random gain across
OpenFOAM (sparse) -> OpenRadioss (medium) -> Code_Aster (dense), with
p < 0.05 on the dense solver, confirms the thesis hypothesis.

Output:
    * results/summary.csv — solver, scheduler, n, mean, ci_low, ci_high
    * results/paired_tests.csv — solver, comparison, gain_pct, t_p, wilcoxon_p
"""

from __future__ import annotations

import argparse
import csv
import math
import statistics
import sys
from collections import defaultdict
from pathlib import Path

try:
    import numpy as np
    from scipy import stats
except ImportError:
    sys.stderr.write("error: numpy + scipy required (pip install numpy scipy)\n")
    sys.exit(2)

ALPHA = 0.05
WARMUP_REPS = 1  # drop only the cold-start rep; REPS_PER_CONFIG=6 -> n=5
BOOTSTRAP_SEED = 42  # deterministic bootstrap CIs (EXPERIMENT.md requirement)

# Pairwise comparisons, named for the output CSV. Each is (label, baseline,
# variant); gain_pct > 0 means the variant is faster than the baseline.
# Scheduler strings match run_benchmark.py's SCHEDULERS list.
COMPARISONS = [
    ("greedy_vs_random", "random-scheduler", "topology-aware"),
    ("mm_vs_random", "random-scheduler", "mueller-merbach"),
    ("mm_vs_greedy", "topology-aware", "mueller-merbach"),
]


def load_runs(path: Path) -> list[dict]:
    with path.open() as f:
        reader = csv.DictReader(f)
        rows = []
        for r in reader:
            if r["status"] != "completed":
                continue
            try:
                r["wall_time_s"] = float(r["wall_time_s"])
            except ValueError:
                continue
            r["mpi_time_s"] = _maybe_float(r.get("mpi_time_s"))
            r["mpi_pct"] = _maybe_float(r.get("mpi_pct"))
            r["rep"] = int(r["rep"])
            if r["rep"] <= WARMUP_REPS:
                continue
            rows.append(r)
    return rows


def _maybe_float(x):
    try:
        return float(x)
    except (TypeError, ValueError):
        return None


def group_by_config(rows: list[dict]) -> dict[tuple[str, str], list[dict]]:
    groups: dict[tuple[str, str], list[dict]] = defaultdict(list)
    for r in rows:
        groups[(r["solver"], r["scheduler"])].append(r)
    return groups


def ci_for_cell(samples: list[float]) -> tuple[float, float, float, str]:
    """Return (mean, low, high, method)."""
    n = len(samples)
    mean = statistics.mean(samples)
    if n < 3:
        return mean, mean, mean, "n<3"
    sw_stat, sw_p = stats.shapiro(samples)
    if sw_p >= ALPHA:
        # normal → Student-t
        sd = statistics.stdev(samples)
        margin = stats.t.ppf(1 - ALPHA / 2, df=n - 1) * sd / math.sqrt(n)
        return mean, mean - margin, mean + margin, "t-CI"
    # bootstrap
    boot = stats.bootstrap(
        (np.array(samples),),
        np.mean,
        confidence_level=1 - ALPHA,
        n_resamples=5000,
        method="percentile",
        random_state=BOOTSTRAP_SEED,
    )
    return mean, float(boot.confidence_interval.low), float(boot.confidence_interval.high), "bootstrap-CI"


def paired_tests(baseline: list[float], variant: list[float]) -> dict:
    """Compare variant against baseline, paired by repetition index order.

    gain_pct > 0 means the variant is faster (lower metric) than the baseline.
    """
    n = min(len(baseline), len(variant))
    blank = {"n_pairs": n, "mean_baseline": "", "mean_variant": "",
             "gain_pct": "", "t_stat": "", "t_p": "", "wilcoxon_p": ""}
    if n < 3 or len(baseline) != len(variant):
        return blank

    mean_b = statistics.mean(baseline)
    mean_v = statistics.mean(variant)
    gain = 100.0 * (mean_b - mean_v) / mean_b if mean_b else 0.0
    # One-sided tests: the hypothesis is directional (variant is FASTER, i.e.
    # lower metric, than baseline). One-sided is required for Wilcoxon to be
    # able to reach p<0.05 at n=5 at all (two-sided bottoms out at 0.0625).
    # To revert to two-sided, drop the alternative="less" arguments.
    t_stat, t_p = stats.ttest_rel(variant, baseline, alternative="less")
    try:
        _, w_p = stats.wilcoxon(variant, baseline, alternative="less")
        w_p = f"{w_p:.4g}"
    except ValueError:
        # all-zero differences or sample too small for Wilcoxon
        w_p = ""
    return {
        "n_pairs": n,
        "mean_baseline": f"{mean_b:.3f}",
        "mean_variant": f"{mean_v:.3f}",
        "gain_pct": f"{gain:.2f}",
        "t_stat": f"{t_stat:.4f}",
        "t_p": f"{t_p:.4g}",
        "wilcoxon_p": w_p,
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--runs", type=Path, default=Path("results/runs.csv"))
    ap.add_argument("--out-summary", type=Path, default=Path("results/summary.csv"))
    ap.add_argument("--out-tests", type=Path, default=Path("results/paired_tests.csv"))
    # MPI time is the thesis primary metric (mpiP report); wall time is secondary.
    ap.add_argument("--metric", choices=("mpi_time_s", "wall_time_s"), default="mpi_time_s")
    args = ap.parse_args()

    rows = load_runs(args.runs)
    if not rows:
        print("no rows to analyse — did you point at results/runs.csv?", file=sys.stderr)
        return 1

    groups = group_by_config(rows)

    args.out_summary.parent.mkdir(parents=True, exist_ok=True)
    with args.out_summary.open("w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["solver", "scheduler", "n", "mean", "ci_low", "ci_high", "method"])
        for (solver, sched), runs in sorted(groups.items()):
            samples = [r[args.metric] for r in runs if r.get(args.metric) is not None]
            if len(samples) < 3:
                w.writerow([solver, sched, len(samples), "", "", "", "skip"])
                continue
            mean, lo, hi, method = ci_for_cell(samples)
            w.writerow([solver, sched, len(samples),
                        f"{mean:.3f}", f"{lo:.3f}", f"{hi:.3f}", method])

    with args.out_tests.open("w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["solver", "metric", "comparison", "n_pairs",
                    "mean_baseline", "mean_variant", "gain_pct",
                    "t_stat", "t_p", "wilcoxon_p"])
        solvers = sorted({s for (s, _) in groups})
        for solver in solvers:
            for label, base_sched, var_sched in COMPARISONS:
                base = sorted(groups.get((solver, base_sched), []), key=lambda r: r["rep"])
                var = sorted(groups.get((solver, var_sched), []), key=lambda r: r["rep"])
                base_s = [r[args.metric] for r in base if r.get(args.metric) is not None]
                var_s = [r[args.metric] for r in var if r.get(args.metric) is not None]
                res = paired_tests(base_s, var_s)
                w.writerow([solver, args.metric, label, res["n_pairs"],
                            res["mean_baseline"], res["mean_variant"], res["gain_pct"],
                            res["t_stat"], res["t_p"], res["wilcoxon_p"]])

    print(f"wrote {args.out_summary} and {args.out_tests}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
