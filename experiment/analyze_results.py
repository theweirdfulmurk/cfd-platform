#!/usr/bin/env python3
"""Statistical analysis of bench results from run_benchmark.py.

Per the head-of-department's requirement we report:
    * Shapiro-Wilk normality check on each cell
    * 95% confidence interval — Student-t when normal, bootstrap otherwise
    * Paired t-test between (default, topology-aware) per solver
    * Wilcoxon signed-rank as a non-parametric backup

Output:
    * results/summary.csv — solver, scheduler, n, mean, ci_low, ci_high
    * results/paired_tests.csv — solver, t_stat, p_value, wilcoxon_p
    * results/figure.png — wall_time and mpi_time bar chart (optional)
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
WARMUP_REPS = 2


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
    )
    return mean, float(boot.confidence_interval.low), float(boot.confidence_interval.high), "bootstrap-CI"


def paired_tests(default: list[float], topo: list[float]) -> dict:
    """Compare topo vs default. Pairs assumed by repetition index order."""
    if len(default) != len(topo) or len(default) < 3:
        return {"t_stat": "", "t_p": "", "wilcoxon_p": "", "n_pairs": len(default)}
    t_stat, t_p = stats.ttest_rel(topo, default)
    w_stat, w_p = stats.wilcoxon(topo, default)
    return {
        "t_stat": f"{t_stat:.4f}",
        "t_p": f"{t_p:.4g}",
        "wilcoxon_p": f"{w_p:.4g}",
        "n_pairs": len(default),
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--runs", type=Path, default=Path("results/runs.csv"))
    ap.add_argument("--out-summary", type=Path, default=Path("results/summary.csv"))
    ap.add_argument("--out-tests", type=Path, default=Path("results/paired_tests.csv"))
    ap.add_argument("--metric", choices=("wall_time_s", "mpi_time_s"), default="wall_time_s")
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
        w.writerow(["solver", "metric", "n_pairs", "t_stat", "t_p", "wilcoxon_p"])
        solvers = sorted({s for (s, _) in groups})
        for solver in solvers:
            default = sorted(groups.get((solver, "default"), []), key=lambda r: r["rep"])
            topo = sorted(groups.get((solver, "topology-aware"), []), key=lambda r: r["rep"])
            def_s = [r[args.metric] for r in default if r.get(args.metric) is not None]
            topo_s = [r[args.metric] for r in topo if r.get(args.metric) is not None]
            r = paired_tests(def_s, topo_s)
            w.writerow([solver, args.metric, r["n_pairs"],
                        r["t_stat"], r["t_p"], r["wilcoxon_p"]])

    print(f"wrote {args.out_summary} and {args.out_tests}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
