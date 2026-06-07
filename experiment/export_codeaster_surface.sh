#!/bin/bash
# Export a Code_Aster result surface (.vtp + stats.json) for the web viewer.
# Host-side. The .rmed (MED = HDF5) is read directly by meshio; result_to_vtp.py
# skins the solid (SimJEB bracket is tetra) and maps nodal von Mises + |DEPL|.
#
#   export_codeaster_surface.sh <result.rmed> <out_dir>
#
# Copy out/{surface.vtp,stats.json} into the case PVC (/pvc/simulations/<id>/).
#
# REQUIREMENT on the .comm we write for SimJEB: it must produce NODAL fields in
# the MED so colouring is clean —
#   CALC_CHAMP(reuse=RES, RESULTAT=RES, CONTRAINTE='SIGM_NOEU', CRITERES='SIEQ_NOEU')
#   IMPR_RESU(FORMAT='MED', RESU=_F(RESULTAT=RES, NOM_CHAM=('DEPL','SIEQ_NOEU')))
# DEPL is SI metres → scale 1e3 (мм); SIEQ_NOEU VMIS is SI Pa → scale 1e-6 (МПа).
set -euo pipefail
RMED="${1:?usage: export_codeaster_surface.sh <result.rmed> <out_dir>}"
OUT="${2:?usage: export_codeaster_surface.sh <result.rmed> <out_dir>}"
mkdir -p "$OUT"
command -v python3 >/dev/null || { echo "python3 required" >&2; exit 1; }
python3 - <<'PY' || { echo "install deps: pip install meshio numpy h5py" >&2; exit 1; }
import importlib, sys
for m in ("meshio", "numpy"):
    importlib.import_module(m)
PY
python3 "$(dirname "$0")/result_to_vtp.py" --solver code_aster "$RMED" "$OUT"
echo "CODEASTER_EXPORT_DONE -> $OUT/{surface.vtp,stats.json}"
