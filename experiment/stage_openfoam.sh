#!/bin/bash
# Stage the motorBike OpenFOAM case: serial mesh (blockMesh + snappyHexMesh),
# set decomposeParDict to 16 scotch subdomains, export the reconstructed case
# (constant/ system/ 0/) to /root/stage/motorBike_meshed for tarballing.
# NOTE: opencfd image needs --entrypoint /bin/bash + sourced etc/bashrc so the
# OpenFOAM apps resolve LD_LIBRARY_PATH (plain `bash -lc` only sets PATH).
set -euo pipefail
exec > /root/stage_openfoam.log 2>&1
IMG=ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest
mkdir -p /root/stage
docker run --rm --entrypoint /bin/bash -v /root/stage:/stage "$IMG" -c '
set -e
source /usr/lib/openfoam/openfoam2306/etc/bashrc
cd /tmp && rm -rf mb && cp -r "$FOAM_TUTORIALS"/incompressible/simpleFoam/motorBike mb && cd mb
mkdir -p constant/triSurface
cp -f "$FOAM_TUTORIALS"/resources/geometry/motorBike.obj.gz constant/triSurface/
echo "[stage] surfaceFeatureExtract"; surfaceFeatureExtract > log.sfe 2>&1
echo "[stage] blockMesh";            blockMesh            > log.blockMesh 2>&1
echo "[stage] snappyHexMesh (serial, may take several min)"; snappyHexMesh -overwrite > log.snappy 2>&1
cp -r 0.orig 0
cat > system/decomposeParDict <<EOF
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
'
echo HOST_DONE
