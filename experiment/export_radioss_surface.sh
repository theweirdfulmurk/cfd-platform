#!/bin/bash
# Export an OpenRadioss result surface (.vtp + stats.json) for the web viewer.
# Host-side. Runs anim_to_vtk INSIDE the radioss image (the binary lives in
# /opt/OpenRadioss/exec) to turn the latest animation state into legacy VTK,
# then result_to_vtp.py (host meshio) extracts the shell surface → surface.vtp.
#
#   export_radioss_surface.sh <case_dir> <out_dir>
#
# <case_dir> holds the run's animation files (<run>A001, A002, ...). Newest =
# most deformed. Copy the produced out/{surface.vtp,stats.json} into the case
# PVC (/pvc/simulations/<id>/) so the backend serves them.
#
# NOTE: OpenRadioss stress UNITS are model-defined (LS-DYNA mm-ms-kg→ГПа,
# mm-s-tonne→МПа). After reading the Yaris model's unit system, pass it through:
#   STRESS_UNIT=ГПа STRESS_SCALE=1   (or МПа / 1)
set -euo pipefail
CASE="${1:?usage: export_radioss_surface.sh <case_dir> <out_dir>}"
OUT="${2:?usage: export_radioss_surface.sh <case_dir> <out_dir>}"
IMG="${RADIOSS_IMAGE:-ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest}"
STRESS_UNIT="${STRESS_UNIT:-МПа}"
STRESS_SCALE="${STRESS_SCALE:-1}"
mkdir -p "$OUT"

# Pick the newest animation state (most deformed). OpenRadioss names states
# <run>A001..A999 then A1000, A1001, ... (NOT zero-padded past 999), so a plain
# lexicographic sort ranks A999 above A1000. Match 3+ trailing digits and sort
# NUMERICALLY by that suffix (GNU sed \t = tab; runs on the Linux VM host).
LAST="$(ls "$CASE"/*A[0-9][0-9][0-9]* 2>/dev/null \
  | sed -E 's/.*A([0-9]+)$/\1\t&/' \
  | sort -n -k1,1 | tail -1 | cut -f2- || true)"
[ -n "$LAST" ] || { echo "no animation files (*A###) in $CASE" >&2; exit 1; }
ANIM="$(basename "$LAST")"
echo "[radioss] anim_to_vtk on $ANIM"

# Resolve the converter binary inside the image (exact name varies by build).
# Pass the filename as a positional arg ($1) so quoting survives the container
# boundary — splicing it into the -c body would word-split on spaces / glob on
# metacharacters in the run name.
docker run --rm -v "$CASE":/case:ro -v "$OUT":/out --entrypoint /bin/bash "$IMG" -c '
  set -e
  bin="$(command -v anim_to_vtk 2>/dev/null || ls /opt/OpenRadioss/exec/anim_to_vtk* 2>/dev/null | head -1)"
  [ -n "$bin" ] || { echo "anim_to_vtk not found in image" >&2; exit 1; }
  "$bin" "/case/$1" > /out/anim.vtk
' _ "$ANIM"
echo "[radioss] anim.vtk $(wc -l < "$OUT/anim.vtk") lines; converting to surface.vtp"
python3 "$(dirname "$0")/result_to_vtp.py" --solver openradioss \
    --stress-unit "$STRESS_UNIT" --stress-scale "$STRESS_SCALE" \
    "$OUT/anim.vtk" "$OUT"
echo "RADIOSS_EXPORT_DONE -> $OUT/{surface.vtp,stats.json}"
