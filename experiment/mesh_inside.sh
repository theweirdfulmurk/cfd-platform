#!/bin/bash
# Runs INSIDE the openfoam container. Serial-mesh motorBike, set 16-way scotch
# decomposition, export reconstructed case to the mounted /stage volume.
# Source BEFORE set -e: the OpenFOAM bashrc uses `return 1` as normal control
# flow (optional ThirdParty libs absent), which would trip set -e and abort.
source /usr/lib/openfoam/openfoam2306/etc/bashrc || true
set -e
cd /tmp && rm -rf mb && cp -r "$FOAM_TUTORIALS"/incompressible/simpleFoam/motorBike mb && cd mb
mkdir -p constant/triSurface
cp -f "$FOAM_TUTORIALS"/resources/geometry/motorBike.obj.gz constant/triSurface/
echo "[stage] surfaceFeatureExtract"; surfaceFeatureExtract > log.sfe 2>&1
echo "[stage] blockMesh";            blockMesh            > log.blockMesh 2>&1
echo "[stage] snappyHexMesh (serial, several min)"; snappyHexMesh -overwrite > log.snappy 2>&1
echo "[stage] mesh built; configuring case"
# restore0Dir semantics: snappyHexMesh writes its own 0/ (cellLevel, pointLevel,
# ...); wipe it and restore the clean initial fields from 0.orig, else the real
# p/U/k fields never reach the processor dirs and simpleFoam -parallel aborts.
rm -rf 0 && cp -r 0.orig 0
cat > system/decomposeParDict <<'EOF'
FoamFile
{
    version     2.0;
    format      ascii;
    class       dictionary;
    object      decomposeParDict;
}
numberOfSubdomains 16;
method          scotch;
EOF
echo "[stage] checkMesh"
checkMesh -constant 2>&1 | grep -E "cells:|points:|faces:" | head
rm -rf /stage/motorBike_meshed && mkdir -p /stage/motorBike_meshed
cp -r constant system 0 /stage/motorBike_meshed/
echo STAGE_DONE
