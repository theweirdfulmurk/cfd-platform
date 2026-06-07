#!/bin/bash
# Isolating test: run the decomposed motorBike case with mpirun -np 16 inside a
# SINGLE container (no MPI-operator / ssh / netem). Tells us whether the solver
# + decomposed case is healthy, separate from the cluster MPI plumbing.
source /usr/lib/openfoam/openfoam2306/etc/bashrc >/dev/null 2>&1
cd /tmp && rm -rf mb && cp -r /stage/motorBike_meshed mb && cd mb
echo "[solo] decomposePar -force"
decomposePar -force > /stage/solo_decompose.log 2>&1; echo "DECOMP_RC=$?"
grep -E "Processor [0-9]+|Number of cells" /stage/solo_decompose.log | head -3
echo "[solo] mpirun -np 16 simpleFoam -parallel  (no mpiP first)"
mpirun --allow-run-as-root -np 16 bash -lc "cd /tmp/mb && simpleFoam -parallel" > /stage/solo_solver.log 2>&1
echo "SOLVER_RC=$?"
echo "[solo] ---- last 30 lines of solver log ----"
tail -30 /stage/solo_solver.log
echo "[solo] DONE"
