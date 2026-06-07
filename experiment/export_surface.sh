#!/bin/bash
# Export a solved OpenFOAM case as a single result surface (.vtp) + stats.json
# for the platform's web viewer (frontend reads surface.vtp via vtk.js and
# /field-stats for labels). Post-solve only — NEVER call this inside the timed
# mpirun, it must not pollute the mpiP benchmark timing.
#
#   export_surface.sh <case_dir> <out_dir>
#
# Produces <out_dir>/surface.vtp (the body patch, real p/U/k) and
# <out_dir>/stats.json. Idempotent. Works both inside a fresh openfoam
# container (sources the bashrc) and inside the launcher (env already set).
set -euo pipefail
CASE="${1:?usage: export_surface.sh <case_dir> <out_dir>}"
OUT="${2:?usage: export_surface.sh <case_dir> <out_dir>}"

[ -z "${WM_PROJECT_DIR:-}" ] && source /usr/lib/openfoam/openfoam2306/etc/bashrc
mkdir -p "$OUT"
cd "$CASE"

# If the run was parallel and never reconstructed, merge the latest time so
# foamToVTK has a serial field to read.
if ls -d processor0 >/dev/null 2>&1; then
  if [ -z "$(foamListTimes 2>/dev/null | grep -vx 0 | tail -1)" ]; then
    echo "[export] reconstructPar -latestTime"
    reconstructPar -latestTime > "$OUT/reconstruct.log" 2>&1 || true
  fi
fi

rm -rf VTK
echo "[export] foamToVTK -latestTime (p U k)"
foamToVTK -latestTime -ascii -fields '(p U k)' > "$OUT/foamToVTK.log" 2>&1

# The standard motorBike tutorial puts the whole body in one 'motorBike' wall
# patch -> one .vtp. Fall back to the largest boundary patch if named otherwise.
SRC="$(ls VTK/*/boundary/motorBike.vtp 2>/dev/null | head -1 || true)"
if [ -z "$SRC" ]; then
  SRC="$(ls -S VTK/*/boundary/*.vtp 2>/dev/null | head -1 || true)"
fi
[ -n "$SRC" ] || { echo "[export] ERROR: no boundary .vtp produced" >&2; exit 1; }
cp "$SRC" "$OUT/surface.vtp"

# Surface face count from the .vtp header (best effort, for the CSV).
CELLS="$(grep -o 'NumberOfPolys="[0-9]*"' "$OUT/surface.vtp" | head -1 | grep -o '[0-9]*' | head -1 || true)"
CELLS="${CELLS:-0}"

# Field descriptors. name = array name in the .vtp; the viewer derives the
# numeric range/mean live from the .vtp so the legend always matches the render.
# OpenFOAM incompressible `p` is KINEMATIC pressure (p/ρ) → unit м²/с², scale 1
# (NOT Pa). U/k are already in SI display units.
# Emit ONLY fields that actually landed in surface.vtp — foamToVTK silently omits
# fields absent from the latest time (e.g. no k in a laminar case), and a
# stats.json name that doesn't resolve to a real .vtp array makes the viewer show
# a bogus 0..1 legend on a flat-coloured surface.
{
  echo '{'
  echo "  \"surface\": \"$(basename "$SRC" .vtp)\","
  echo "  \"cells\": ${CELLS},"
  echo '  "fields": ['
  first=1
  while IFS='|' read -r nm lb un; do
    grep -q "Name=\"${nm}\"" "$OUT/surface.vtp" || continue
    [ "$first" -eq 0 ] && printf ',\n'
    printf '    { "name": "%s", "label": "%s", "unit": "%s", "scale": 1 }' "$nm" "$lb" "$un"
    first=0
  done <<'SPECS'
p|Давление|м²/с²
U|Скорость|м/с
k|Турбулентная энергия|м²/с²
SPECS
  printf '\n  ]\n}\n'
} > "$OUT/stats.json"

echo "[export] DONE surface=$SRC cells=${CELLS}"
ls -la "$OUT"
