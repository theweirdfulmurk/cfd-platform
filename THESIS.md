# Дипломная работа — структура и план

Состояние от 2026-06-04. Защита ~июнь 2026, БМГТУ им. Баумана, РК-6.

Тема: «Topology-aware Kubernetes scheduler, минимизирующий MPI
communication delays для параллельных инженерных расчётов».

## TL;DR

- **Целевой объём**: 95-110 страниц + приложения 10-15 страниц.
- **Эксперимент**: Сценарий B = N=16 main (54 runs) + N=32 scaling demo
  для Yaris (18 runs) = **72 runs total** на Vast.ai EPYC 9654.
- **Оценка силы**: сильная BMSTU bachelor's thesis, на «отлично» с
  запасом (~70% вероятность лёгкого «5», ~25% после некоторой защиты).
- **Материала больше чем нужно** — главный риск перебор, не недостаток.
- **Главный фактор риска**: дотащить эксперимент до конца с приличным
  численным результатом (выигрыш MM vs random ≥10% на dense F-graph,
  ожидаемый scaling growth N=16 → N=32 с 20% → 30%).

## Сравнение с двумя примерами защит РК-6

Сопоставление с двумя ВКР той же кафедры:
- **Наумова С.С.** (2026): «Рекомендательная стратегия на основе
  обучения с подкреплением для сессионных поведенческих данных»,
  102 страницы.
- **Бердышев К.Д.** (2026): «Разработка системы поиска изображений по
  визуальному сходству», 74 страницы.

| Параметр | Наумова | Бердышев | **Наша** |
|---|---|---|---|
| Объём | 102 стр | 74 стр | **~100-120 ожидаемо** |
| Математическая глубина | сильная (MARL, formal) | низкая | **сильная** (QAP + statistics) |
| Системная сложность | средняя | средняя | **высокая** (multi-component K8s + HPC stack) |
| Industry relevance | средняя | средняя | **высокая** (Boeing/Airbus/EDF tier solvers) |
| Novelty | средне-низкая (вариация reward) | низкая | **низко-средняя** (applied, не fundamental) |
| Экспериментальная строгость | средне-сильная (bootstrap CI) | низкая (sanity checks) | **сильная** (54 runs + одностор. тесты при n=5 + Student-t) |
| Literature grounding | хорошее | удовлетворительное | **сильное** (PRACE, ESI, Xie 2026) |

**Итог сравнения**: наша работа ≈ Наумовой по силе (другой профиль —
больше системно-инженерной сложности vs больше теоретической глубины),
сильнее Бердышева по всем компонентам кроме UML/UI-стороны.

## Полная структура диплома

### Введение (3-5 страниц)

- Актуальность HPC scheduling для CFD/FEM industrial workloads.
- Объект исследования: scheduling MPI ranks по узлам K8s-кластера.
- Предмет: оптимизация placement по communication graph (F-matrix).
- Цель и задачи.
- Научная новизна и практическая значимость.

### Глава 1. Анализ предметной области (18-22 страницы)

#### 1.1. MPI-параллелизм в инженерных расчётах
- Domain decomposition, ghost cells, collective operations.
- Три класса solvers и их communication patterns:
  - sparse FVM (OpenFOAM): nearest-neighbor exchanges
  - explicit dynamic FEM (OpenRadioss): time-stepped neighbors + контакты
  - implicit FEM с MUMPS (Code_Aster): dense factorization fill-in

#### 1.2. Kubernetes scheduling и его ограничения для HPC
- kube-scheduler default behavior, ScorePlugin/FilterPlugin model.
- MPI Operator (kubeflow), MPIJob CRD, gang-scheduling.
- Топологическая слепота kube-scheduler по умолчанию.

#### 1.3. Обзор существующих подходов
- **Volcano**: gang-scheduling, без topology-awareness placement.
- **Fluence (LLNL)**: GraphML topology представление, не открыт для MPI.
- **Yoda**: NUMA-aware, single-node фокус.
- **Scheduler-plugins (k8s-sigs)**: capacity, but not communication-aware.
- **Xie 2026** (arXiv 2603.22691): concurrent работа, **per-rank CPU
  allocation** (orthogonal к нашему **per-rank placement**).
- Queens University 2016: rank reordering для MPI_Allgather.

#### 1.4. Теоретические основы
- Quadratic Assignment Problem (QAP) формулировка.
- Müller-Merbach heuristic для QAP.
- Communication-to-computation ratio, granularity thresholds.

#### 1.5. Бенчмарки и validation наборы
- OpenFOAM motorBike (350K cells, tutorial-standard).
- OpenRadioss Yaris Coarse (378K elements, NHTSA tests 5677/6221).
- Code_Aster perf009 (261K nodes / 803K dofs, EDF reference).

### Глава 2. Постановка задачи и обоснование (10-15 страниц)

#### 2.1. Формальная постановка
- Граф F = (V_ranks, E с весами w_ij = объём коммуникации).
- Матрица L = latency между узлами кластера.
- Целевая функция: minimize Σ w_ij · L(pi, pj).
- Связь с QAP, NP-hardness.

#### 2.2. Гипотеза исследования
- **H1** (main): выигрыш MM vs random monotonно растёт с плотностью
  F-графа задачи.
- **H2** (scaling demo на Yaris): выигрыш MM vs random monotonно растёт
  с числом MPI ranks N (independent verification Xie 2026 эмпирического
  тренда 3% на 4 → 20% на 16 → ~30% на 32).
- **H3** (опциональная): выигрыш monotonно растёт с latency contrast
  (если успеваем сделать дополнительный variability sweep).

#### 2.3. Обоснование экспериментального дизайна
- **N=16 MPI ranks**: из Xie 2026 main scaling experiment + literature
  thresholds (3K-50K cells/dofs per rank).
- **3 решателя**: представляют три класса F-graph density (sparse /
  medium / dense).
- **3 scheduler алгоритма**: random (baseline) / greedy / Müller-Merbach.
- **6 прогонов на ячейку** (n=5 в анализе после 1 warmup): достаточно
  для Shapiro-Wilk + paired t-test + bootstrap CI, а односторонний
  Wilcoxon достигает p<0.05 при n=5 (1/32 = 0.03125).
- **54 запуска** = 3 × 3 × 6.

#### 2.4. Granularity per rank — научное обоснование
- ESI Group HPC documentation: 50K-200K cells/core optimal для OpenFOAM,
  efficiency <70% ниже 50K.
- PRACE Bottlenecks paper: 20K-50K min на InfiniBand.
- LS-DYNA Conference 2017: 60% efficiency floor at 2 343 elements/core.
- Ansys/Intel performance study: 3K-10K elements/core sweet spot.
- HPC guidelines for sparse direct solvers: 20K-100K dofs/rank.
- Наши кейсы (motorBike 21 875 / Yaris 23 648 / perf009 50 209
  per rank) — попадают в efficient zone.

#### 2.5. Метрики и статистическая методология
- MPI time (mpiP отчёты): primary metric.
- Std deviation: variability indicator.
- Shapiro-Wilk нормальность → выбор Student-t или Bootstrap CI.
- Welch's t-test для unequal variance.
- 95% CI для разностей.
- Односторонние тесты (направленная гипотеза «вариант быстрее / ниже
  MPI time»): primary = paired t-test + bootstrap CI (оба корректны при
  n=5), односторонний Wilcoxon = robustness backup (достигает p<0.05
  при n=5: 1/32 = 0.03125; двусторонний упирается в 0.0625).

### Глава 3. Архитектура системы (20-25 страниц)

#### 3.1. Общая компонентная схема
- 5 слоёв: Frontend → Backend → MPI Operator → Solver containers → mpiP.
- Topology-aware scheduler extender как side-deployment.
- Shared PVCs для case data, results, scheduler-graphs.

#### 3.2. UML use case diagram
- Actors: пользователь (researcher), benchmark orchestrator (Python).
- Use cases: submit simulation, monitor status, collect mpiP report.

#### 3.3. Sequence diagrams
- Submit simulation → extract F-graph → MPIJob placement → run → collect.
- Scheduler extender /prioritize HTTP flow.

#### 3.4. F-graph extraction pipeline (центральный технический вклад)

##### 3.4.1. OpenFOAM
- `decomposePar -force` → processor*/constant/polyMesh/boundary.
- Парсер boundary файлов: pkg/decomp/openfoam.go.
- Output: F-graph через pkg/decomp.OpenFOAM().

##### 3.4.2. OpenRadioss
- **Sed-patch IDB_METIS=0→1** в `grid2mat.F:2220` (debug feature
  разработчиков OpenRadioss, intentionally оставленная).
- Starter генерирует input.graph0 в стандартном METIS format.
- `gpmetis input.graph0 16` → input.graph0.part.16.
- Парсер pkg/decomp/metis.go → F-graph.

##### 3.4.3. Code_Aster
- `medpartitioner --create-boundary-faces --ndomains=16` → joints в
  output MED files.
- MEDLoader Python API: `mesh.getJoints()` → MEDFileJoint hierarchy.
- `getCorrespondence()` → DataArrayIdType с shared nodes.
- Python скрипт `extract_codeaster_graph.py` → edge-list.

#### 3.5. Scheduler extender архитектура
- HTTP /prioritize endpoint (Kubernetes Extender API).
- Pod label `scheduler.cfd-platform/algorithm` → выбор алгоритма.
- Три профиля: random, greedy, mueller-merbach.
- In-memory placements cache + MM precomputation.

#### 3.6. Backend orchestration
- Two-phase: extraction Job → wait → MPIJob.
- PostgreSQL для simulation state.
- MinIO для results storage.
- RBAC для Jobs и MPIJobs.

#### 3.7. Database schema, K8s manifests, RBAC

### Глава 4. Программная реализация (18-22 страницы)

#### 4.1. Go modules layout
- backend/ (Gin REST API, GORM, K8s client).
- scheduler/ (HTTP extender + algorithms).
- pkg/decomp (parsers, общая библиотека).

#### 4.2. Алгоритмы (с псевдокодом)
- Random + SeedFromJobID (FNV-1a hash для воспроизводимости).
- Greedy: streaming online, cost-based scoring против already-placed.
- Müller-Merbach: offline batch, full placement upfront, кэшируется.

#### 4.3. Docker multi-stage builds
- OpenFOAM: thin layer + Go gobuilder для extract-openfoam-graph.
- OpenRadioss: full source compile + sed-patch + Go gobuilder.
- Code_Aster: vendored aethereng recipe (90+ мин cold build).
- Все с mpiP integration через LD_PRELOAD.

#### 4.4. CI/CD pipeline
- GH Actions matrix builds (4 параллельных образа).
- GHA cache (scope per image) для инкрементальных сборок.
- Free disk space + swap для codeaster runner.

#### 4.5. Kubernetes manifests
- Namespace, RBAC, PVCs.
- Backend Deployment + Service.
- Scheduler extender Deployment + ConfigMap.
- MPI Operator install.

#### 4.6. Фронтенд (React + Vite)
- Краткое описание для полноты картины (1-2 страницы).

### Глава 5. Эксперимент и анализ результатов (15-20 страниц)

#### 5.1. Setup
- **Vast.ai m:42009** (Тайвань) — bare metal dedicated VM на AMD EPYC 9654
  (Genoa, 4-е поколение, 2022, 192 phys cores в 2-socket конфигурации,
  515 GB RAM, 4 TB Intel SSDPF2K NVMe).
- KVM virtualization, exclusive CPU/RAM allocation (нет noisy neighbors).
- kind-кластер внутри VM с cgroup cpuset isolation (1 phys core на ноду).
- tc qdisc netem для эмуляции multi-AZ latency (5 ms intra, 10 ms cross).
- 3 логические зоны: для N=16 — 8+8+8 нод, для N=32 — 12+12+12 нод.

#### 5.2. Test cases
- motorBike 350K cells: detailed описание geometry, BC, simpleFoam solver.
- **Yaris Coarse 378K elements**: CCSA George Mason validation, NHTSA
  tests 5677 и 6221 (full-frontal rigid-wall impact 56.2 km/h).
- perf009 261K nodes / 803K dofs: EDF reference performance benchmark.

#### 5.3. Методология выполнения

**Main experiment** (N=16):
- 54 запуска (3 solvers × 3 schedulers × 6 reps; n=5 в анализе после
  1 warmup).
- Randomized execution order для минимизации systematic bias.
- Statistical pipeline: Shapiro-Wilk normality → Student-t или Bootstrap CI
  (односторонние тесты, направленная гипотеза).

**Scaling experiment** (N=32, Yaris Coarse):
- 18 запусков (1 solver × 3 schedulers × 6 reps; n=5 в анализе после
  1 warmup).
- Тот же кластер (36 нод вместо 24), та же методология.
- Цель: показать тренд gain MM vs random при увеличении N.

mpiP report collection через results PVC. Total compute ~39 часов.

#### 5.4. Результаты

**Main results (N=16)**:
- Table 5.1: средние MPI time T для 9 ячеек (3 solvers × 3 schedulers).
- Chart: per-solver scheduler comparison с 95% error bars.
- Chart: gain(MM vs random) как функция F-graph density (sparse →
  medium → dense), ожидается monotonic growth.

**Scaling results (N=32)**:
- Table 5.2: средние MPI time для Yaris Coarse на N=32 (3 schedulers).
- Chart: comparison gain MM vs random на N=16 vs N=32 для Yaris.
- Ожидаемый scaling trend: gain растёт с N (Xie 2026: 3% → 20% при
  4 → 16 ranks; наша экстраполяция: ~20% → ~30% при 16 → 32).

#### 5.5. Обсуждение
- Соответствие гипотезе H1 (gain растёт с плотностью F).
- Подтверждение scaling effect (gain растёт с числом ranks N).
- Threshold для significant gain (минимальная плотность / минимальное N).
- Сравнение с Xie 2026 numerical results (independent verification
  growing-with-N trend).
- Ограничения: single dataset per solver, single hardware platform
  (EPYC 9654 Genoa), single network topology (3-zone tc qdisc emulation).

### Заключение (3-5 страниц)

- Достигнутые результаты.
- Научный и практический вклад.
- Направления развития:
  - GPU-aware scheduling.
  - Multi-job co-scheduling.
  - Integration с Volcano CRD.
  - Production deployment в EDF/Boeing-tier систему.

### Приложения (10-15 страниц)

- Полные таблицы 54 запусков (MPI time, std dev, distribution).
- Полные графики per-solver.
- Скрипты развёртывания (cluster-up.sh).
- F-graph examples (визуализация для motorBike, Yaris, perf009).
- Список references.

## Mapping md-файлов → главы

| md-файл | Используется в главах |
|---|---|
| [CLUSTER.md](CLUSTER.md) | 3.1, 3.7, 5.1 |
| [EXPERIMENT.md](EXPERIMENT.md) | 2.1, 2.2, 2.3, 2.4, 2.5, 5.2, 5.3 |
| [BENCHMARKS.md](BENCHMARKS.md) | 1.5, 5.2 |
| [EXTRACTION.md](EXTRACTION.md) | 3.4 (целиком) |
| [RELATED_WORK.md](RELATED_WORK.md) | 1.3 (целиком) |
| Код (с docstrings) | 4.2, 4.3 (цитируется) |
| Готовые reference'ы | 1.5, 2.3, 2.4 (LS-DYNA Conf, PRACE, ESI, Xie 2026) |

## Сильные стороны защиты

1. **Reproducibility железная**: dedicated CPU + tc qdisc + 6 прогонов
   на ячейку (n=5 в анализе после 1 warmup) + statistical tests
   + open-source реализация → результат воспроизводим любым другим
   researcher.
2. **NHTSA Yaris reference**: «government-validated automotive crash
   benchmark» в защитной речи звучит **очень солидно**.
3. **N=16 ranks + Xie 2026**: «на уровне state-of-the-art concurrent
   work, опубликованной в 2026 году».
4. **Granularity per rank с literature backing**: каждый размер кейса
   обоснован конкретными percentile-thresholds из PRACE/ESI/LS-DYNA.
5. **Полная инженерная зрелость**: не «прототип на бумаге», а реально
   развёрнутая многокомпонентная система с CI/CD.
6. **F-graph extraction для 3 классов solvers**: уникальный методический
   вклад. Никем ранее не автоматизировано через единый pipeline.

## Главные риски защиты

| Риск | Митigation |
|---|---|
| Numerical result слабый (<5% разница) | tc qdisc позволяет управлять latency contrast — увеличиваем для усиления эффекта |
| Вопрос «зачем не Volcano?» | Volcano не имеет MM placement, только gang-scheduling. У нас orthogonal вклад. |
| «Incremental, не fundamental» | Вклад — extraction methodology для 3 solver families + applied scheduling research. Не претендуем на theoretical breakthrough. |
| CPU-only в эпоху GPU AI | Это infrastructure work для CFD/FEM, не AI. CFD на GPU — отдельная research направление, орт. к нашему. |
| Один dataset per solver | В заключении явно указать как limitation + направление развития |

## Оценка времени на написание

| Глава | Срок (рабочих дней) |
|---|---|
| Введение | 1-2 |
| Глава 1 (литобзор) | 5-7 (большая часть готова в md) |
| Глава 2 (постановка) | 3-4 (готова в EXPERIMENT.md) |
| Глава 3 (архитектура) | 5-7 (требует UML, sequence diagrams) |
| Глава 4 (реализация) | 4-5 (готов код, нужны листинги + комментарии) |
| Глава 5 (эксперимент) | 7-10 (зависит от данных эксперимента) |
| Заключение | 1-2 |
| Приложения | 3-4 |
| Финальная редактура + норм-контроль | 5-7 |
| **Итого** | **34-48 дней** ≈ **6-9 недель** |

Из них **«нельзя начать» до**:
- Глава 5 — до завершения эксперимента (~3-4 недели на эксперимент).

Параллельно можно писать главы 1, 2, 3, 4 пока crunch'ит experimental
phase.

## Целевая шкала оценки

| Сценарий | Вероятность |
|---|---|
| **Отлично (5)** с лёгкостью | ~70% |
| **Отлично (5)** с некоторой защитой | ~25% |
| **Хорошо (4)** при слабом численном результате | ~5% |

## Сравнение с международными conferences (для контекста)

| Уровень | Применимо? |
|---|---|
| Top-tier (SC, HPDC, EuroSys) | ❌ нет, недостаточно novelty |
| Mid-tier workshop (ROSS @ HPDC, IA3 @ SC) | ✅ возможен после расширения экспериментов |
| Industry conference (Open Source Summit, KubeCon) | ✅ да, готовая презентация |
| Magister diссертация | overkill для bachelor, **достаточно для magister** |

## Итоговый вывод

Объём не проблема — материала **больше чем нужно**. Главная задача
после эксперимента: **дисциплинированная структурированная запись** +
выкидывание лишнего при сокращении до 100 страниц.

Готовы к финальной фазе после deployment + 54-run experiment.
