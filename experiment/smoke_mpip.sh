#!/usr/bin/env bash
# Dry-run the metric chain BEFORE committing to the 54+18 benchmark.
#
# The whole thesis primary metric (MPI time) hangs on one untested link: that
# mpiP actually emits a report inside the running cluster and parse_mpip.py can
# read it. This submits ONE real openfoam job per algorithm (3 jobs, 1 rep each)
# through the exact run_benchmark.py path the full run will use, then asserts the
# resulting CSV rows carry a non-empty mpi_time_s (mpiP works) and zones (kubectl
# placement capture works).
#
# It is a smoke test, not a measurement run — but it DOES submit real jobs, so
# run it only when the cluster is up and you intend to validate.
#
# Usage:
#   smoke_mpip.sh <backend-url> <cases-dir> [namespace]
# Example:
#   smoke_mpip.sh http://cfd-platform-backend.cfd-platform cases/ cfd-platform
set -euo pipefail

BACKEND="${1:?usage: smoke_mpip.sh <backend-url> <cases-dir> [namespace]}"
CASES="${2:?usage: smoke_mpip.sh <backend-url> <cases-dir> [namespace]}"
NS="${3:-cfd-platform}"
HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="$(mktemp -d)/smoke.csv"

echo "[smoke] submitting 1 rep × openfoam × 3 algorithms via run_benchmark.py"
python3 "${HERE}/run_benchmark.py" \
  --backend "${BACKEND}" \
  --cases "${CASES}" \
  --namespace "${NS}" \
  --solvers openfoam \
  --reps 1 \
  --out "${OUT}"

echo "[smoke] --- ${OUT} ---"
column -s, -t "${OUT}" || cat "${OUT}"

# Assert: at least one completed row with a non-empty mpi_time_s.
fail=0
if ! awk -F, 'NR>1 && $6=="completed" && $9!="" {ok=1} END{exit !ok}' "${OUT}"; then
  echo "[smoke] FAIL: no completed run produced an mpi_time_s — mpiP report not found/parsed." >&2
  echo "        Check: LD_PRELOAD=/opt/mpiP/lib/libmpiP.so present in image; MPIP=-f /results/<id>;" >&2
  echo "        and that run_benchmark --results-root points where rank 0 writes *.mpiP." >&2
  fail=1
else
  echo "[smoke] OK: mpiP MPI time captured."
fi

# Warn (not fail) if placement is empty — analysis nicety, not a blocker.
if awk -F, 'NR>1 && $6=="completed" && $11!="" {ok=1} END{exit !ok}' "${OUT}"; then
  echo "[smoke] OK: zone placement captured (kubectl reachable)."
else
  echo "[smoke] WARN: zones column empty — kubectl placement capture not working" >&2
  echo "        (runs still valid; you just lose the placement picture)." >&2
fi

exit "${fail}"
