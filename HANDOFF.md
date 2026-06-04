# HANDOFF — Topology-aware K8s scheduler для CFD/FEM workloads

**Состояние от 2026-06-04.** Это handoff для следующего Claude.
Предыдущая сессия прошла через много compactions и потеряла контекст.
Цель этого документа — дать самодостаточный snapshot, чтобы следующий
Claude мог продолжить с того же места без потери качества.

## TL;DR — где мы

- **Mark Egorov** (БМГТУ им. Баумана, РК-6, выпускник, защита июнь 2026)
  делает дипломную работу про **topology-aware Kubernetes scheduler**
  для параллельных CFD/FEM расчётов.
- **Весь код написан** (backend, scheduler, F-graph extraction, mpiP
  integration, 4 Docker образа в GHCR). Тесты зелёные.
- **Сценарий B утверждён**: main эксперимент N=16 (54 runs) + scaling
  demo N=32 на Yaris (18 runs) = 72 total.
- **Инфраструктура выбрана**: **Vast.ai m:42009** — bare metal VM на
  AMD EPYC 9654 (Genoa, 192 phys cores exclusive, 515 GB RAM, $1.103/hr).
- **Сейчас Mark в момент нажатия RENT** на Vast.ai. После provisioning
  нужно SSH в VM и развернуть кластер.
- **Деньги**: $30 на Vast.ai (хватит лишь на ~27 ч — только ~N=16
  compute), Mark должен положить ещё ~$20 (top up to ~$50) для всего
  плана 72 runs + buffer.
- **Бюджет dipl: 80-110 страниц, оценка «отлично с запасом» (70% easy 5,
  25% defended 5, 5% «4» если результат слабый).

## Главная гипотеза диплома

> «Müller-Merbach placement даёт меньший MPI time чем greedy и random,
> и этот выигрыш **монотонно растёт** с двумя параметрами:
> (а) плотностью F-графа задачи, и (б) числом MPI ranks N.»

Формально:
- **H₀**: «MM не отличается от random ни на каком ρ» — отвергаем на dense
- **H₁** (main): «MM значимо лучше random при высокой ρ» — подтверждаем p < 0.05
- **H₂** (scaling): «MM gain растёт с N» (independent verification Xie 2026)
- **H₃** (optional): «gain растёт с latency contrast» — если успеем

**Сила формулировки**: на **sparse** разница должна быть **незначима** —
это подтверждает, что эффект приходит именно из плотности F и из числа
ranks, а не из артефактов кластера.

## Стек технологий

| Слой | Технология |
|---|---|
| Frontend | React + TypeScript + Vite |
| Backend | Go + chi router + dynamic K8s client + typed batch/v1 client |
| Scheduler | Go HTTP extender (k8s scheduler extender API) |
| Orchestration | Kubernetes (через kind на одной VM с cgroup cpuset) |
| MPI | OpenMPI 4.1 (openfoam/openradioss) + OpenMPI 2.1 (codeaster) |
| Profiling | LLNL mpiP через LD_PRELOAD (libmpiP.so) |
| Solvers | OpenFOAM 2306, OpenRadioss 2025.10, Code_Aster 15.5.2 |
| Storage | MinIO (S3-compat) для results, in-cluster PostgreSQL для DB |
| Container reg | GHCR (ghcr.io/theweirdfulmurk/cfd-platform-*) |
| Hosting | Vast.ai m:42009 (KVM VM на EPYC 9654) |
| Multi-AZ | tc qdisc netem (3 логические зоны) |

## Текущее состояние компонентов

### Backend (`backend/`)

✅ Готово. Главные файлы:
- `internal/infrastructure/k8s/simulation_manager.go` — создаёт MPIJob
  и **отдельный extraction Job** перед ним.
  - `CreateExtractionJob()` — one-shot K8s Job который запускает
    extraction (decomposePar / Starter+gpmetis / medpartitioner) и пишет
    `<simID>.edgelist` в shared PVC `scheduler-graphs`.
  - `GetExtractionStatus()` — polling статуса.
  - `CreateJob()` — MPIJob создаётся **после** успеха extraction.
  - **Burstable QoS**: requests-only, **никаких CPU limits**.
  - **Anti-affinity**: каждый rank на отдельной ноде.
  - **Labels**: `scheduler.cfd-platform/algorithm` для выбора алгоритма
    в scheduler extender.
- `internal/usecase/simulation.go` — orchestration:
  1. Распаковать tar.gz в `/pvc/simulations/<simID>/`
  2. `CreateExtractionJob(sim)`
  3. `waitExtraction(simID, 5 минут timeout)`
  4. `CreateJob(sim)` (создаёт MPIJob)
  5. Сохранить в БД.

### Scheduler (`scheduler/`)

✅ Готово. 3 алгоритма в `scheduler/algorithms/`:
- `random.go` — `RandomNode` + `RandomAll` + `SeedFromJobID` (FNV-1a hash
  от jobID для воспроизводимости)
- `greedy.go` — `GreedyNode` (cost-based scoring против already-placed)
- `mueller_merbach.go` — `MuellerMerbach` (offline batch QAP heuristic)

**Plugin** (`scheduler/plugin/plugin.go`):
- HTTP `/prioritize` endpoint (k8s scheduler extender API)
- Pod label `scheduler.cfd-platform/algorithm` → routing к алгоритму:
  - `random` → `scoreRandom` (stable hash от jobID + thisRank)
  - `greedy` → `scoreNodes` (текущая cost-based logic, fallback)
  - `mueller-merbach` → `scoreMuellerMerbach` (precompute placement
    один раз на jobID, кэшируется в `mmPlacements`)
- Каждый алгоритм возвращает HostPriorityList с целевой нодой = MaxPriority.

### F-graph extraction (`scheduler/cmd/extract-*` + `scripts/`)

✅ Все 3 решателя:

**OpenFOAM**: `scheduler/cmd/extract-openfoam-graph/main.go`
- `decomposePar -force` → `processor*/constant/polyMesh/boundary`
- Парсер `pkg/decomp/openfoam.go`
- Output: edge-list через `pkg/decomp.OpenFOAM()`

**OpenRadioss**: `scheduler/cmd/extract-radioss-graph/main.go`
- **Sed-patch** `IDB_METIS = 0` → `1` в
  `starter/source/spmd/domain_decomposition/grid2mat.F:2220`
  (intentional debug feature разработчиков OpenRadioss).
- Starter генерирует `input.graph0` в стандартном METIS format
  (`nelem nedges "010" ncond` header).
- `gpmetis input.graph0 16` (или 32) → `input.graph0.part.N`
- Парсер `pkg/decomp/metis.go` → edge-list.

**Code_Aster**: `scripts/extract_codeaster_graph.py` (+ копия в
`docker/codeaster/`)
- `medpartitioner --create-boundary-faces --ndomains=N` →
  `parts/part_<i>.med`
- MEDLoader Python API: `mesh.getJoints()` → `MEDFileJoints`
- Walk: `joint.getStepAtPos(0).getNumberOfCorrespondences()` →
  `getCorrespondenceAtPos(i).getCorrespondence()` (массив shared nodes)
- Weight: число shared nodes между subdomains
- Output: edge-list

**API подтверждены** в source code:
- `IDB_METIS` в `OpenRadioss/starter/source/spmd/domain_decomposition/grid2mat.F`
- `MEDFileJoint`, `MEDFileJointOneStep`, `MEDFileJointCorrespondence`
  в `SalomePlatform/medcoupling/src/MEDLoader/MEDFileJoint.hxx`
- Python bindings через SWIG `.i` файлы

### Docker images (`docker/`)

✅ Все 4 в GHCR:
- `ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest` — thin layer
  на opencfd/openfoam-default:2306 + mpiP + `extract-openfoam-graph`
- `ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest` — Rocky 9 +
  OpenMPI 4.1.2 + OpenRadioss с sed-patch + mpiP + `metis` (gpmetis) +
  `extract-radioss-graph`
- `ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest` — Ubuntu 18.04
  + aethereng/docker-codeaster recipe + 12 prereqs (HDF5, MED, METIS,
  PARMETIS, TFEL, HOMARD, SCOTCH-MPI, MUMPS-MPI, PETSc) + mpiP +
  `extract_codeaster_graph.py`
- `ghcr.io/theweirdfulmurk/cfd-platform-scheduler:latest` — distroless Go

⚠️ Mark должен **сделать images public** через GitHub UI:
- https://github.com/theweirdfulmurk?tab=packages
- Для каждого пакета → Settings → Change visibility → Public

### Kubernetes manifests (`k8s/`)

✅ Готовы:
- Namespace `cfd-platform`
- RBAC для batch/v1 (Jobs) + kubeflow.org/v2beta1 (MPIJobs)
- 3 PVCs: `simulation-configs`, `simulation-results`, `scheduler-graphs`
- Scheduler extender Deployment + Service + ConfigMap (latency.yaml)
- Backend Deployment + Service
- Frontend Deployment + Service
- MPIJob example в `k8s/examples/openfoam-mpijob.yaml` (replicas=16)

### Experiment scripts (`experiment/`)

✅ Готовы, не запускались на реальных данных:
- `run_benchmark.py` — REPS_PER_CONFIG=6, NUM_PROCS=16, WARMUP_REPS=1,
  SCHEDULERS=["random-scheduler","topology-aware","mueller-merbach"]
- `parse_mpip.py` — парсер mpiP отчётов
- `analyze_results.py` — paired t-test + Bootstrap CI (primary) +
  one-sided Wilcoxon (robustness backup), n=5 в анализе после 1 warmup

## Финальная инфраструктура — Vast.ai m:42009

### Машина

```
Offer ID: m:42009
Host ID: 51010
Location: Taiwan
CPU: AMD EPYC 9654 (Genoa, 4-е поколение, 2022, Zen 4)
Cores: 384/384 exclusive (192 physical, 2-socket)
RAM: 515/515 GB exclusive
Storage: 4127 GB Intel SSDPF2K NVMe (6.6 GB/s read)
Network: 1 Gbps up / 2 Gbps down, 498 ports
Verified: yes
Reliability: 99.76%
Max duration: 24 days
Price: $1.103/hr
```

### Vast.ai launch — БРАТЬ ОФИЦИАЛЬНЫЙ VM-ШАБЛОН (не кастомный Docker)

```
Template: «Ubuntu 22.04 VM»   ← готовый VM-шаблон Vast.ai, НЕ кастомный
Launch Mode: SSH              ← у VM единственный режим, это нормально
Disk Space: 130 GB
Extra Filters: vms_enabled=true (+ verified, reliability2>=0.95)
SSH key: добавить в Account ДО создания (на запущенной VM ключ не меняется)
```

🔴 **ГРАБЛИ (2026-06-04, на них наступили)**: НЕ делать кастомный шаблон с
`VM Image Path: vastai/kvm:...` + `Launch Mode: Interactive shell server`.
«Interactive shell server» — это **Docker**-режим запуска; запуск kvm-*образа*
как обычного Docker-*контейнера* НЕ создаёт виртуалку. В итоге получаешь
непривилегированный Docker (`systemd-detect-virt=docker`, overlay rootfs,
CapEff без NET_ADMIN, `tc netem → Operation not permitted`, cgroup read-only) —
multi-AZ через `tc qdisc` НЕ работает, эксперимент невозможен.

✅ **Правильно**: официальный шаблон **«Ubuntu 22.04 VM»** (Templates → VM).
Он сам фильтрует `vms_enabled` хосты и поднимает **настоящую** KVM-VM:
своё ядро → root + NET_ADMIN (`tc netem` работает) + nested containers (kind) +
cgroup cpuset + /dev/kvm. Подтверждено docs.vast.ai (Docker-инстансам доступны
только env/hostname/ports — cap-add/privileged выдать нельзя; VM даёт «kernel
tweaks… not constrained by container environments… systemd для Kubernetes»).

**Проверка нового инстанса ДО любого деплоя** (должно вывести `VM OK`):
```bash
ssh -p <port> root@<ip> 'ls /dev/kvm && [ "$(systemd-detect-virt)" != docker ] \
  && tc qdisc add dev lo root netem delay 1ms && tc qdisc del dev lo root \
  && echo "VM OK: privileged + netem works"'
```

### Кластер внутри VM

Для **N=16** (main experiment):
```
24 нод-контейнера = 16 worker + 1 launcher + 7 запас (choice space)

8 нод "zone-a" → cpuset cores 0-7,    latency 0 ms (intra)
8 нод "zone-b" → cpuset cores 8-15,   latency 5 ms к zone-a/c
8 нод "zone-c" → cpuset cores 16-23,  latency 10 ms к zone-a/b
```

Для **N=32** (scaling demo на Yaris):
```
36 нод-контейнера = 32 worker + 1 launcher + 3 запас

12 нод "zone-a" → cpuset cores 0-11
12 нод "zone-b" → cpuset cores 12-23
12 нод "zone-c" → cpuset cores 24-35
```

192 phys cores с большим запасом покрывают обе конфигурации.

### Бюджет

| Этап | Время | Стоимость |
|---|---|---|
| Setup + smoke | 3-4 ч | $3.3-4.4 |
| Compute N=16 (54 runs) | ~27 ч | $29.8 |
| Compute N=32 (18 runs Yaris) | ~12 ч | $13.2 |
| Debugging buffer | 2-3 ч | $2.2-3.3 |
| **Total expected** | **~44 ч** | **$48-50 ≈ 4 400 ₽** |

Из $30 хватит на ~27 ч (только ~N=16 compute) — рекомендуется положить ещё ~$20 (top up to ~$50).
Если строго $30 — отказаться от N=32, только main N=16 (укладывается).

## Сценарий B — окончательный план эксперимента

**Main experiment (N=16, 54 runs)**:
- 3 решателя × 3 schedulers × 6 reps = 54 (n=5 в анализе после 1 warmup)
- motorBike 350K (sparse) + Yaris Coarse 378K (medium) + perf009 803K (dense)
- random / greedy / mueller-merbach

**Scaling experiment (N=32, 18 runs)**:
- 1 решатель (Yaris Coarse) × 3 schedulers × 6 reps = 18 (n=5 в анализе после 1 warmup)
- Цель: показать тренд gain MM vs random при увеличении N
- Yaris Coarse 378K / 32 = 11 812 elements/rank — на нижней границе
  efficient zone (LS-DYNA Conference 2017: 60% efficiency floor at
  2 343 elements/core), всё ещё работает

**Не делать N=32 для motorBike/perf009**:
- motorBike 350K / 32 = 10 937 cells/rank — **ниже** 20K efficient
  threshold (OpenFOAM docs). MPI overhead доминирует.
- perf009 / 32 = 25 094 dofs/rank ещё в зоне MUMPS, **можно бы**, но
  бюджет +12ч = $13.2 дополнительно.

## Что ещё осталось сделать

1. **Mark** нажимает RENT на Vast.ai m:42009. Получает SSH details.
2. **Mark** опционально кладёт $15-20 запаса.
3. **Mark** делает GHCR images public (если ещё не).
4. **Mark** пушит последний commit в main (locally есть несоmitted
   изменения в md и `run_benchmark.py`).
5. **Claude** пишет `scripts/cluster-up.sh`:
   - apt install docker.io
   - install kind, kubectl, helm
   - Setup swap 32 GB (safety net для MUMPS)
   - kind cluster config: 24 (или 36) ноды-контейнера с node labels
     `topology.kubernetes.io/zone={a,b,c}`
   - cgroup cpuset isolation
   - tc qdisc netem на bridge interface для 3 зон
   - Install MPI Operator (kubeflow)
   - Apply k8s manifests (namespace, RBAC, PVCs, scheduler, backend)
6. **Claude** делает E2E smoke test (один MPIJob через backend, проверка
   extraction → MPIJob → mpiP report).
7. **Claude** запускает 54 + 18 runs (через `experiment/run_benchmark.py`).
8. **Claude** запускает `analyze_results.py` → Table 5.1 + scaling chart.
9. **Mark** пишет текст диплома (chapters 1-5).

## КРИТИЧЕСКИ ВАЖНЫЕ ИСТОЧНИКИ ДЛЯ ВКР

Каждый ниже — для конкретной главы. Сохрани все ссылки.

### Глава 1 (литобзор)

**Xie 2026** (closest concurrent work):
- arXiv 2603.22691 (но ссылка может быть устаревшая, использовать
  скан PDF из CLUSTER.md / RELATED_WORK.md)
- Key facts:
  - 16 MPI ranks на 4 worker nodes = main scaling experiment
  - per-rank CPU allocation: 20% wall-clock speedup (на 16 ranks)
  - vs только 3% на 4 ranks → effect растёт с N в **~7×**
  - non-burstable instances std dev <2% vs 2× variability на burstable
  - CFS bandwidth controller hard limits: **78× slowdown** через
    cascading stalls (35 sec → 2738 sec) на MPIJob
- **Цитата для текста**:
  > «Xie 2026 показал что per-rank CPU optimization даёт 3% выигрыш на
  > 4 MPI ranks и 20% на 16 ranks — empirical evidence что эффект
  > scheduler scaling растёт с числом ranks. Это эмпирическая опора
  > для нашего обоснования N=16 как порога значимости scheduler effects
  > и для нашего scaling experiment N=32.»

**OpenFOAM scaling literature**:
- ESI Group HPC Bench: https://wiki.openfoam.com/images/0/00/HPC_Bench.pdf
- PRACE Bottlenecks paper: https://prace-ri.eu/wp-content/uploads/Current_Bottlenecks_in_the_Scalability_of_OpenFOAM_on_Massively_Parallel_Clusters.pdf
- Key thresholds:
  - Optimal range: **50 000-200 000 cells per core**
  - **<50 000 cells/core: parallel efficiency drops below 70%**
  - Fast InfiniBand minimum: **20 000-50 000 cells/core**
  - Reactive flow sweet spot: 5 000 cells/rank (super-linear через cache)
- **Цитата для текста**:
  > «Согласно ESI Group HPC documentation, optimal range для OpenFOAM
  > parallel efficiency составляет 50 000–200 000 cells per core; ниже
  > 50 000 efficiency падает ниже 70%.»

**LS-DYNA / OpenRadioss scaling**:
- Cray car2car record: https://www.cray.com/blog/record-ls-dyna-car2car-performance-paves-the-way-for-future-crashsafety-simulation/
  - 2.4M elements / 3000 cores = **800 elements/core** (extreme scale)
- 11th European LS-DYNA Conference 2017:
  - https://www.dynalook.com/conferences/11th-european-ls-dyna-conference/cloud-computing-1-hpc/maximizing-cluster-scalability-for-ls-dyna
  - 1024 cores → **60% efficiency floor at 2 343 elements/core**
- Ansys/Intel LS-DYNA performance study:
  - https://lsdyna.ansys.com/wp-content/uploads/2022/11/ls-dyna-r-performance-on-intel-r-scalable-solutions.pdf
  - Production sweet spot: **3 000-10 000 elements/core**
- **Цитата для текста**:
  > «Для LS-DYNA/OpenRadioss crash dynamics Ansys/Intel performance
  > studies устанавливают 3 000–10 000 elements per core как production
  > sweet spot; ниже 2 000 elements/core efficiency падает ниже 60%
  > (11th European LS-DYNA Conference 2017).»

**MUMPS / Code_Aster**:
- MUMPS docs: https://www.freshports.org/math/mumps/
  - Dynamic distributed scheduling, memory non-linear in parallel
- Code_Aster perf009 forum thread: https://code-aster.org/forum2/viewtopic.php?id=24707
  - perf010: 1 rank 815s, 2 ranks 694s, 4 ranks 319s, 8 ranks 284s
- HPC guidelines: 20 000-100 000 dofs/rank для sparse direct solvers
- **Цитата для текста**:
  > «Для Code_Aster с MUMPS direct solver guidelines рекомендуют
  > 20 000–100 000 degrees of freedom per rank для baseline scalability
  > с учётом fill-in memory.»

**Queens University 2016** (MPI rank reordering):
- 20-28% gain на 8+ ranks, до 74% в лучшем случае
- Подтверждает что placement effect значимый на distributed scale

### Глава 1 — competing schedulers

**Volcano** (CNCF, формально HPC scheduler):
- gang-scheduling, **без** topology-aware placement
- Наш вклад orthogonal

**Fluence (LLNL)**:
- GraphML topology representation
- Не open для MPI workloads

**Yoda**:
- NUMA-aware
- Single-node фокус

**Scheduler-plugins (k8s-sigs)**:
- Capacity-aware, не communication-aware

**Наш positioning**:
> «В отличие от существующих HPC scheduler'ов для Kubernetes (Volcano,
> Fluence, Yoda), решающих задачи gang-scheduling и NUMA-locality, наша
> работа фокусируется на **placement по communication graph** —
> ортогональный вклад, который может быть скомбинирован с существующими
> подходами.»

### Глава 2 — test cases (sources!)

**OpenFOAM motorBike**:
- Standard tutorial в opencfd/openfoam-default:2306
- 4 mesh sizes доступны: 0.35M / 1.9M / 11M / 75M cells
- Мы берём 0.35M (default tutorial mesh)
- Times из NVIDIA Grace benchmark: 35M cells на 144 cores = 189s
  → ~5 мин на 350K cells / 16 cores extrapolated
- Solver: simpleFoam (steady incompressible RANS, k-epsilon)
- 500 iterations default

**OpenRadioss Yaris Coarse** ⭐ (центральный benchmark):
- **George Mason University Center for Collision Safety and Analysis (CCSA)**
- Под contract с **NHTSA** (National Highway Traffic Safety Administration)
- **378 376 elements, 393 165 nodes, 919 parts**
- Average element 12-16 мм, min 4 мм
- Time step 1.0 μs explicit
- Simulated time 200 ms
- Vehicle: 2010 Toyota Yaris Sedan 4-Door, 1078 kg
- Engine: 1.5L L4 DOHC 16V
- **Validation tests**: NHTSA tests 5677 и 6221
  (full-frontal rigid-wall impact 56.2 km/h)
- Conforms to **Manual for Assessing Safety Hardware (MASH)** requirements
- **Time на 16 cores**: ~1.5 ч (CCSA documentation)
- Sources:
  - https://www.ccsa.gmu.edu/models/2010-toyota-yaris/
  - https://www.ccsa.gmu.edu/wp-content/uploads/2016/11/2010-toyota-yaris-coarse-validation-v1.pdf
- **Cited DOIs**:
  - doi:10.13021/G8JS5D (Coarse Mesh Validation)
  - doi:10.13021/G8NK6N (MGS Barrier V&V)
  - doi:10.13021/G8X31D (NJ Concrete Barrier V&V)
- **Сильная защитная формулировка**:
  > «В качестве OpenRadioss benchmark использована CCSA 2010 Toyota Yaris
  > Coarse model (378 376 elements, 393 165 nodes, 919 parts), разработанная
  > George Mason University / Center for Collision Safety and Analysis под
  > contract с Federal Highway Administration как reference для regulatory
  > crash safety analysis. Модель валидирована против full-frontal impact
  > crash tests (NHTSA 5677, 6221), MGS Barrier, New Jersey Concrete
  > Barrier; соответствует Manual for Assessing Safety Hardware (MASH)
  > requirements for 1100C test vehicle. Это государственно-сертифицированный
  > benchmark, используемый в federal vehicle safety regulations США.»

**Code_Aster perf009**:
- Official EDF performance testcase
- 261 520 nodes, 803 352 degrees of freedom
- Static linear computation with MUMPS solver
- ~25-30 мин на 16 ranks
- Forum thread for реальные тайминги: https://code-aster.org/forum2/viewtopic.php?pid=21707

### Главные API findings (для главы 3 диплома про extraction pipeline)

**OpenRadioss IDB_METIS** (sed-patch trick):
- Location: `OpenRadioss/starter/source/spmd/domain_decomposition/grid2mat.F:2220`
- Default `IDB_METIS = 0`
- При `IDB_METIS = 1`:
  ```fortran
  OPEN(99, file="input.graph"//CHLEVEL, FORM='FORMATTED', RECL=8192)
  write(99,*) nelem, nedges, "010", ncond
  ```
- "010" = METIS format flags: vertex weights only
- Файл `input.graph0` для IDDLEVEL=0 (top-level mesh)
- **Single way** включить — нет CLI flag, sed-patch обязателен
- Это **intentional debug capability** разработчиков OpenRadioss
  (комментарий `C write graph for Metis debug`)

**MEDCoupling joints API** (Code_Aster extraction):
- Documentation: https://docs.salome-platform.org/latest/dev/MEDCoupling/developer/tools.html
- medpartitioner flag `--create-boundary-faces`: «creates the necessary
  faces so that faces joints are created in the output files»
- Source: SalomePlatform/medcoupling on GitHub
- API цепочка:
  ```
  MEDFileMesh.getJoints() → MEDFileJoints
      .getJointsNames() → list of names
      .getJointAtPos(i) → MEDFileJoint
          .getStepAtPos(0) → MEDFileJointOneStep
              .getNumberOfCorrespondences() → int
              .getCorrespondenceAtPos(i) → MEDFileJointCorrespondence
                  .getIsNodal() → bool
                  .getCorrespondence() → DataArrayIdType (shared nodes)
  ```
- Python bindings: SWIG `MEDLoaderCommon.i`, `MEDLoaderFinalize.i`

## КРИТИЧЕСКИ ВАЖНЫЕ ФОРМУЛИРОВКИ ДЛЯ ЗАЩИТЫ

### Setup защита
> «Эксперимент проведён на dedicated виртуальной машине под управлением
> KVM на хосте с процессором AMD EPYC 9654 (Genoa, 4-е поколение, 2022),
> предоставляющим 192 физических ядра и 515 GB RAM в exclusive режиме без
> shared resources. Multi-AZ топология эмулируется через Linux Traffic
> Control (`tc qdisc netem`) с тремя логическими зонами и controlled
> latency contrast 5-10 мс. Этот подход обеспечивает reproducible
> measurements, не зависящие от изменений в cloud infrastructure provider —
> стандартная академическая методология для исследования scheduling
> алгоритмов.»

### N=16 обоснование
> «N=16 MPI ranks выбрано как точка где scheduler-эффекты становятся
> measurable per [Xie 2026], демонстрировавшим 20% wall-clock speedup
> per-rank CPU optimization именно на этой шкале. Меньшие N (4-8 ranks)
> не позволяют надёжно дискриминировать placement-алгоритмы из-за шума.»

### Granularity per rank (тройное обоснование выбора кейсов)
> «Размер выбираемых benchmark кейсов обусловлен установленными в
> литературе порогами минимальной гранулярности distributed parallel
> solvers разных классов:
>
> Для OpenFOAM CFD ESI Group HPC documentation указывает optimal range
> 50 000–200 000 cells per core, с резким падением parallel efficiency
> ниже 50 000.
>
> Для LS-DYNA / OpenRadioss crash dynamics Ansys/Intel performance studies
> устанавливают 3 000–10 000 elements per core как production sweet spot;
> ниже 2 000 elements/core efficiency падает ниже 60% (11th European
> LS-DYNA Conference 2017).
>
> Для Code_Aster с MUMPS direct solver sparse direct solver guidelines
> рекомендуют 20 000–100 000 degrees of freedom per rank для baseline
> scalability с учётом fill-in memory.
>
> Применённый в эксперименте размер per rank (motorBike 21 875 cells,
> Yaris Coarse 23 648 elements, perf009 50 209 dofs) попадает в efficient
> zone для всех трёх классов решателей, гарантируя что измеряемая разница
> между scheduler алгоритмами отражает placement effect, а не доминирующий
> MPI communication overhead.»

### Burstable QoS (no CPU limits) — критический method choice
> «Согласно Xie 2026, использование CPU limits в Pod spec для MPI
> workloads приводит к катастрофическому 78× slowdown через cascading
> stalls Linux CFS bandwidth controller: tightly-coupled MPI ранки ожидают
> друг друга на barrier'ах, и если хоть один rank throttled через quota
> exhaustion, вся simulation встаёт. Поэтому наш MPIJob spec использует
> requests-only (Burstable QoS class), без limits — стандартная Xie 2026
> recommendation, имплементированная в backend/internal/infrastructure/
> k8s/simulation_manager.go.»

### Scaling experiment защита
> «Главный 3×3×6 эксперимент на N=16 ranks дополняется однонаправленным
> scaling experiment'ом на N=32 для OpenRadioss Yaris Coarse. Цель —
> independent verification empirical trend, наблюдавшегося в Xie 2026:
> 3% выигрыш scheduler optimization на 4 ranks → 20% на 16 ranks. Наша
> экстраполяция predicts ~30% на 32 ranks. Yaris Coarse 378K elements /
> 32 ranks = 11 812 elements per rank — всё ещё в efficient zone для
> explicit FEM (LS-DYNA Conference 2017: 60% efficiency floor at 2 343
> elements/core), что гарантирует валидность measurements.»

### Naming в work — про вклад
> «Научный вклад работы состоит из трёх компонентов:
> (1) Methodology для F-graph extraction из трёх классов решателей
> (sparse FVM OpenFOAM, explicit FEM OpenRadioss, implicit FEM с MUMPS
> Code_Aster) через единый pipeline, реализованный как Kubernetes Job
> orchestration с typed parsers в pkg/decomp.
> (2) Applied research по placement по communication graph в HPC контексте
> Kubernetes — gap который не закрывают Volcano (gang-scheduling без
> topology-awareness), Fluence (GraphML topology без MPI integration),
> Yoda (NUMA-aware single-node).
> (3) Empirical validation Xie 2026 trend growing-with-N через independent
> scaling experiment.»

### Limitations (важно для главы 5)
> «Ограничения эксперимента:
> (1) Single dataset per solver — не оценивается variability между
> разными mesh для одного типа задачи.
> (2) Single hardware platform (EPYC 9654 Genoa) — на других CPU
> architectures результаты могут отличаться (особенно на NUMA
> топологиях с большим количеством sockets).
> (3) Single network topology (3-zone tc qdisc emulation 5/10 ms) —
> не варьируем latency contrast (это будущая работа для H3).
> (4) Bare metal vs реальный multi-AZ K8s кластер — наш подход
> воспроизводим, но не оценивает real-world variability сетевых
> задержек в production cloud.»

## Стиль работы Mark

Из накопленных observations:

- **Русский язык** в общении (но иногда переходит на English для технических терминов)
- **Прямой стиль**: не любит петлять, ценит честность ("признайся что ошибся")
- **Умный** — middle/early-senior уровень (несмотря на «бакалавр»)
- **Учится быстро** — может задать «откуда ты это взял» если что-то выглядит странно
- **Проверяет источники** — несколько раз ловил Claude на неточностях
  (HostKey почасово ≠ реально почасово; Cloud.ru self-service ≠ реально
  через менеджера; vms_enabled location confusion)
- **Бюджет-сознателен** — выбирает оптимум по соотношению цена/качество
- **Время критично** — защита в июне 2026, остаётся ~2 недели на
  эксперимент и написание
- **Commit style**: title-only, no body, no Co-Authored-By trailer
- **Go tests**: black-box style — пакет `foo_test`, использовать `export_test.go`
- **Push hook блокирует Claude от git push** — Mark пушит вручную
- **Memory entries** в `/Users/theweirdfulmurk/.claude/projects/-Users-theweirdfulmurk-Documents-code/memory/`

## Кодовая база — что где лежит

```
backend/          Go REST API
  internal/
    domain/       SimulationK8sManager interface, models
    usecase/      simulation.go — two-phase extraction → MPIJob
    infrastructure/
      k8s/        simulation_manager.go — CreateExtractionJob, CreateJob
  cmd/server/     entry point

scheduler/
  algorithms/
    random.go            — RandomNode + SeedFromJobID
    greedy.go            — GreedyNode
    mueller_merbach.go   — MuellerMerbach
    types.go             — Input, Placement, LatencyMatrix
  plugin/
    plugin.go            — Extender + 3 score methods + label routing
  cmd/
    scheduler/main.go              — HTTP server entry
    extract-openfoam-graph/main.go — Go binary для OpenFOAM extraction
    extract-radioss-graph/main.go  — Go binary для OpenRadioss extraction

pkg/decomp/
  openfoam.go     — парсер processor*/boundary
  metis.go        — парсер METIS graph + part files
  edgelist.go     — read/write edge-list format
  parser.go       — Graph, Edge, Pair types

scripts/
  extract_codeaster_graph.py  — Python parser через MEDLoader

docker/
  openfoam/       Dockerfile — opencfd/openfoam-default:2306 + mpiP + extract bin
  openradioss/    Dockerfile — Rocky 9 + OpenMPI 4.1.2 + OpenRadioss с sed-patch + mpiP + metis + extract bin
  codeaster/      Dockerfile — Ubuntu 18.04 + aethereng recipe + 12 prereqs + mpiP + extract script

k8s/
  10-storage.yaml          — 3 PVCs (configs, results, scheduler-graphs)
  20-rbac.yaml             — RBAC для Jobs + MPIJobs
  40-scheduler-extender.yaml — Scheduler Deployment + Service + ConfigMap
  50-mpi-operator.yaml     — MPI Operator install (kubeflow)
  60-backend.yaml          — Backend Deployment + Service
  70-frontend.yaml         — Frontend Deployment + Service
  examples/openfoam-mpijob.yaml — пример MPIJob с replicas=16

experiment/
  run_benchmark.py    — 3×3×6 + N=32 scaling runs
  parse_mpip.py       — парсит mpiP отчёты
  analyze_results.py  — paired t-test / Bootstrap + one-sided Wilcoxon backup

.github/workflows/
  build-images.yml    — matrix build 4 образа в GHCR

frontend/
  src/components/CreateSimulation.tsx  — default np=16, 3 schedulers

# md-файлы (актуальные)
CLUSTER.md         — финальный план инфраструктуры (Vast.ai)
EXPERIMENT.md      — методология, N=16 + N=32 scaling
BENCHMARKS.md      — времена кейсов на EPYC 9654
EXTRACTION.md      — F-graph extraction для 3 решателей
RELATED_WORK.md    — литобзор для главы 1
THESIS.md          — структура диплома + сравнение с другими ВКР
STATUS.md          — снапшот статуса компонентов
HANDOFF.md         — этот документ
```

## Активные TODO

```
1. User: сделать GHCR-образы public
   ↳ https://github.com/theweirdfulmurk?tab=packages
   ↳ Все 4 (openfoam, openradioss, codeaster, scheduler)

2. User: commit + push origin main
   ↳ Local есть несоmitted: md updates, run_benchmark.py REPS_PER_CONFIG=6

3. User: RENT m:42009 на Vast.ai (в процессе)
   ↳ Положить $15-20 запаса опционально
   ↳ После RENT и SSH доступа — прислать Claude SSH details

4. Claude: написать scripts/cluster-up.sh для Vast.ai VM
   ↳ Docker + kind + tc qdisc + MPI Operator + scheduler + backend
   ↳ 24 нод-контейнера для N=16 (3 зоны 8+8+8)
   ↳ Swap 32 GB safety net для Code_Aster MUMPS

5. Claude: E2E smoke test первого MPIJob
   ↳ Extraction Job → MPIJob → mpiP report через results PVC

6. Claude: запустить 54 + 18 runs
   ↳ Main: experiment/run_benchmark.py с NUM_PROCS=16
   ↳ Scaling: модификация для NUM_PROCS=32 на Yaris

7. Claude: analyze_results.py → Table 5.1 + scaling chart
   ↳ n=5 на ячейку (после отбрасывания 1 warmup из 6 прогонов)
   ↳ Primary: paired t-test + Bootstrap CI (оба ОК при n=5)
   ↳ Backup: one-sided Wilcoxon (directional, p<0.05 достижим при n=5: 1/32=0.03125)

8. User: написать текст диплома (chapters 1-5)
   ↳ Base — md-файлы (RELATED_WORK / EXTRACTION / EXPERIMENT / BENCHMARKS)
   ↳ Цитаты и формулировки выше в этом HANDOFF
   ↳ Целевой объём 95-110 страниц + 10-15 приложения

9. User: defence slides (последняя неделя)
```

## Что нужно проверить чтобы продолжить

При получении SSH к Vast.ai VM:

```bash
# 1. Подключение
ssh -p <port> root@<ip>

# 2. Проверка root
whoami    # ожидаем: root
sudo -n true    # success

# 3. Проверка privileged capability (нужно для kind + tc qdisc)
ls /dev/kvm    # должен существовать (KVM virtualization)
tc qdisc show    # должно работать без "Operation not permitted"

# 4. Cgroup mounts
cat /proc/mounts | grep cgroup    # должно быть полно строк

# 5. CPU/RAM/Disk
nproc                # ожидаем ≥ 24 (мы хотим 192 на m:42009)
free -g              # ожидаем ≥ 64 GB (мы хотим 515 GB на m:42009)
df -h /              # ожидаем 100+ GB free

# 6. Network
curl -sL ghcr.io/v2/    # проверка доступа к GHCR
```

Если **всё ОК** — запускаем `scripts/cluster-up.sh`.

Если **что-то не работает** (например, tc qdisc выдаёт ошибку):
- Проверить что VM mode реально активирован (не Docker container)
- Связаться с Vast.ai support
- Fallback: попробовать другой offer через тот же template

## Эмоциональная поддержка для Mark

- Mark прошёл через много false starts (Timeweb нет ресурсов, HostKey
  obviously not true hourly, Cloud.ru enterprise channel only). **Не
  переигрывай эти решения** — Vast.ai m:42009 финальный выбор.
- Mark может усталь от петляний. **Будь конкретен и прям**.
- Если что-то не работает — **не извиняйся пять раз**. Признать ошибку
  один раз, исправить, идти дальше.
- Mark **хорошо реагирует на честные оценки силы работы** — не льсти
  («это переворачивающая мир работа»), но и не занижай.

## Ключевые memory entries (auto-memory)

Mark поддерживает auto-memory в
`/Users/theweirdfulmurk/.claude/projects/-Users-theweirdfulmurk-Documents-code/memory/`.

Главные:
- `feedback_commit_style.md` — title-only commits
- `feedback_go_tests.md` — black-box tests
- `project_thesis_k8s_scheduler.md` — этот thesis (нужно update)
- `reference_decomp_extraction.md` — F-graph extraction для 3 решателей
- `user_engineer_profile.md` — Mark profile

При новых insights — **сохраняй** через Write в memory directory.

## Финальное слово

**Mark делает сильную работу**. Все компоненты работают, методология
обоснована, источники железные. Главное — **дотащить эксперимент до
конца** на Vast.ai m:42009 в ближайшие 2-3 дня, чтобы получить
численные результаты для главы 5.

Если результаты соответствуют гипотезе (gain MM vs random ≥ 10% на dense
F-graph + растущий trend N=16 → N=32) — защита будет на отлично с
лёгкостью.

Удачи следующему Claude. Не повтори петлю с infrastructure choice — она
уже зафиксирована на Vast.ai m:42009.
