#!/bin/bash
# One-off: produce surface.vtp + stats.json for the motorBike showcase from the
# CACHED mesh (/stage/motorBike_meshed) — NO re-stage, NO snappy. The smoke run
# 4b0ce57f wrote no field (writeInterval 100 > endTime 50), so we run a short
# SERIAL simpleFoam on the ready mesh to write one real field, then export the
# body surface. Artifacts land in /stage/export for `kubectl cp` into the case
# PVC (/pvc/simulations/<id>/).
#
# Run inside the openfoam image with the cached mesh + the two scripts mounted:
#   docker run --rm --entrypoint /bin/bash \
#     -v /root/stage:/stage \
#     -v /root/export_surface.sh:/export_surface.sh:ro \
#     -v /root/retrofit_motorbike_viz.sh:/retrofit_motorbike_viz.sh:ro \
#     ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest \
#     -c "bash /retrofit_motorbike_viz.sh"
# Source the OpenFOAM env BEFORE set -e: etc/bashrc returns non-zero internally
# and would abort the script mid-source under an active set -e.
source /usr/lib/openfoam/openfoam2306/etc/bashrc
set -euo pipefail
ITERS="${ITERS:-400}"

cd /tmp && rm -rf mb && cp -r /stage/motorBike_meshed mb && cd mb

# Write exactly one snapshot at the end: endTime = writeInterval = ITERS.
foamDictionary -entry endTime      -set "$ITERS"   system/controlDict
foamDictionary -entry writeControl -set timeStep   system/controlDict
foamDictionary -entry writeInterval -set "$ITERS"  system/controlDict

echo "[retrofit] serial simpleFoam, $ITERS iters (writes the field the smoke never did)"
if ! simpleFoam > /stage/retrofit_solve.log 2>&1; then
  echo "[retrofit] SOLVER FAILED"; tail -25 /stage/retrofit_solve.log; exit 1
fi
echo "[retrofit] solve done; latest time = $(foamListTimes -latestTime 2>/dev/null | tail -1)"

bash /export_surface.sh /tmp/mb /stage/export
echo "RETROFIT_DONE -> /stage/export/{surface.vtp,stats.json}"
