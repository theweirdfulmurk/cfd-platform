#!/usr/bin/env python3
"""Orchestrate the 9-config × 15-repetition benchmark.

For each (solver, scheduler) combination this script:

    1. Submits an MPIJob via the cfd-platform backend HTTP API.
    2. Polls the job to completion.
    3. Collects wall-clock time, mpiP output, and exit code.
    4. Writes one CSV row per run to results/runs.csv.

The first 2 repetitions per config are dropped as warm-up.

Usage:
    run_benchmark.py --backend http://cfd-platform-backend.cfd-platform --cases cases/
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import subprocess
import sys
import time
from pathlib import Path

SOLVERS = ["openfoam", "openradioss", "code_aster"]
SCHEDULERS = ["default", "topology-aware"]  # third config: greedy and MM variants
                                            # exposed via scheduler ConfigMap.

REPS_PER_CONFIG = 15
WARMUP_REPS = 2
NUM_PROCS = 16  # MPI ranks per job (see EXPERIMENT.md for justification)


def submit_job(backend: str, name: str, solver: str, scheduler: str,
               np: int, case_path: Path) -> str:
    """POST /api/simulations with multipart form; return simulation ID."""
    cmd = [
        "curl", "-fs", "-X", "POST",
        f"{backend}/api/simulations",
        "-F", f"name={name}",
        "-F", f"type={solver}",
        "-F", f"np={np}",
        "-F", f"scheduler={scheduler}",
        "-F", f"file=@{case_path}",
    ]
    raw = subprocess.check_output(cmd)
    payload = json.loads(raw)
    return payload["ID"]


def poll_until_done(backend: str, sim_id: str, timeout_s: int = 7200) -> dict:
    """Poll /api/simulations/<id> until Status is terminal."""
    deadline = time.time() + timeout_s
    while time.time() < deadline:
        raw = subprocess.check_output([
            "curl", "-fs", f"{backend}/api/simulations/{sim_id}",
        ])
        payload = json.loads(raw)
        if payload["Status"] in ("completed", "failed"):
            return payload
        time.sleep(5)
    raise TimeoutError(f"simulation {sim_id} did not finish in {timeout_s}s")


def fetch_mpip(results_root: Path, sim_id: str) -> Path | None:
    """Locate the mpiP report inside the results PVC mount.

    The launcher pod writes <name>.<ranks>.<pid>.1.mpiP next to the
    launcher's working dir; the backend volume mount exposes it under
    `/results/<sim_id>/`.
    """
    folder = results_root / sim_id
    if not folder.exists():
        return None
    matches = list(folder.glob("*.mpiP"))
    if not matches:
        return None
    return matches[0]


def run_one(backend: str, results_root: Path, mpip_parse: Path,
            solver: str, scheduler: str, rep: int, case_path: Path) -> dict:
    name = f"bench-{solver}-{scheduler}-r{rep:02d}"
    start = time.time()
    sim_id = submit_job(backend, name, solver, scheduler, NUM_PROCS, case_path)
    final = poll_until_done(backend, sim_id)
    wall = time.time() - start

    mpi_time = ""
    mpi_pct = ""
    report = fetch_mpip(results_root, sim_id)
    if report:
        out = subprocess.check_output([sys.executable, str(mpip_parse), str(report)]).decode()
        # output is TSV with a header; second line has the numbers
        rows = [r for r in out.strip().splitlines() if r]
        if len(rows) >= 2:
            fields = rows[-1].split("\t")
            if len(fields) >= 4:
                mpi_time = fields[2]
                mpi_pct = fields[3]

    return {
        "sim_id": sim_id,
        "solver": solver,
        "scheduler": scheduler,
        "rep": rep,
        "status": final["Status"],
        "wall_time_s": f"{wall:.3f}",
        "mpi_time_s": mpi_time,
        "mpi_pct": mpi_pct,
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--backend", required=True,
                    help="cfd-platform backend base URL, e.g. http://cfd-platform-backend")
    ap.add_argument("--cases", required=True, type=Path,
                    help="directory holding <solver>.tar.gz fixtures")
    ap.add_argument("--results-root", type=Path, default=Path("/results"))
    ap.add_argument("--out", type=Path, default=Path("results/runs.csv"))
    args = ap.parse_args()

    args.out.parent.mkdir(parents=True, exist_ok=True)
    mpip_parse = Path(__file__).parent / "parse_mpip.py"

    fields = ["sim_id", "solver", "scheduler", "rep",
              "status", "wall_time_s", "mpi_time_s", "mpi_pct"]
    new_file = not args.out.exists()
    with args.out.open("a", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        if new_file:
            w.writeheader()

        for solver in SOLVERS:
            case = args.cases / f"{solver}.tar.gz"
            if not case.exists():
                print(f"skip {solver}: no fixture at {case}", file=sys.stderr)
                continue
            for sched in SCHEDULERS:
                for rep in range(1, REPS_PER_CONFIG + 1):
                    if rep <= WARMUP_REPS:
                        print(f"warmup {solver}/{sched} rep {rep}", file=sys.stderr)
                    print(f"run {solver}/{sched} rep {rep}", file=sys.stderr)
                    try:
                        row = run_one(args.backend, args.results_root, mpip_parse,
                                       solver, sched, rep, case)
                    except Exception as e:
                        print(f"  failed: {e}", file=sys.stderr)
                        row = {
                            "sim_id": "", "solver": solver, "scheduler": sched,
                            "rep": rep, "status": "error",
                            "wall_time_s": "", "mpi_time_s": "", "mpi_pct": "",
                        }
                    w.writerow(row)
                    f.flush()
    return 0


if __name__ == "__main__":
    sys.exit(main())
