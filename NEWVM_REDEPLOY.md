# New-VM redeploy checklist (400 GB, 36 nodes)

Captures everything learned bringing all 3 solvers up multi-node. Old VM was
destroyed for a 400 GB / 36-node rebuild (36 nodes = 12/zone, needed for the
np=32 scaling cohort; 400 GB fits all 3 solver images × 36 nodes + transient).

## 0. Prereqs
- SSH into the new VM. Copy the repo over (`scp -r` local `cfd-platform/` or
  `git pull` once pushed). The repo already contains every backend/image fix below.
- Fixtures: if backed up, restore `~/cfd-cases-backup` → `/root/cases/` and
  `~/yaris-backup` → `/root/cases-src/yaris/`. Else rebuild (step 4).

## 1. Cluster (36 nodes)
```bash
ZONE_SIZE=12 scripts/cluster-up.sh      # 36 workers (12 a / 12 b / 12 c) + netem + MPI-operator + scheduler + backend + PVCs
```
Netem RTT model unchanged: intra 0.5 / a-b 5 / b-c 5 / a-c 10 ms (in cluster-up.sh `rtt_ms()`).
Zone label = `topology.kubernetes.io/zone`.

## 2. Build + load images (per-phase friendly; ~4 GB each unpacked × 36)
```bash
# backend (Go, multi-stage) — has ALL the fixes in this repo
docker build -t cfd-platform-backend:local backend/ && kind load docker-image cfd-platform-backend:local --name cfd
kubectl rollout restart deploy/cfd-platform-backend -n cfd-platform

# openfoam — published image works as-is
docker pull ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest && kind load docker-image ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest --name cfd

# radioss — MUST build the overlay (adds libcrypt.so.1 via libxcrypt-compat + sshd)
docker build -f docker/openradioss/overlay.Dockerfile -t ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest .
kind load docker-image ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest --name cfd

# codeaster — published image is the full MPI build (PETSc/parallel MUMPS/ParMETIS); works as-is
docker pull ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest && kind load docker-image ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest --name cfd
```

## 3. Fixtures → /root/cases/
- `openfoam.tar.gz` — motorBike (restore from backup or re-fetch the OpenFOAM tutorial).
- `openradioss.tar.gz` — Yaris. master deck renamed **main.key** (LS-Dyna), with
  `*CONTROL_UNITS\nmm,s,tonne,K` after `*KEYWORD` and `*CONTROL_TERMINATION` endtim
  **0.005** (5 ms — full crash is 0.2 s ≈ 3 h/run). Includes: yaris-coarse-v1l.key,
  set-yaris-coarse-v1l.key, wall.key. (Backend extraction picks `main.key` via the
  `*_0000.rad` / `main.{key,rad}` / `*.rad` priority.)
- `code_aster.tar.gz` — **study.comm + mesh.med**, both from the codeaster image's
  test suite (sslv155a is medium, scales clean to np=16/32; forma01a is too small →
  segfaults under MPI):
  ```bash
  docker run --rm -v /root/cabuild:/out --entrypoint bash ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest \
    -lc 'cp /aster/aster/share/aster/tests/sslv155a.comm /out/study.comm; cp /aster/aster/share/aster/tests/sslv155a.mmed /out/mesh.med'
  tar czf /root/cases/code_aster.tar.gz -C /root/cabuild study.comm mesh.med
  ```

## 4. Backend port-forward + smoke each solver
```bash
nohup kubectl port-forward --address 127.0.0.1 svc/cfd-platform-backend -n cfd-platform 8088:80 >/root/bepf.log 2>&1 & disown
# then per solver (1 rep × 3 algos):
cd experiment && python3 run_benchmark.py --backend http://localhost:8088 --cases /root/cases \
  --results-root /mnt/cfd/results --namespace cfd-platform --solvers openfoam --reps 1 --np 16 --out /root/smoke.csv
# repeat --solvers openradioss / code_aster
```

## KEY FIXES BAKED INTO THIS REPO (do not re-discover)
- **mpiP removed for radioss AND codeaster.** `LD_PRELOAD=libmpiP.so` corrupts the
  Fortran solvers' `MPI_Comm_size` (Fortran/C PMPI name-mangling) → "wrong number of
  MPI processes". Works for OpenFOAM (C++). Metric is therefore **wall / solver ELAPSED**,
  not mpiP MPI-time (see metric note below).
- **Pod-local scratch.** radioss → `/tmp/rad`, codeaster → `/tmp/ca`, inputs symlinked
  from the shared PVC. Avoids 0-byte temp files (radioss) / JEVEUX_41 VOLATILE base
  collision (codeaster) on the shared kind hostPath.
- **radioss = .key (LS-Dyna), not native .rad.** Backend extraction resolves the master
  deck (`*_0000.rad` / `main.{key,rad}` / `*.rad`); validator accepts `.key`.
- **codeaster runs `python3 fort.1`, NOT `as_run`.** as_run does its own *local* mpiexec
  and won't distribute across worker pods. The MPIJob mpirun launches python directly;
  imports are prepended, mesh on fort.20. Validator accepts `.comm`+`.med`.
- **extraction job pullPolicy = IfNotPresent** (was defaulting to Always for :latest →
  would pull the broken published image over the kind-loaded fix).

## TO VERIFY ON FIRST DEPLOY (not yet confirmed cross-pod)
1. **codeaster cross-pod run** — single-container `as_run` np=16 = DIAGNOSTIC OK, but the
   backend's `mpirun python3 fort.1` cross-pod path is unverified (mirrors radioss). Smoke it.
2. **codeaster extraction** — `medpartitioner --input-file=*.med` + extract_codeaster_graph.py
   on sslv155a's mesh → must emit a valid edgelist for the scheduler.
3. **Disk during loads** — 4 GB × 36 nodes × 3 ≈ 430 GB?? Recheck: codeaster unpacked
   ~1.9 GB/node × 36 = 68 GB, all 3 ≈ 143 GB + base. Watch `df -h /` during kind loads;
   if tight, load per-phase (gc unused solver image before loading the next).

## METRIC (decided)
Primary = **model communication cost** (hop-bytes/congestion = the QAP objective, from
`zones` + edgelist + zone-latency matrix — computed, no profiling) + **empirical wall /
solver ELAPSED** (radioss prints `ELAPSED TIME`; openfoam ExecutionTime; codeaster CPU time).
mpiP kept as an OpenFOAM-only bonus. Rationale: mpiP incompatible with Fortran solvers via
LD_PRELOAD; wall-clock is the field-standard placement metric (cf. arXiv 2012.14757, TreeMatch,
Hoefler-Snir). `analyze_results.py --metric wall_time_s` already supported.

## KNOWN-GOOD RESULT (reproduce to sanity-check)
OpenRadioss Yaris 5 ms, np=16, clean engine ELAPSED:
mueller-merbach **648 s** (packed zone a) vs random 994 s (+35%) vs greedy 1090 s (+41%).
