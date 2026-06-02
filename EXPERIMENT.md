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
```

| Размерность | Сколько | Зачем |
|---|---|---|
| Решатели | 3 | три точки на оси плотности F-графа — для **формы** кривой Δ(ρ), не направления |
| Schedulers | 3 | random (baseline = «kube-scheduler default») + greedy (state-of-the-art) + MM (наше) |
| Повторов | 5 | минимум для Shapiro-Wilk normality test и paired t-test с разумной power |

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

Один запуск на 8 ranks:

| Решатель | Time/run |
|---|---|
| OpenFOAM motorBike | ~10 мин |
| OpenRadioss Chrysler Neon 1M | ~45 мин |
| Code_Aster perf009 | ~30 мин |
| **Среднее** | **~30 мин** |

45 запусков × 30 мин = **22.5 ч sequential**.

С 18-нод кластером (2× parallelism) = **~11 ч** — за ночь.

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
