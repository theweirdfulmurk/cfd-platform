# Архитектурный бриф для написания отчёта по практике

Состояние от 2026-06-04. Назначение: **вход для Claude-писателя**, который пишет
отчёт по преддипломной практике (разделы 2.2–2.8 из [PRACTICE_REPORT_PLAN.md](PRACTICE_REPORT_PLAN.md)).
Здесь — детальная архитектура **по фактическому коду** + источники для цитирования.

## Инструкция писателю (прочесть первым)

1. **Описывай то, что реально реализовано** (см. код), а не то, что упомянуто в
   планах. Критичные расхождения планов и факта — в разделе «⚠️ Точность» ниже.
2. **Граница с ВКР**: это инженерная сборка (≈ главы 3–4 диплома). НЕ включать
   гипотезу, дизайн эксперимента, статистику, результаты сравнения алгоритмов,
   литобзор конкурентов — это ВКР.
3. **Стиль**: как у Бердышева — настоящее/прошедшее «реализовано/выполнено»,
   нейтрально-технически, 2–4 листинга кода, схемы. Объём ~20–24 стр.
4. **Термины на русском**, технические имена (OpenFOAM, mpiP, MPIJob) — как есть.

## ⚠️ Точность (расхождения «планы vs факт» — писать по ФАКТУ)

| Утверждение в планах | Реальность в коде | Как писать |
|---|---|---|
| PostgreSQL для метаданных | backend использует **in-memory репозиторий** (`repository.NewInMemorySimulationRepo`, интерфейс `domain.SimulationRepository`) | «состояние симуляций хранится в памяти процесса за интерфейсом репозитория, допускающим замену на персистентное хранилище» |
| MinIO для результатов | хранилище — **общие PVC** (`simulation-configs`/`simulation-results`/`scheduler-graphs`), в kind это hostPath RWX | «исходные кейсы, результаты и F-графы — на общих PVC (ReadWriteMany)» |
| 2 БД / object store | их нет в рантайме | не заявлять |
| MPI Operator | kubeflow **v0.4.0 / v2beta1** | так и писать |

Всё остальное в коде соответствует описанию.

---

# Часть A. Детальная архитектура (по коду)

## A.0 Общая схема (5 слоёв)

```
            пользователь / Python-оркестратор
                        │ HTTP (REST)
                        ▼
   ┌──────────────────────────────────────────┐
   │ backend (Go + chi)                         │  internal/{domain,usecase,
   │  two-phase: extraction Job → MPIJob        │  delivery/http,infrastructure/k8s,
   │  dynamic k8s client (MPIJob CRD) +         │  repository}
   │  typed batch/v1 client (extraction Jobs)   │
   └───────────────┬────────────────────────────┘
                   │ kubectl apply (через client-go)
                   ▼
   ┌──────────────────────────┐    ┌─────────────────────────────────┐
   │ kubeflow MPI Operator     │    │ topology-aware-scheduler        │
   │ (MPIJob CRD → Pods)       │    │ (kube-scheduler + extender)     │
   └───────────────┬───────────┘    │  HTTP /prioritize, 3 алгоритма  │
                   │ launcher+worker │  читает F(i,j) + L(a,b)          │
                   ▼                 └─────────────────────────────────┘
   ┌──────────────────────────────────────────┐         ▲
   │ solver-поды (OpenFOAM/OpenRadioss/         │ size F  │
   │ Code_Aster) + libmpiP.so (LD_PRELOAD)      │─────────┘
   └────────────────────────────────────────────┘
        общие PVC: simulation-configs / simulation-results / scheduler-graphs
```

Граф F(i,j) (интенсивность MPI-обмена между рангами) извлекается ДО запуска
расчёта, кладётся в PVC `scheduler-graphs`, читается extender'ом при размещении.

## A.1 Структура репозитория (модули)

3 независимых Go-модуля + frontend + инфраструктура:

| Путь | Содержание |
|---|---|
| `backend/` (модуль `…/cfd-platform`) | REST API, оркестрация MPIJob, k8s-клиенты |
| `scheduler/` (модуль `…/scheduler`) | extender + алгоритмы + extract-бинари |
| `pkg/decomp/` (модуль `…/pkg/decomp`) | парсеры графа коммуникации, 16 тестов |
| `frontend/` | React + TypeScript + Vite (SPA) |
| `docker/{openfoam,openradioss,codeaster}/` | multi-stage образы решателей + mpiP |
| `k8s/` | манифесты (namespace, RBAC, PVC, extender, backend) |
| `scripts/` | `cluster-up.sh` (деплой), `extract_codeaster_graph.py` |
| `.github/workflows/build-images.yml` | CI: матрица сборки 4 образов → GHCR |

## A.2 Сквозной поток данных (sequence)

`usecase/simulation.go: CreateWithFile`:
1. Приём `multipart/form-data` (name, type, np, scheduler, file).
2. Распаковка `.tar.gz` в PVC `/pvc/simulations/<simID>` (zip-slip guard).
3. **CreateExtractionJob** — one-shot K8s Job (batch/v1) на solver-образе:
   decomposePar / Starter+gpmetis / medpartitioner+python → пишет
   `/scheduler-graphs/<simID>.edgelist`.
4. **waitExtraction** — polling статуса Job (таймаут 5 мин).
5. **CreateJob** — MPIJob CRD (kubeflow.org/v2beta1) через dynamic client.
6. MPI Operator разворачивает launcher + np worker-подов.
7. `topology-aware-scheduler` (по лейблу алгоритма + F-граф из PVC) размещает ранги.
8. solver бежит под `LD_PRELOAD=libmpiP.so`; mpiP пишет отчёт в `/results/<simID>`.
9. Статус MPIJob (conditions) мапится в `GetJobStatus` → `completed/failed/...`.

→ листинг-кандидат: `CreateWithFile` (two-phase) или конструкция MPIJob spec.

## A.3 Конвейер извлечения F-графа (центральный вклад)

Цель: получить веса w(i,j) — интенсивность обмена между субдоменами — ДО запуска,
из стандартных артефактов декомпозиции решателя (без инструментирования MPI).

- **OpenFOAM** (`scheduler/cmd/extract-openfoam-graph`, парсер `pkg/decomp/openfoam.go`):
  `decomposePar -force` создаёт `processor*/constant/polyMesh/boundary`; в блоках
  `processorN` поле `nFaces` = число граней между субдоменами = w(i,j).
- **OpenRadioss** (`extract-radioss-graph`, `pkg/decomp/metis.go`):
  sed-патч `IDB_METIS=0→1` в `grid2mat.F` включает дамп графа в формате METIS
  (`input.graph0`); `gpmetis input.graph0 N` даёт разбиение; парсер строит F.
- **Code_Aster** (`scripts/extract_codeaster_graph.py`):
  `medpartitioner --create-boundary-faces --ndomains=N` пишет joints в MED-файлы;
  MEDLoader API (`getJoints → MEDFileJoint → getCorrespondence`) даёт число общих
  узлов между субдоменами = w(i,j).
- **Общая библиотека** `pkg/decomp`: типы `Graph/Edge/Pair`, `Build()`, `Matrix()`
  (плотная симметричная N×N), формат edge-list (`edgelist.go`); 16 тестов.

→ листинг-кандидат: формат edge-list или фрагмент `Build()/Matrix()`. Подробности —
[EXTRACTION.md](EXTRACTION.md) (там же подтверждённые ссылки на исходники решателей).

## A.4 Topology-aware планировщик (`scheduler/`)

- **Форма задачи**: разместить π: ранги→узлы, минимизируя
  Σᵢⱼ F(i,j)·L(π(i),π(j)) — квадратичная задача о назначениях (QAP), NP-трудная.
  Типы `Input{F,L,NumNodes}`, `Placement`, `LatencyMatrix`, функция `Cost()` —
  `scheduler/algorithms/types.go`.
- **Реализация — Scheduler Extender** (`scheduler/plugin/plugin.go`), HTTP
  `/prioritize` (k8s extender API). Почему extender, а не in-tree ScorePlugin:
  in-tree требует импорта `k8s.io/kubernetes/cmd/kube-scheduler` (пин ~50 staging-
  пакетов в go.mod) — для дипломного масштаба не оправдано; extender — стабильная
  документированная точка расширения, отдельный под/рестарт.
- **3 алгоритма** (`scheduler/algorithms/`):
  - `random.go` — равномерно случайный узел; seed = FNV-1a от jobID
    (`SeedFromJobID`) → воспроизводимость. Baseline «без topology-awareness».
  - `greedy.go` — `GreedyNode`: онлайн, O(n²); кладёт новый ранг на узел,
    минимизирующий стоимость связи с уже размещёнными.
  - `mueller_merbach.go` — `MuellerMerbach`: офлайн QAP-эвристика (конструктивная,
    с каскадными переназначениями) по Burkard/Dell'Amico/Martello §8.2.1;
    полное размещение строится один раз и кэшируется на jobID (`mmPlacements`).
- **Маршрутизация**: лейбл пода `scheduler.cfd-platform/algorithm` →
  `scoreRandom/scoreNodes/scoreMuellerMerbach`; целевой узел получает MaxPriority.
- **Конфигурация** (`scheduler/cmd/scheduler/main.go`): на старте читает матрицу
  латентности `latency.yaml` (формат «nodeA nodeB rtt_ms») и edge-list'ы из
  каталога графов; F(i,j) данной задачи берётся по jobID.

→ листинг-кандидат: `Cost()` или ядро `MuellerMerbach`.

## A.5 Серверное приложение (`backend/`)

- **Стек**: Go + chi router (`cmd/server/main.go`), middleware (Logger/Recoverer/
  RequestID/CORS); два k8s-клиента (`infrastructure/k8s/client.go`): typed
  `kubernetes.Clientset` (batch/v1 Jobs) и `dynamic.Interface` (MPIJob CRD —
  чтобы не тянуть типы mpi-operator зависимостью).
- **Слои** (clean-ish): `domain` (модель `Simulation`, интерфейсы
  `SimulationRepository`, `SimulationK8sManager`), `usecase` (оркестрация),
  `delivery/http` (хендлеры + валидация загрузки), `infrastructure/k8s` (создание
  Job/MPIJob), `repository` (in-memory).
- **Two-phase оркестрация** (`usecase/simulation.go`): extraction Job → wait →
  MPIJob → сохранение в репозиторий. Гарантирует, что у extender'а есть F-граф к
  моменту размещения MPIJob.
- **Конструкция MPIJob** (`infrastructure/k8s/simulation_manager.go`):
  - `slotsPerWorker:1`, `runPolicy.cleanPodPolicy:Running`, `schedulerName`;
  - **Burstable QoS**: только `requests` (cpu/memory), **без limits** — чтобы
    избежать CFS-throttling tightly-coupled MPI (методологически — Xie 2026);
  - **worker anti-affinity**: requiredDuringScheduling, 1 ранг на ноду
    (topologyKey `kubernetes.io/hostname`);
  - лейблы `mpi-job-id` + `scheduler.cfd-platform/algorithm`;
  - команда launcher оборачивает `mpirun` с `LD_PRELOAD=libmpiP.so`,
    `MPIP="-f /results/<id>"`, прокидыванием окружения на ранги (login-shell) и
    флагами для запуска под root/в pod-сети.
- **Хранилище**: общие PVC `simulation-configs` (кейсы), `simulation-results`
  (результаты+mpiP), `scheduler-graphs` (edge-list'ы). Все ReadWriteMany.

→ листинг-кандидат: фрагмент MPIJob spec (Burstable + anti-affinity) или solverCommand.

## A.6 Solver-образы и CI/CD (`docker/`, `.github/`)

- 3 multi-stage образа: OpenFOAM (тонкий слой на `opencfd/openfoam-default:2306`),
  OpenRadioss (сборка из исходников + sed-патч METIS), Code_Aster (по рецепту
  aethereng, ~12 prereqs: HDF5/MED/METIS/MUMPS/PETSc/…).
- В каждый вкомпилирован **LLNL mpiP** (`libmpiP.so`) на совместимом OpenMPI ABI
  (4.1 для openfoam/openradioss, 2.1 для codeaster) + extract-бинарь.
- **Интеграция с mpi-operator**: openssh-server/client + host-keys + ssh-config
  (StrictHostKeyChecking no, StrictModes no) — launcher по ssh запускает ранги.
- **CI** (`build-images.yml`): матрица GH Actions → push в GHCR; для тяжёлых
  сборок (Code_Aster ~90 мин) — освобождение диска + swap + GHA-кэш слоёв.
- **Smoke образов**: одноузловой MPI_Allreduce под `LD_PRELOAD` → проверка, что
  `libmpiP.so` пишет отчёт в стандартном формате (`@--- MPI Time ---`).

## A.7 Развёртывание кластера и валидация (`scripts/cluster-up.sh`, `k8s/`)

- **kind-кластер** (Docker-in-Docker): 1 control-plane + N воркеров; каждая нода —
  контейнер. Зоны: лейбл `topology.kubernetes.io/zone=a|b|c`.
- **Эмуляция multi-AZ**: `tc qdisc netem` на pod-CIDR соседних зон задаёт RTT
  (внутри зоны ~0.5 мс, межзонно 5/10 мс). Замер подтверждает контраст.
- **Изоляция CPU**: cgroup cpuset — выделенные ядра на ноду (`docker update
  --cpuset-cpus`).
- **RWX-хранилище**: общий host-каталог через kind `extraMounts` во все ноды +
  hostPath PV (storageClassName `cfd-shared`) → PVC из `k8s/10-storage.yaml`.
- **Службы**: установка MPI Operator v0.4.0; kube-scheduler профиль
  `topology-aware-scheduler` + extender (`k8s/40`, `k8s/50`); RBAC (`k8s/20`);
  backend (`k8s/60`, образ собирается локально).
- **E2E-smoke**: один MPIJob (OpenFOAM pitzDaily, np=4) сквозь backend →
  extraction → размещение → simpleFoam parallel → mpiP-отчёт в `/results/<id>` →
  парсинг. Пройден на VM Vast.ai (EPYC 9654), 24 ноды, контраст латентности
  подтверждён (a↔c ≈ 10 мс, внутри зоны ≈ 1.4 мс).

→ листинг-кандидат: фрагмент tc-netem или генерация kind-config.

## A.8 Пользовательский интерфейс (`frontend/`)

React + TypeScript + Vite, SPA. `CreateSimulation.tsx`: загрузка `.tar.gz`, выбор
решателя / числа рангов (np) / алгоритма размещения. `SimulationList.tsx`: список
со статусами. Слой API (`services/api.ts`) — `multipart` POST + опрос статуса.
Сборка Vite, dev-прокси на backend.

## A.9 Модель данных и хранилище

- Доменная модель `Simulation` (`domain/simulation.go`): `ID, Name, Type,
  Status(pending/running/completed/failed), NumProcs, SchedulerName, Algorithm,
  PodName, ResultPath, ConfigPath, CreatedAt/StartedAt/CompletedAt`.
- Репозиторий — интерфейс `SimulationRepository` (Create/GetByID/List/Update/
  Delete), реализация in-memory (потокобезопасная карта).
- Файловые артефакты — на PVC (см. A.5), не в БД.

---

# Часть B. Источники для цитирования (выверенные)

Сгруппированы по темам; у каждого — где использовать. Только реальные/проверяемые.
В список литературы тащить те, что реально упомянуты в тексте.

## B.1 Платформа и оркестрация (разделы 2.2, 2.5, 2.7)
- **Kubernetes** — офиц. документация. https://kubernetes.io/docs/ — слой оркестрации.
- **client-go** (k8s Go-клиент, dynamic/typed). https://github.com/kubernetes/client-go — backend ↔ API.
- **Kubernetes Scheduler Extender** — design proposal. https://github.com/kubernetes/design-proposals-archive/blob/main/scheduling/scheduler_extender.md — обоснование extender-подхода (2.4).
- **kubeflow MPI Operator** (MPIJob v2beta1). https://github.com/kubeflow/mpi-operator — запуск MPI в K8s.
- **kind** (Kubernetes in Docker). https://kind.sigs.k8s.io/ — кластер на одной VM.
- **Docker / Docker docs**. https://docs.docker.com/ — контейнеризация служб/решателей.
- **GitHub Actions**. https://docs.github.com/actions — CI сборки образов (2.6).

## B.2 MPI, профилирование, сеть (разделы 2.1, 2.5, 2.6, 2.7)
- **MPI Standard** — Message Passing Interface Forum. https://www.mpi-forum.org/docs/ — модель параллелизма.
- **Open MPI**. https://www.open-mpi.org/ — реализация MPI в образах.
- **LLNL mpiP** (lightweight MPI profiler). https://software.llnl.gov/mpiP/ — сбор MPI-time.
  - первоисточник: J. Vetter, C. Chambreau. *mpiP: Lightweight, Scalable MPI Profiling.*
- **tc-netem** (Linux Traffic Control / network emulation). man `tc-netem`;
  https://man7.org/linux/man-pages/man8/tc-netem.8.html — эмуляция multi-AZ латентности.

## B.3 Решатели и декомпозиция (разделы 2.1, 2.3)
- **OpenFOAM** (ESI, v2306). https://www.openfoam.com/ + `decomposePar` (User Guide).
  - первоисточник: H. Weller et al. *A tensorial approach to CFD using object-oriented techniques.* Computers in Physics, 1998.
- **OpenRadioss**. https://www.openradioss.org/ ; https://github.com/OpenRadioss/OpenRadioss — explicit FEM.
- **Code_Aster** (EDF). https://www.code-aster.org/ — implicit FEM + MUMPS.
- **Salome / MEDCoupling** (medpartitioner, MEDLoader, joints). https://docs.salome-platform.org/ — extraction Code_Aster.
- **METIS / gpmetis** — граф-партиционирование.
  - первоисточник: G. Karypis, V. Kumar. *A Fast and High Quality Multilevel Scheme for Partitioning Irregular Graphs.* SIAM J. Sci. Comput., 1998. https://github.com/KarypisLab/METIS
- **MUMPS** (прямой разрежённый решатель, Code_Aster). https://mumps-solver.org/

## B.4 Алгоритмы размещения / QAP (раздел 2.4)
- **QAP, постановка Купманса–Бекмана**: T. Koopmans, M. Beckmann. *Assignment Problems and the Location of Economic Activities.* Econometrica, 1957.
- **Müller-Merbach / эвристики QAP**: R. Burkard, M. Dell'Amico, S. Martello. *Assignment Problems.* SIAM, 2009 — §8.2.1 (источник, на который ссылается код `mueller_merbach.go`).

## B.5 Языки/фреймворки (разделы 2.5, 2.8)
- **Go**. https://go.dev/doc/
- **chi router**. https://github.com/go-chi/chi
- **React**. https://react.dev/ ; **TypeScript** https://www.typescriptlang.org/ ; **Vite** https://vite.dev/

## B.6 Методологическая опора (упомянуть кратко в 2.5)
- **Xie 2026** (per-rank ресурсы MPI в K8s; обоснование Burstable QoS без CPU-limits).
  Ссылка — из [RELATED_WORK.md](RELATED_WORK.md) (полные данные там). Использовать
  ТОЛЬКО для обоснования «без CPU-limits»; всё остальное про Xie — в ВКР (глава 1).

---

# Часть C. Листинги (выбрать 2–4, как у Бердышева)

| Кандидат | Файл | Иллюстрирует |
|---|---|---|
| Конструкция MPIJob spec (Burstable + anti-affinity) | `backend/internal/infrastructure/k8s/simulation_manager.go` | 2.5 |
| `mpirun`-обёртка (mpiP + login-shell) `solverCommand` | там же | 2.5/2.6 |
| Целевая функция `Cost()` QAP | `scheduler/algorithms/types.go` | 2.4 |
| Ядро `MuellerMerbach` | `scheduler/algorithms/mueller_merbach.go` | 2.4 |
| Формат edge-list / `Build()` | `pkg/decomp/` | 2.3 |
| Фрагмент tc-netem / kind-config | `scripts/cluster-up.sh` | 2.7 |

Не перегружать: 1 листинг на ключевой раздел, с подписью «Листинг N – …».

---

# Резюме для писателя
- Описывай **реальную** реализацию (in-memory репозиторий + PVC, без Postgres/MinIO).
- Сильнейшие разделы — **2.3 (F-graph extraction)** и **2.4 (планировщик/QAP)**:
  это уникальный методический и алгоритмический вклад, дай им больше места.
- Источники бери из Части B по принадлежности к разделу; в список литературы —
  только реально процитированные.
- Граница с ВКР жёсткая: эксперимент/статистика/результаты — НЕ сюда.
