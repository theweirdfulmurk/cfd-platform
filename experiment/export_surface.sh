#!/bin/bash
# Export a solved OpenFOAM case as ONE render-ready surface (.vtp) + stats.json
# for the web viewer. Post-solve only — never inside the timed mpirun.
#
#   export_surface.sh <case_dir> <out_dir>
#
# The model body is split across many boundary patches (e.g. motorBike has ~67
# motorBike_* patches), so we DON'T pick one foamToVTK patch file — we use a
# `surfaces` functionObject (sampledSurface type=patch, patches="motorBike.*")
# which merges all matched patches into a SINGLE surface carrying the fields.
# vtk.js (XMLPolyDataReader) reads the resulting .vtp; foamVtk writes single-
# quoted XML attributes, which the browser DOMParser handles and our stats grep
# matches with ['"].
set -euo pipefail
CASE="${1:?usage: export_surface.sh <case_dir> <out_dir>}"
OUT="${2:?usage: export_surface.sh <case_dir> <out_dir>}"

# Source the OpenFOAM env with set -e AND set -u OFF: etc/bashrc runs non-zero
# commands (trip -e) and references unset vars like WM_PROJECT_DIR (trip -u).
if [ -z "${WM_PROJECT_DIR:-}" ]; then
  set +eu
  source /usr/lib/openfoam/openfoam2306/etc/bashrc
  set -euo pipefail
fi
mkdir -p "$OUT"
cd "$CASE"

# Merge the parallel result to a serial latest time if not already reconstructed.
if ls -d processor0 >/dev/null 2>&1; then
  if [ -z "$(foamListTimes 2>/dev/null | grep -vx 0 | tail -1)" ]; then
    echo "[export] reconstructPar -latestTime"
    reconstructPar -latestTime > "$OUT/reconstruct.log" 2>&1 || true
  fi
fi

# Replace controlDict with a minimal one whose ONLY function samples the model
# body patches into one merged surface with the fields. (Overwrite, not append,
# so postProcess doesn't also fire the tutorial's forceCoeffs/cuttingPlane etc.)
# PATCHES regex defaults to the OpenFOAM motorBike body; override via env.
PATCHES="${BODY_PATCHES:-\"motorBike.*\"}"
cat > system/controlDict <<CD
FoamFile { version 2.0; format ascii; class dictionary; object controlDict; }
application     simpleFoam;
startFrom       latestTime;
startTime       0;
stopAt          endTime;
endTime         1;
deltaT          1;
writeControl    timeStep;
writeInterval   1;
functions
{
    bodySurface
    {
        type            surfaces;
        libs            (sampling);
        surfaceFormat   vtk;
        formatOptions   { vtk { format ascii; } }
        fields          (p U k);
        interpolate     true;
        surfaces
        {
            body { type patch; patches ( ${PATCHES} ); triangulate false; }
        }
    }
}
CD

echo "[export] postProcess -latestTime -fields (p U k) (sample body surface)"
# -fields is REQUIRED: without it postProcess registers NO volume fields and the
# surfaces functionObject samples geometry only ("Cannot find registered field").
postProcess -latestTime -fields '(p U k)' > "$OUT/postProcess.log" 2>&1

# foamVtk surface writer emits .vtp (XML PolyData) under the function's own dir.
# Target postProcessing/bodySurface specifically — the case may already hold
# stale postProcessing/ output from the solver run (streamlines, cutting planes).
SRC="$(find postProcessing/bodySurface -type f \( -name '*.vtp' -o -name '*.vtk' \) 2>/dev/null | sort | tail -1)"
[ -n "$SRC" ] || { echo "[export] ERROR: no surface produced" >&2; tail -25 "$OUT/postProcess.log" >&2; exit 1; }
cp "$SRC" "$OUT/surface.vtp"

# Surface face count (foamVtk uses single-quoted attrs).
CELLS="$(grep -oE "NumberOfPolys=['\"][0-9]+['\"]" "$OUT/surface.vtp" | head -1 | grep -oE '[0-9]+' | head -1 || true)"
CELLS="${CELLS:-0}"

# Emit only fields actually present in the .vtp. OpenFOAM `p` is KINEMATIC
# pressure (м²/с², scale 1), not Pa.
{
  echo '{'
  echo "  \"surface\": \"motorBike\","
  echo "  \"cells\": ${CELLS},"
  echo '  "fields": ['
  first=1
  while IFS='|' read -r nm lb un; do
    grep -qE "Name=['\"]${nm}['\"]" "$OUT/surface.vtp" || continue
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
