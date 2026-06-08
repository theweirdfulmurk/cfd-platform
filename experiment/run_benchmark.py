#!/usr/bin/env python3
"""Orchestrate the 9-config × 6-repetition main benchmark (54 runs, N=16).

For each (solver, scheduler) combination this script:

    1. Submits an MPIJob via the cfd-platform backend HTTP API.
    2. Polls the job to completion.
    3. Collects wall-clock time, mpiP output, and exit code.
    4. Writes one CSV row per run to results/runs.csv.

The first repetition per config is dropped as warm-up, leaving n=5.

Usage:
    # main table: 3 solvers × 3 schedulers × 6 reps = 54 runs at N=16
    run_benchmark.py --backend http://cfd-platform-backend.cfd-platform --cases cases/

    # scaling demo: Yaris Coarse × 3 schedulers × 6 reps = 18 runs at N=32
    run_benchmark.py --backend ... --cases cases/ --np 32 --solvers openradioss

Both write to the SAME results/runs.csv (the np column keeps the two cohorts
apart; analyze_results.py groups by (solver, scheduler, np)).
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

DEFAULT_SOLVERS = ["openfoam", "openradioss", "code_aster"]
# Three placement algorithms, selected per-MPIJob via the
# scheduler.cfd-platform/algorithm label (see scheduler/plugin/plugin.go).
# The backend maps these scheduler names to the label values:
#   random-scheduler   → random
#   topology-aware     → greedy (default fallback)
#   mueller-merbach    → mueller-merbach
SCHEDULERS = ["random-scheduler", "topology-aware", "mueller-merbach"]

# 3 solvers × 3 schedulers × 6 reps = 54 main runs (N=16). The first rep
# per cell is warm-up (dropped in analyze_results.py), leaving n=5 for the
# statistics. Plus N=32 scaling demo on Yaris Coarse (1 × 3 × 6 = 18 runs).
# See EXPERIMENT.md for statistical methodology.
REPS_PER_CONFIG = 6
WARMUP_REPS = 1
DEFAULT_NUM_PROCS = 16  # MPI ranks per job (see EXPERIMENT.md for justification)

# Node label that scripts/cluster-up.sh writes on every worker node (a|b|c).
# We read it to turn the per-rank node placement the extender chose into a
# zone histogram — the thesis's "why it wins" picture.
ZONE_LABEL = "topology.kubernetes.io/zone"

# Cache of node -> zone, filled once on first placement capture. The cluster
# topology is fixed for the whole run, so one kubectl call suffices.
_NODE_ZONE_CACHE: dict[str, str] = {}


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


def node_zone_map(namespace: str) -> dict[str, str]:
    """node name -> zone label, cached. Best-effort: {} if kubectl is absent."""
    if _NODE_ZONE_CACHE:
        return _NODE_ZONE_CACHE
    # jsonpath: dots inside the label key must be backslash-escaped.
    key = ZONE_LABEL.replace(".", "\\.")
    jsonpath = ("{range .items[*]}{.metadata.name}{\"=\"}"
                "{.metadata.labels." + key + "}{\"\\n\"}{end}")
    try:
        out = subprocess.check_output(
            ["kubectl", "get", "nodes", "-o", f"jsonpath={jsonpath}"],
            stderr=subprocess.DEVNULL,
        ).decode()
    except Exception:
        return {}
    for line in out.splitlines():
        name, sep, zone = line.partition("=")
        if name.strip() and sep:
            _NODE_ZONE_CACHE[name.strip()] = zone.strip() or "?"
    return _NODE_ZONE_CACHE


def capture_placement(namespace: str, sim_id: str) -> str:
    """Zone histogram of this job's worker pods, e.g. "a:10|b:4|c:2".

    This is the placement the extender actually chose for the 16/32 ranks —
    the evidence that mueller-merbach packs ranks into fewer zones than
    random. Best-effort: returns "" if kubectl is unavailable or the worker
    pods are not scheduled yet / already cleaned up. Never raises — placement
    is an analysis nicety, not something a run should die on.
    """
    selector = f"mpi-job-id={sim_id},mpi-role=worker"
    jsonpath = "{range .items[*]}{.spec.nodeName}{\"\\n\"}{end}"
    try:
        out = subprocess.check_output(
            ["kubectl", "get", "pods", "-n", namespace,
             "-l", selector, "-o", f"jsonpath={jsonpath}"],
            stderr=subprocess.DEVNULL,
        ).decode()
    except Exception:
        return ""
    nodes = [n.strip() for n in out.splitlines() if n.strip()]
    if not nodes:
        return ""
    zone_of = node_zone_map(namespace)
    hist: dict[str, int] = {}
    for n in nodes:
        z = zone_of.get(n, "?")
        hist[z] = hist.get(z, 0) + 1
    return "|".join(f"{z}:{hist[z]}" for z in sorted(hist))


def poll_until_done(backend: str, namespace: str, sim_id: str,
                    timeout_s: int = 7200) -> tuple[dict, str]:
    """Poll /api/simulations/<id> to a terminal Status. Never raises.

    While polling, opportunistically capture the worker placement (first
    non-empty result wins) — the worker pods exist only between scheduling
    and cleanup, so we must read them mid-run, not after.

    Resilient by design: a single curl/JSON blip is retried (a transient
    network hiccup must not kill tracking of a multi-minute job), but a
    sustained API outage gives up after MAX_CONSEC_FAIL polls instead of
    hanging the full timeout. Returns (payload, zone_histogram) where
    payload["Status"] is completed / failed / timeout / error.
    """
    deadline = time.time() + timeout_s
    placement = ""
    consec_fail = 0
    max_consec_fail = 24  # ~2 min of 5s polls before declaring the API dead
    while time.time() < deadline:
        try:
            raw = subprocess.check_output(
                ["curl", "-fs", f"{backend}/api/simulations/{sim_id}"],
                stderr=subprocess.DEVNULL,
            )
            payload = json.loads(raw)
            consec_fail = 0
        except Exception as e:
            consec_fail += 1
            print(f"  poll blip {consec_fail}x for {sim_id}: {e}", file=sys.stderr)
            if consec_fail >= max_consec_fail:
                print(f"  giving up on {sim_id}: API unreachable", file=sys.stderr)
                return {"Status": "error"}, placement
            payload = {"Status": "pending"}
        if not placement:
            placement = capture_placement(namespace, sim_id)
        if payload.get("Status") in ("completed", "failed"):
            return payload, placement
        time.sleep(5)
    return {"Status": "timeout"}, placement


def fetch_mpip(results_root: Path, sim_id: str) -> Path | None:
    """Locate the mpiP report inside the results PVC mount.

    The MPIJob runs each rank with MPIP="-f /results/<sim_id>" (see
    backend solverCommand), so rank 0 writes <exe>.<ranks>.<pid>.1.mpiP
    into `/results/<sim_id>/`, which the orchestrator mounts at
    results_root.
    """
    folder = results_root / sim_id
    if not folder.exists():
        return None
    matches = list(folder.glob("*.mpiP"))
    if not matches:
        return None
    return matches[0]


def run_one(backend: str, namespace: str, results_root: Path, mpip_parse: Path,
            solver: str, scheduler: str, nproc: int, rep: int,
            case_path: Path, timestamp: str) -> dict:
    """Run one job and ALWAYS return a complete CSV row — never raises.

    Every exit path carries the known coordinates, and the sim_id the moment
    the job is submitted, so a failure is still a traceable, recorded row
    (you can chase the pod/mpiP/logs by sim_id) rather than a black hole.
    """
    row = {
        "sim_id": "", "solver": solver, "scheduler": scheduler, "np": nproc,
        "rep": rep, "status": "error", "wall_time_s": "", "app_time_s": "",
        "mpi_time_s": "", "mpi_pct": "", "zones": "", "timestamp": timestamp,
    }
    name = f"bench-{solver}-{scheduler}-n{nproc}-r{rep:02d}"
    start = time.time()

    try:
        row["sim_id"] = submit_job(backend, name, solver, scheduler, nproc, case_path)
    except Exception as e:
        print(f"  submit failed ({name}): {e}", file=sys.stderr)
        return row  # nothing submitted; status stays "error"

    final, placement = poll_until_done(backend, namespace, row["sim_id"])
    row["wall_time_s"] = f"{time.time() - start:.3f}"
    row["status"] = final.get("Status", "error")
    row["zones"] = placement

    # Metric extraction must never turn a completed run into a lost row: a
    # completed job whose mpiP we could not parse is still recorded (blank
    # metric), chaseable by sim_id, instead of crashing or being mislabelled.
    try:
        report = fetch_mpip(results_root, row["sim_id"])
        if report:
            out = subprocess.check_output(
                [sys.executable, str(mpip_parse), str(report)]).decode()
            # parse_mpip TSV header: file<TAB>app_time<TAB>mpi_time<TAB>mpi_pct
            lines = [r for r in out.strip().splitlines() if r]
            if len(lines) >= 2:
                fields = lines[-1].split("\t")
                if len(fields) >= 4:
                    row["app_time_s"] = fields[1]
                    row["mpi_time_s"] = fields[2]
                    row["mpi_pct"] = fields[3]
    except Exception as e:
        print(f"  mpiP parse failed for {row['sim_id']}: {e}", file=sys.stderr)

    return row


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--backend", required=True,
                    help="cfd-platform backend base URL, e.g. http://cfd-platform-backend")
    ap.add_argument("--cases", required=True, type=Path,
                    help="directory holding <solver>.tar.gz fixtures")
    ap.add_argument("--results-root", type=Path, default=Path("/results"))
    ap.add_argument("--out", type=Path, default=Path("results/runs.csv"))
    ap.add_argument("--np", type=int, default=DEFAULT_NUM_PROCS,
                    help="MPI ranks per job (16 for the main table, 32 for the "
                         "scaling demo)")
    ap.add_argument("--solvers", default=",".join(DEFAULT_SOLVERS),
                    help="comma-separated solver subset (default: all three; "
                         "use 'openradioss' for the N=32 scaling cohort)")
    ap.add_argument("--namespace", default="cfd-platform",
                    help="namespace the MPIJob worker pods run in (for the "
                         "kubectl placement capture)")
    ap.add_argument("--reps", type=int, default=REPS_PER_CONFIG,
                    help=f"reps per cell incl. warm-up (default {REPS_PER_CONFIG} "
                         f"-> n={REPS_PER_CONFIG - WARMUP_REPS}; raise to 8-10 for "
                         "a safety margin, or 1 for a smoke test)")
    args = ap.parse_args()

    if args.reps < 1:
        print("--reps must be >= 1", file=sys.stderr)
        return 2

    solvers = [s.strip() for s in args.solvers.split(",") if s.strip()]
    unknown = [s for s in solvers if s not in DEFAULT_SOLVERS]
    if unknown:
        print(f"unknown solver(s): {unknown}; valid: {DEFAULT_SOLVERS}", file=sys.stderr)
        return 2

    args.out.parent.mkdir(parents=True, exist_ok=True)
    mpip_parse = Path(__file__).parent / "parse_mpip.py"

    fields = ["sim_id", "solver", "scheduler", "np", "rep", "status",
              "wall_time_s", "app_time_s", "mpi_time_s", "mpi_pct",
              "zones", "timestamp"]
    new_file = not args.out.exists()
    with args.out.open("a", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        if new_file:
            w.writeheader()

        for solver in solvers:
            case = args.cases / f"{solver}.tar.gz"
            if not case.exists():
                print(f"skip {solver}: no fixture at {case}", file=sys.stderr)
                continue
            for sched in SCHEDULERS:
                for rep in range(1, args.reps + 1):
                    ts = time.strftime("%Y-%m-%dT%H:%M:%S")
                    tag = f"{solver}/{sched}/np{args.np} rep {rep}"
                    if rep <= WARMUP_REPS:
                        print(f"warmup {tag}", file=sys.stderr)
                    print(f"run {tag}", file=sys.stderr)
                    # run_one is exception-safe and always returns a complete
                    # row; this except is a last-ditch net for a truly
                    # unexpected error (e.g. a bug in row assembly).
                    try:
                        row = run_one(args.backend, args.namespace,
                                       args.results_root, mpip_parse,
                                       solver, sched, args.np, rep, case, ts)
                    except Exception as e:
                        print(f"  unexpected failure: {e}", file=sys.stderr)
                        row = {
                            "sim_id": "", "solver": solver, "scheduler": sched,
                            "np": args.np, "rep": rep, "status": "error",
                            "wall_time_s": "", "app_time_s": "",
                            "mpi_time_s": "", "mpi_pct": "",
                            "zones": "", "timestamp": ts,
                        }
                    # Durability: write + flush the Python buffer + fsync to
                    # disk after EVERY run. A killed VM / lost power then loses
                    # at most the in-flight run, never a completed one. Re-runs
                    # append (the CSV is the resumable source of truth).
                    w.writerow(row)
                    f.flush()
                    os.fsync(f.fileno())
    return 0


if __name__ == "__main__":
    sys.exit(main())
