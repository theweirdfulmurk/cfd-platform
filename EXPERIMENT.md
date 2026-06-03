# Эксперимент — методология

Дизайн количественного эксперимента для защиты диплома.
Состояние от 2026-06-02.

## Главная гипотеза

> «Müller-Merbach placement даёт меньший MPI-time чем greedy и random,
> и этот выигрыш **растёт с плотностью** F-графа задачи.»

Формально:
- **H₀**: «MM не отличается от random ни на каком ρ» — отвергаем на dense
- **H₁**: «MM значимо лучше random при высокой ρ» — подтверждаем p < 0.05

Сила формулировки в том, что на **sparse** разница должна быть
**незначима** — это подтверждает, что эффект приходит именно из
плотности F, а не из артефактов кластера.

## Что измеряем

Каждый запуск даёт одно число — **total MPI time** в миллисекундах,
извлекаемое из mpiP-отчёта (секция `@--- MPI Time (seconds) ---`,
строка `*` aggregated по всем ranks).

Парсится в `experiment/parse_mpip.py`.

## Дизайн эксперимента

```
3 решателя × 3 schedulers × 5 повторов = 45 запусков
N MPI ranks per job = 16
```

| Размерность | Сколько | Зачем |
|---|---|---|
| Решатели | 3 | три точки на оси плотности F-графа — для **формы** кривой Δ(ρ), не направления |
| Schedulers | 3 | random (baseline = «kube-scheduler default») + greedy (state-of-the-art) + MM (наше) |
| Повторов | 5 | минимум для Shapiro-Wilk normality test и paired t-test с разумной power |
| **N ranks per job** | **16** | обоснование ниже |

### Минимальный размер задачи per rank (научное обоснование)

Для корректного измерения **scheduler placement effect** через MPI time,
размер задачи должен быть таким чтобы communication overhead **не
доминировал** над reasonable computation per rank. Иначе любая
placement-стратегия даст одинаковый результат — всё съест MPI overhead.

Установившиеся пороги из академической литературы для OpenFOAM CFD
(применимые с поправкой к FEM):

| Источник | Threshold |
|---|---|
| [OpenFOAM HPC Performance documentation](https://wiki.openfoam.com/images/0/00/HPC_Bench.pdf) | optimal range **50 000-200 000 cells per core** |
| Same | **<50 000 cells per core: parallel efficiency drops below 70%** |
| [PRACE Bottlenecks of OpenFOAM Scalability](https://prace-ri.eu/wp-content/uploads/Current_Bottlenecks_in_the_Scalability_of_OpenFOAM_on_Massively_Parallel_Clusters.pdf) | on fast InfiniBand minimum 20 000-50 000 cells per core |
| OpenFOAM reactive flow studies | sweet spot **5 000 cells per rank** (super-linear via caching effects) |

**Для explicit FEM crash dynamics** (LS-DYNA / OpenRadioss):

| Источник | Threshold per rank |
|---|---|
| [Cray car2car record performance](https://www.cray.com/blog/record-ls-dyna-car2car-performance-paves-the-way-for-future-crashsafety-simulation/) | 2.4M elements / 3000 cores = **800 elements/core** (extreme scale) |
| [11th European LS-DYNA Conference 2017](https://www.dynalook.com/conferences/11th-european-ls-dyna-conference/cloud-computing-1-hpc/maximizing-cluster-scalability-for-ls-dyna) | 1024 cores → **60% efficiency floor at 2 343 elements/core** |
| [Ansys/Intel LS-DYNA performance study](https://lsdyna.ansys.com/wp-content/uploads/2022/11/ls-dyna-r-performance-on-intel-r-scalable-solutions.pdf) | production sweet spot **3 000-10 000 elements/core** |

**Для implicit FEM с MUMPS** (Code_Aster):

| Источник | Threshold per rank |
|---|---|
| [MUMPS documentation (math/mumps)](https://www.freshports.org/math/mumps/) | dynamic distributed scheduling, **memory non-linear in parallel** |
| Code_Aster perf009 official benchmark | 803K dofs / варьируется по числу процессов, sub-linear scaling |
| HPC guidelines для sparse direct solvers | recommended **20 000-100 000 dofs/rank** |

### Наш выбор кейсов в свете этих порогов

При N=16 MPI ranks размер per rank:

| Решатель | Кейс | Per rank | Зона |
|---|---|---|---|
| OpenFOAM | motorBike 350K cells | **21 875 cells/rank** | ✅ на границе efficient (20K+) |
| OpenRadioss | Yaris Coarse 378K elements | **23 648 elements/rank** | ✅ efficient |
| Code_Aster | perf009 803K dofs | **50 209 dofs/rank** | ✅ комфортная зона |

Tutorial-grade OpenRadioss benchmarks НЕ попадают в efficient zone
при N=16:

| Tutorial | Elements | Per rank |
|---|---|---|
| Football Shot | 1 480 | **92** ❌ ниже порога в **~270×** |
| Tensile Test | ~10 000 | ~625 ❌ |
| Bumper Beam | ~20 000 | ~1 250 ❌ ниже порога в ~20× |

На таком масштабе measurements доминируются MPI overhead, scheduler
placement effect нельзя надёжно измерить. **Использование Yaris Coarse**
(378K elements) обусловлено научно подтверждённой минимальной
гранулярностью distributed FEM workloads.

### Почему N=16 ranks

Выбор N — критический для измеримости эффекта scheduler.

Эмпирические наблюдения из литературы:
- **Xie 2026** на 4 ranks → выигрыш per-rank CPU optimization **3%**
- **Xie 2026** на 16 ranks → тот же подход даёт **20%** (в 7 раз ярче)
- **Queens University 2016** — placement effect на MPI_Allgather: 20-28% на 8+ ranks

На малом N (≤4 ranks) различия между placement-алгоритмами **тонут в шуме**.
На большом N (≥32) compute time резко растёт, бюджет вырастает несоразмерно.

**N=16 — sweet spot**:
- Эффект placement уже значимый (>10% выигрыш ожидается на dense F-graph)
- Не помещается на типичный потребительский CPU (M1: 8 ядер, типовой Xeon: 8-12) — distributed setup оправдан
- Соответствует main scaling experiment Xie 2026
- Compute time на запуск разумный (5-30 мин в зависимости от решателя)

Защита формулируется так:
> «N=16 MPI ranks выбрано как точка где scheduler-эффекты становятся
> measurable per [Xie 2026], демонстрировавшим 20% wall-clock speedup
> per-rank CPU optimization именно на этой шкале. Меньшие N (4-8 ranks)
> не позволяют надёжно дискриминировать placement-алгоритмы из-за шума.»

### Решатели и кейсы

| Решатель | Кейс | Размер | Плотность F | Откуда |
|---|---|---|---|---|
| OpenFOAM | `motorBike` (incompressible/simpleFoam) | ~350K cells | sparse (~0.1) | tutorial в `opencfd/openfoam-default:2306` |
| OpenRadioss | [`Chrysler Neon 1M`](https://openradioss.atlassian.net/wiki/spaces/OPENRADIOSS/pages/47546369/HPC+Benchmark+Models#1M-Element-Neon-model) | 1M elements | medium (~0.3) | официальный HPC benchmark |
| Code_Aster | `perf009` (EDF reference) | 261K nodes, 803K dofs | dense (~0.7) | официальный perf testcase EDF |

Все три — **iconic** в своих коммьюнити (для CFD это motorBike, для
crash-analysis это Neon, для FEM это perf-серия EDF). Не custom mesh
— защищается как «standard reference cases».

### Schedulers

| Имя | Что делает | Роль в эксперименте |
|---|---|---|
| `random` | случайное размещение ranks → nodes | baseline «без topology-awareness» |
| `greedy` | greedy bin-pack по F-графу (online) | state-of-the-art из литературы |
| `mueller-merbach` | offline QAP-approximation | наш предлагаемый алгоритм |

Все три реализованы в `scheduler/algorithms/`.

## Compute budget

Эталонные времена выполнения см. в [BENCHMARKS.md](BENCHMARKS.md).

Один запуск на 16 ranks (зависит от выбранного варианта benchmark'ов):

| Решатель | Time/run (16 ranks) | Заметки |
|---|---|---|
| OpenFOAM motorBike (0.35M cells) | ~6 мин | tutorial-grade, sparse F |
| OpenRadioss Cell Phone Drop (30K) | ~7 мин | tutorial-grade, medium F |
| Code_Aster ssnv128a | ~10 мин | nonlinear+MUMPS, dense F |
| **Среднее (Вариант C — рекомендуется)** | **~7-8 мин** | |

45 запусков × 7-8 мин = **~6 ч чистого compute** + накладные на setup/отладку.

Полный uptime кластера: **~30 часов** включая отладку.

С учётом инфраструктуры (Timeweb VPS 24 phys cores):
- 30 ч × 55 ₽/час = **~1 650 ₽** за весь эксперимент.

## Статистическая методология

### Step 1: проверка нормальности (Shapiro-Wilk)

Для каждой ячейки эксперимента (например, «OpenFOAM + MM, 5 запусков»)
применяем `scipy.stats.shapiro(samples)` → `p-value`.

| p-value | Интерпретация |
|---|---|
| p ≥ 0.05 | данные не отличаются от нормальных → используем параметрические тесты |
| p < 0.05 | отклонение от нормальности → используем непараметрические |

Требует `n ≥ 4`. Поэтому повторов **минимум 5**.

### Step 2: доверительный интервал (95%)

Зависит от Shapiro-Wilk:

| Если нормально (Shapiro p ≥ 0.05) | Если ненормально |
|---|---|
| **Student-t CI**: `mean ± t_{0.975,n-1} × SD/√n` | **Bootstrap CI**: 5000 resamples, 2.5-й/97.5-й перцентиль |
| быстрее, узкие CI | устойчивее к outliers, шире CI |

Реализовано в `experiment/analyze_results.py:ci_for_cell()`.

### Step 3: парные сравнения между scheduler'ами

Для каждого решателя сравниваем три scheduler'а попарно:

| Если данные нормальные | Если ненормальные |
|---|---|
| **paired t-test** (`scipy.stats.ttest_rel`) | **Wilcoxon signed-rank** (`scipy.stats.wilcoxon`) |

Реализовано в `experiment/analyze_results.py:paired_tests()`.

## Burstable QoS (никаких CPU limits)

**Критический constraint** из Xie 2026 ([RELATED_WORK.md](RELATED_WORK.md)):

Если в MPIJob pod spec поставить `limits.cpu` равный `requests.cpu`,
pod попадает в Kubernetes Guaranteed QoS class. Linux CFS bandwidth
controller жёстко ограничивает pod квотой (например, 25 ms из 100 ms
для `limit: 250m`). Когда квота исчерпана — kernel выгружает thread'ы
до следующего периода.

Для **tightly-coupled MPI** это катастрофа: каждый rank ждёт всех
остальных на `MPI_Allreduce` barrier. Если хоть один rank
throttled — **вся** simulation встаёт. Xie квантифицировал эффект:
**78× slowdown** через cascading stalls (35 сек → 2738 сек).

### Правила для нашего MPIJob spec

```yaml
spec:
  containers:
    - resources:
        requests:
          cpu: "1"
          memory: "1Gi"
        # НИКАКИХ limits — Burstable QoS class
```

Это правило применяется ко **всем** worker и launcher pods в MPIJob.
Реализовано в `backend/internal/infrastructure/k8s/simulation_manager.go`.

## Что обеспечивает валидность результатов

| Угроза валидности | Mitigation |
|---|---|
| CPU jitter на burstable/shared CPU | **dedicated CPU** instances (см. [CLUSTER.md](CLUSTER.md)) |
| CPU throttling через CFS hard limits | **никаких CPU limits** в MPIJob spec (см. выше) |
| Network jitter | n=5 повторов на ячейку |
| Различия кластеров между запусками | один кластер, фиксированный config, заранее warmup |
| Auto-correlation последовательных запусков | warmup — первые **2 повтора отбрасываем** |
| Subjective выбор кейсов | три **независимо признанных** benchmark (motorBike / Chrysler Neon / perf009) |
| Cherry-picking результатов | весь сырой CSV публикуется вместе с дипломом |
| Bootstrap nondeterminism | `random_state=42` в `scipy.stats.bootstrap()` (TODO) |

### Target std dev

Xie 2026 на dedicated CPU (c5.xlarge non-burstable) получает **std dev <2%** от среднего. Это наш **KPI воспроизводимости**:

- std dev < 2% → отлично, статистика чистая
- std dev 2-5% → приемлемо, t-test работает
- std dev > 5% → подозрение что кластер шумит, debug нужен

## Что в коде и что ещё нужно

| Компонент | Где | Состояние |
|---|---|---|
| Запуск 45 jobs через backend | `experiment/run_benchmark.py` | написан, не тестировался на реальном кластере |
| Парсинг mpiP отчётов | `experiment/parse_mpip.py` | написан, тестировался на smoke-отчёте |
| Shapiro-Wilk + CI + paired tests | `experiment/analyze_results.py` | написан, не запускался на реальных данных |
| Bootstrap random_state | `analyze_results.py:ci_for_cell()` | **TODO** добавить `random_state=42` |
| Warmup-отбрасывание (WARMUP_REPS=2) | оба скрипта | реализовано |

## Альтернативные планы (если время поджимает)

| План | Запусков | Compute | Защита |
|---|---|---|---|
| **Базовый 3×3×5 ⭐** | 45 | 22.5 ч / 11 ч parallel | strong, все стандарты соблюдены |
| Stratified 3+3+7 reps (sparse/med/dense) × 3 sched | 39 | 19.5 ч / 10 ч parallel | strong, defensible как «adaptive design» |
| Pilot 3×3×3 | 27 | 13.5 ч / 7 ч parallel | weaker — Shapiro на грани, t-test marginal |

По умолчанию идём **базовым 3×3×5 = 45**. Stratified — если хотим
сэкономить compute без потери силы на dense (где H₁ ожидается значимой).
