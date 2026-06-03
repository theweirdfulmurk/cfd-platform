# Связанные работы

Литературный обзор для Главы 2 диплома. Состояние от 2026-06-02.

## Группировка

Существующая литература делится на четыре направления, ни одно из
которых не пересекается с нашим подходом полностью:

1. **K8s for HPC: foundational evaluation** — поднимают вопрос «можно ли запускать MPI на K8s», измеряют overhead
2. **K8s scheduling plugins** — общая инфраструктура network-aware планирования
3. **Per-rank resource allocation** — varying CPU per rank, fixed placement
4. **MPI library-level rank reordering** — оптимизация внутри MPI runtime

Наша работа = **scheduler-level placement по F-графу**, что ортогонально каждому из этих направлений.

## Ключевые работы

### Xie 2026 — Rank-Aware Resource Scheduling for Tightly-Coupled MPI Workloads on Kubernetes

[arXiv:2603.22691v1](https://arxiv.org/pdf/2603.22691), Purdue University, March 2026.

**Что делает**: per-rank fractional CPU allocation, proportional к cell count
субдомена. Использует OpenFOAM `processorWeights` для bias partitioner'а
и Kubernetes resource requests для bias scheduler'а. Демонстрирует In-Place
Pod Vertical Scaling (KEP-1287, GA в v1.35) для resize CPU без restart.

**Экспериментальный setup**:
- AWS EC2 c5.xlarge (4 vCPU, 8 GB) ×2 или ×4 worker nodes
- k3s v1.35 (cgroups v2)
- OpenFOAM v10, OpenMPI 4.1.x
- AWS EFS (NFS, ReadWriteMany)
- 4-rank: pitzDaily 12K cells, NACA 0012 16K cells
- 16-rank: NACA 0012 refined 101K cells
- **3 runs per configuration**, mean ± std
- std deviations <2% on non-burstable c5

**Ключевые цифры**:
- Hard CPU limits → **78× slowdown** через CFS bandwidth throttling
- Flannel CNI overhead = **0%** (overlay сети не виноваты)
- Cross-node TCP overhead = **30%** vs shared-memory
- Proportional allocation @ 16 ranks: **−20%** wall-clock + **−82%** CPU
  на far-field subdomains (frees 6.5 vCPU)
- Proportional allocation @ 4 ranks: только −3% beyond decomposition topology

**Что заимствуем методологически**:
- **Не использовать CPU limits** (Burstable QoS, requests-only) — критично для MPI
- **Non-burstable instances** для reproducible measurements (наш выбор — Vast.ai m:42009 с EPYC 9654, exclusive 384/384 CPU + 515/515 GB RAM)
- **3+ runs per config** (мы делаем 5 — строже)
- AWS EFS = ReadWriteMany NFS, у нас ReadWriteMany shared storage внутри VM через HostPath / NFS-like

**Где наша работа продолжает**: Xie фиксирует placement (стандартный one-rank-per-vCPU) и варьирует **CPU allocation**. Мы фиксируем CPU allocation и варьируем **placement по F-графу**. Это **complementary contributions**.

**Их limitations, которые мы НЕ закрываем**:
- Только OpenFOAM (мы — 3 решателя)
- 2D кейсы (наш Yaris Coarse 378K elements — 3D crash test с NHTSA validation)
- ≤16 ranks (мы делаем main на N=16 и scaling demo на N=32 — independent verification growing-with-N trend)

### Beltre et al. 2019 — Enabling HPC Workloads on Cloud Infrastructure Using Kubernetes

[CANOPIE-HPC, IEEE 2019, pp.11-20](https://ieeexplore.ieee.org/document/8961095).

**Что делает**: foundational evaluation Kubernetes для MPI на Chameleon Cloud.
HPCG benchmark, network latency tests против bare-metal.

**Ключевая находка**:
- RDMA: near-bare-metal performance
- TCP/IP: **13-22% overhead** в K8s vs bare metal
- Все MPI ranks получают **identical CPU allocation** (без рангового различия)

**Релевантность нам**: confirms что K8s feasible для MPI; задаёт baseline overhead expectations.

### Liu & Guitart 2022 — Fine-Grained Scheduling for Containerized HPC Workloads

[arXiv:2211.11487](https://arxiv.org/pdf/2211.11487), HPCC 2022.

**Что делает**: Scanflow-MPI, two-layer scheduling framework. Внешний слой
выбирает количество контейнеров per job, внутренний — uniform CPU allocation.

**Релевантность**: ещё одна work с **uniform CPU**, никакой per-rank или per-placement differentiation. Mention в нашей работе как «standard practice до per-rank approaches».

### Sochat et al. 2025 — Usability Evaluation of Cloud for HPC Applications

SC'25 Workshops, ACM.

**Что делает**: largest cloud HPC benchmark на сегодня. До **28,672 CPUs** на
AWS/Azure/GCP. 11 proxy applications.

**Релевантность**: ни одно из 11 приложений — CFD/OpenFOAM. Подчёркивает gap в литературе для CFD-specific Kubernetes evaluation. Наша работа этот gap закрывает (3 решателя CFD/FEM).

### Volcano (CNCF) — Network Topology Aware Scheduling

[volcano.sh/docs/network_topology_aware_scheduling](https://volcano.sh/en/docs/network_topology_aware_scheduling/).

**Что делает**: production Kubernetes scheduler с network-aware capabilities.
Введён `HyperNode` CRD для иерархического описания performance domains.

**Релевантность нам**:
- **Не academic work**, а CNCF project
- Использует **manual** HyperNode описание, не **automatically extracted F-граф из задачи**
- Не имеет greedy/MM placement алгоритмов

**Differentiation**: мы делаем automatic F-граф extraction из solver output + algorithmic placement decision; Volcano — только generic «put workloads in same HyperNode if labeled».

### Flux / Fluence (LLNL)

[Fluence на GitHub](https://github.com/flux-framework/flux-k8s).
[LLNL paper](https://www.osti.gov/servlets/purl/1968553).

**Что делает**: Kubernetes scheduler plugin на базе Flux framework. Graph-based resource modeling через Fluxion. Scale до 3000 ranks.

**Ключевая находка**: «lower median end-to-end execution time with lower variability than default scheduler».

**Релевантность**: state-of-the-art HPC scheduler для K8s. Используется в LLNL для production HPC. **Не имеет** explicit F-граф based placement — оптимизирует under resource constraints, не communication graph.

### Houzeaux et al. 2022 — Dynamic Resource Allocation for Efficient Parallel CFD Simulations

Computers & Fluids, vol.243, p.105577. Barcelona Supercomputing Center.

**Что делает**: elastic CFD на Slurm + Alya solver. TALP profiling library
измеряет MPI communication efficiency, autoscaler add/remove whole MPI ranks.

**Релевантность**:
- Closest existing work к нашему по CFD applicability
- **Slurm-based, не Kubernetes** — другая infrastructure
- Adjusts через **add/remove ranks** (disruptive checkpoint-restart), не через placement
- Использует Alya, не OpenFOAM/Code_Aster

### Medeiros et al. 2024, 2025 — Kub, ARC-V

[SBAC-PAD 2024](https://ieeexplore.ieee.org/document/10818000), [Euro-Par 2025](https://link.springer.com/conference/europar).

**Что делает**:
- Kub (2024): horizontal elasticity для MPI на K8s через checkpoint-restart
- ARC-V (2025): vertical scaling для **memory** через In-Place Pod Vertical Scaling

**Релевантность**: первые работы по К8s vertical scaling для HPC. ARC-V **explicitly identifies CPU vertical scaling as open future work** — что закрыла работа Xie 2026.

### Hoefler, Schneider, Lumsdaine 2010 — System Noise Influence on Large-Scale Applications

SC'10, IEEE.

**Что делает**: characterizes **synchronisation amplification** в MPI на больших масштабах. Noise on one rank → cascade через MPI collective barriers.

**Релевантность**: theoretical foundation для понимания **почему placement важен**. Если cross-AZ latency 10ms и один rank в дальнем AZ — все ranks ждут его, cascade slowdown.

### Topology-Aware Rank Reordering для MPI Collectives, Queens University 2016

[IPDRM 2016](https://www.queensu.ca/academia/afsahi/pprl/papers/IPDRM-2016.pdf).

**Что делает**: rank reordering внутри **MPI library** для оптимизации collective operations (MPI_Allgather).

**Ключевые цифры**:
- 20-28% reduction в Allgather execution time
- До 74% reduction для лучшего случая
- Good placement удваивает bandwidth vs random

**Релевантность**: validates **placement matters**. Но это **внутри MPI**, не на уровне scheduler. Наша работа делает это **на уровне K8s**, что shifts decision earlier в pipeline.

## Таблица позиционирования

| Работа | Год | K8s? | CFD/FEM? | F-graph aware? | Multi-region? | Algorithmic placement? |
|---|---|---|---|---|---|---|
| Beltre et al. | 2019 | ✓ | – | – | – | – |
| Queens Univ. (rank reorder) | 2016 | – | – | ◦ MPI-level | – | ◦ heuristic |
| Liu & Guitart (Scanflow) | 2022 | ✓ | – | – | – | – |
| Houzeaux et al. (Alya) | 2022 | – | ✓ | – | – | – |
| Medeiros Kub | 2024 | ✓ | – | – | – | – |
| Sochat et al. | 2025 | ✓ | – | – | – | – |
| Medeiros ARC-V | 2025 | ✓ | – | – | – | – (memory) |
| Fluence / Flux | 2024 | ✓ | – | – | ◦ | ◦ Flux algorithm |
| Volcano (CNCF) | 2024+ | ✓ | – | – | ◦ HyperNode | ◦ manual labels |
| Xie 2026 | 2026 | ✓ | ✓ | – | – | – per-rank CPU |
| **Наша работа** | **2026** | **✓** | **✓ ×3 решателя** | **✓ extracted** | **✓ 3 AZ** | **✓ greedy + Müller-Merbach** |

## Гэп, который закрывает наша работа

Ни одна из перечисленных работ не объединяет **четыре** характеристики
одновременно:

1. **Multi-solver coverage**: 3 разных класса MPI задач (CFD/explicit FEM/implicit FEM)
2. **F-graph extraction**: автоматическое извлечение communication матрицы из solver output
3. **Algorithmic placement**: greedy + Müller-Merbach QAP-approximation в Kubernetes scheduler
4. **Multi-region latency**: реальный bimodal latency contrast (intra-region <1ms vs cross-region 5-30ms)

Все 4 в комбинации = **gap, который закрывает наша работа**.

## Что цитируем в Главе 2

В дипломе порядок цитирования:

1. **Hoefler 2010** — theoretical motivation: synchronisation amplification
2. **Beltre 2019** — first K8s for MPI evaluation, sets baseline
3. **Queens 2016** — placement matters at MPI library level
4. **Houzeaux 2022** — closest CFD work, но на Slurm
5. **Volcano / Fluence** — production schedulers, но без F-graph awareness
6. **Liu & Guitart 2022, Medeiros 2024-2025, Sochat 2025** — K8s HPC scheduling general
7. **Xie 2026** — concurrent work по per-rank CPU; complementary к нашему
8. **Наша работа** — закрывает gap через algorithmic F-graph based placement в multi-region K8s

## Что заимствуем для методологии

Из Xie 2026 (наиболее близкая по setup):

| Methodology choice | Их обоснование | Применяем у нас |
|---|---|---|
| Non-burstable instances | std dev <2% vs 2× variability на burstable | Vast.ai m:42009 — EPYC 9654 exclusive 384/384 CPU + 515/515 GB RAM |
| Burstable QoS (no CPU limits) | hard limits = 78× slowdown через CFS throttling | requests-only в наших MPIJob манифестах |
| ≥3 runs per config | reproducibility statistical | **5 runs** в нашем плане |
| AWS EFS (ReadWriteMany NFS) | shared simulation directories | ReadWriteMany shared storage внутри Vast.ai VM (HostPath / NFS-like) |
| OpenMPI 4.1.x | стандартный, validated | у нас тоже 4.1 в openfoam/openradioss; 2.1 в codeaster |
| k3s v1.35 | lightweight, GA для In-Place scaling | **kind** (lightweight K8s через Docker) внутри Vast.ai VM |
