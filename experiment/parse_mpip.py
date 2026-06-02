#!/usr/bin/env python3
"""Parse an mpiP report and return total MPI time.

mpiP writes a text report at MPI_Finalize with a section titled
"@--- MPI Time (seconds) ---" listing per-task MPI time. We sum the
'MPI' column over all tasks (or use the 'AppTime' / 'MPITime' header
columns where available).

Usage:
    parse_mpip.py path/to/run.mpiP
    parse_mpip.py path/to/*.mpiP --aggregate
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


HEADER_RE = re.compile(r"^@--- MPI Time \(seconds\) ---")
TABLE_HDR_RE = re.compile(r"^Task\s+AppTime\s+MPITime\s+MPI%")


def parse_mpip(path: Path) -> tuple[float, float, float]:
    """Return (total_app_time, total_mpi_time, mpi_pct) over all ranks."""
    text = path.read_text(errors="replace")
    lines = text.splitlines()

    in_section = False
    saw_header = False
    app_total = 0.0
    mpi_total = 0.0
    nranks = 0

    for line in lines:
        if HEADER_RE.search(line):
            in_section = True
            continue
        if not in_section:
            continue
        if TABLE_HDR_RE.search(line):
            saw_header = True
            continue
        if not saw_header:
            continue
        line = line.strip()
        if not line or line.startswith("@") or line.startswith("-"):
            if nranks > 0:
                break
            continue
        # Expected: "<task> <AppTime> <MPITime> <MPI%>"
        parts = line.split()
        if len(parts) < 4:
            continue
        try:
            app = float(parts[1])
            mpi = float(parts[2])
        except ValueError:
            continue
        # '*' aggregate row appears at the end — capture and stop.
        if parts[0] == "*":
            app_total = app
            mpi_total = mpi
            nranks = max(nranks, 1)
            break
        app_total += app
        mpi_total += mpi
        nranks += 1

    if nranks == 0:
        raise ValueError(f"no MPI time section parsed from {path}")
    mpi_pct = 100.0 * mpi_total / app_total if app_total > 0 else 0.0
    return app_total, mpi_total, mpi_pct


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("paths", nargs="+", type=Path)
    ap.add_argument("--aggregate", action="store_true",
                    help="Print one summary line per file (default).")
    args = ap.parse_args()

    print("file\tapp_time\tmpi_time\tmpi_pct")
    for p in args.paths:
        try:
            app, mpi, pct = parse_mpip(p)
        except Exception as e:
            print(f"{p}\tERROR\t{e}", file=sys.stderr)
            continue
        print(f"{p}\t{app:.4f}\t{mpi:.4f}\t{pct:.2f}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
