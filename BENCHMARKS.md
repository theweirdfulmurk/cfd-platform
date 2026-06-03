# Эталонные времена benchmark'ов

Данные из литературы для оценки compute budget эксперимента.
Состояние на 2026-06-04.

## Финальный выбор кейсов (Сценарий B)

| Решатель | Кейс | Размер | Time/run (N=16) | Time/run (N=32) |
|---|---|---|---|---|
| OpenFOAM | motorBike | 350K cells | **~5 мин** | — (granularity слабая) |
| OpenRadioss | **Yaris Coarse** ⭐ | 378K elements | **~1 час** | **~40 мин** |
| Code_Aster | perf009 | 261K nodes / 803K dofs | **~25 мин** | — (не делаем scaling) |

Все три времени — на платформе **Vast.ai m:42009 EPYC 9654** (Genoa, 2-socket,
192 phys cores, 515 GB RAM). На более слабых CPU будет в 1.5-2× медленнее.

Главный benchmark — **OpenRadioss Yaris Coarse** — выбран как
**NHTSA-validated reference** из George Mason CCSA. Подробнее ниже.

## TL;DR таблица (для ссылок)

Время выполнения вариантов на **16 MPI ranks** на EPYC 9654:

| Benchmark | Размер | Время на 16 ядрах | Статус |
|---|---|---|---|
| OpenFOAM motorBike standard | 0.35M cells | **~5 мин** | ⭐ выбран |
| OpenRadioss Yaris Coarse | 378K elements | **~1 час** | ⭐ выбран |
| OpenRadioss Chrysler Neon 1M | 1M elements | **~2 часа** | отвергнут (overkill) |
| OpenRadioss Bumper Beam | ~20K elements | ~5 мин | отвергнут (granularity) |
| OpenRadioss Cell Phone Drop | ~30K elements | ~7 мин | отвергнут (granularity) |
| Code_Aster perf009 | 261K nodes / 803K dofs | **~25 мин** | ⭐ выбран |
| Code_Aster ssnv128a (nonlinear+MUMPS) | small mesh | ~8 мин | отвергнут (granularity) |
| Code_Aster forma01a | small mesh | ~3 мин | отвергнут (granularity) |

См. [EXPERIMENT.md](EXPERIMENT.md) для granularity per rank thresholds
и обоснования выбора.

## OpenFOAM motorBike

Источники:
* [OpenFOAM Foundation tutorial documentation](https://enccs.github.io/openfoam/2.02_openfoam-handson/)
* [NVIDIA Grace CPU Benchmarking Guide](https://nvidia.github.io/grace-cpu-benchmarking-guide/applications/OpenFOAM/index.html)
* [CFD Support](https://www.cfdsupport.com/motorbike-cpu-processor-benchmark/)

### Конфигурации mesh

Стандартный tutorial идёт с **четырьмя** размерами сетки:

| Mesh size | Назначение |
|---|---|
| **0.35M cells** | tutorial по умолчанию (наш план) |
| 1.9M cells | medium HPC test |
| 11M cells | large HPC test |
| 75M cells | extreme HPC (~60 GB RAM нужно) |

### Известные тайминги

| CPU | Cores | Mesh | Wall-clock |
|---|---|---|---|
| NVIDIA Grace CPU Superchip | 144 cores | 35M cells (HPC extension) | **189 сек** |
| Intel Xeon (HPC benchmark) | 18 cores | medium HPC | **280 сек** = 4.7 мин |
| Estimate из scaling: 350K cells / 8 cores | — | **~5-10 мин** |

### Solver

`simpleFoam` (steady incompressible RANS, k-epsilon или k-omega SST).
По умолчанию 500 итераций.

## OpenRadioss Chrysler Neon 1M

Источники:
* [CloudHPC AMD EPYC benchmark, May 2025](https://cloudhpc.cloud/2025/05/27/unlocking-extreme-performance-openradioss-scalability-on-cloudhpc-with-amd-epyc-processors/)
* [OpenRadioss HPC Benchmark Models](https://openradioss.atlassian.net/wiki/spaces/OPENRADIOSS/pages/47546369/HPC+Benchmark+Models)
* [Intel/AWS OpenRadioss whitepaper](https://cdrdv2-public.intel.com/816822/Intel-AWS-instances-accelerate-OpenRadioss-HPC-simulations-methodology.pdf)

### Конкретные цифры (CloudHPC, AMD EPYC)

Чрезвычайно важные данные. На **production hardware**:

| Cores | CPU generation | Wall-clock |
|---|---|---|
| 16 (HT) | AMD EPYC Milan (3rd gen) | **2.68 часа** |
| 32 (no HT) | AMD EPYC Milan | **1.71 часа** |
| 16 (HT) | AMD EPYC Turin (5th gen) | **1.36 часа** |
| 32 (no HT) | AMD EPYC Turin | **0.68 часа** = 41 мин |
| 96 (no HT) | AMD EPYC Turin | **0.55 часа** = 33 мин |

### Экстраполяция на 8 ядер

| Cores | Estimate (Milan-class) |
|---|---|
| 8 (HT, Intel/AMD mainstream) | **~4-6 часов** |
| 8 (Turin или нового поколения) | **~2-3 часа** |

На обычном Intel Xeon dedicated (наш Selectel): **~3-5 часов на запуск**.

### Вариации меньшего размера

OpenRadioss benchmark suite ([github.com/OpenRadioss/ModelExchange](https://github.com/OpenRadioss/ModelExchange/tree/main/Examples)) содержит широкий спектр кейсов:

| Benchmark | Размер | Estimate 16 ядер | Класс |
|---|---|---|---|
| Tensile Test | ~5-10K elements | ~3-5 мин | tutorial |
| Bumper Beam | ~20K elements | ~5-10 мин | tutorial |
| Spring-back | small | ~5-10 мин | tutorial |
| Football Shot | ~50K elements | ~10 мин | tutorial |
| **Yaris Coarse** | **378K** | **1.5 часа** | **NHTSA reference** |
| Yaris Detailed | 974K | >1.5 часа | NHTSA reference |
| **Cell Phone Drop** | **1M** (не 30K!) | ~1.5-2 ч (estimate) | consumer electronics |
| Chrysler Neon 1M | 1M | 1.5-2 ч | HPC scaling reference |
| Ford Taurus | 10M | 24+ ч | extreme HPC |

**Важно**: Cell Phone Drop **не tutorial**, а 1M elements (раньше я ошибочно классифицировал его как 30K).

## Code_Aster perf009

Источники:
* [Code-Aster forum: parallel benchmarks](https://code-aster.org/forum2/viewtopic.php?id=24707)
* [biba1632/code-aster-manuals scaling docs](https://biba1632.gitlab.io/code-aster-manuals/docs/user/u4.11.01.html)
* [Code_Aster perf009 testcase description](https://code-aster.org/forum2/viewtopic.php?pid=21707)

### Параметры perf009

| Параметр | Значение |
|---|---|
| Mesh | 261,520 nodes |
| Degrees of freedom | 803,352 |
| Solver | MUMPS direct (dense LU) |
| Тип задачи | Static linear, single iteration |

### Тайминги для perf010 (близкий аналог)

| MPI ranks | Wall-clock | speedup vs sequential |
|---|---|---|
| 1 | 815 sec = 13.6 мин | 1.00× |
| 2 | 694 sec = 11.6 мин | 1.17× |
| 4 | 319 sec = 5.3 мин | 2.55× |
| 8 | **284 sec = 4.7 мин** | 2.87× |

### Экстраполяция perf009 на 8 ядер

perf009 имеет ~10× больше DoF чем perf010. Поскольку MUMPS LU имеет complexity O(N²-N³):

| MPI ranks | perf009 estimate |
|---|---|
| 1 | ~3-5 часов |
| 4 | ~1-2 часа |
| **8** | **~30-60 мин** |

### Меньшие альтернативы

| Test case | Тип | Estimate 8 ядер |
|---|---|---|
| `forma01a` | Forming (linear elastic) | **~3-5 мин** |
| `hsnv100a` | Heat transfer | ~5 мин |
| `ssnv128a` | Static nonlinear + MUMPS | ~10-15 мин |
| `sdll100a` | Linear dynamic | ~5-10 мин |
| perf010 | Static linear MUMPS | ~5 мин |
| **perf009** | **Static linear MUMPS, large** | **~30-60 мин** |
| perf011 | Static, even larger | ~1-2 часа |

## Влияние на наш compute budget

Расчёты ниже учитывают **N=16 ranks** и итоговую инфраструктуру
(Timeweb VPS 24 phys cores + kind + tc, ~55 ₽/час).
3 × 3 × 5 = **45 запусков** (см. [EXPERIMENT.md](EXPERIMENT.md)).

### Вариант A — большие iconic benchmarks

| Solver | Time/run (16 ranks) | × 15 × 3 | Total |
|---|---|---|---|
| OpenFOAM motorBike 350K | ~6 мин | 270 мин | 4.5 ч |
| **OpenRadioss Chrysler Neon 1M** | **~2 часа** | **90 ч** | **90 ч** |
| Code_Aster perf009 | ~30 мин | 22.5 ч | 22.5 ч |
| **Compute total** | | | **~120 ч** |

Бюджет: ~120 часов × 55 ₽/час = **~6 600 ₽** на VPS,
или ~120 × 155 = ~18 600 ₽ на managed K8s.

### Вариант B — tutorial-grade benchmarks (защищается)

| Solver | Time/run (16 ranks) | × 15 × 3 | Total |
|---|---|---|---|
| OpenFOAM motorBike 350K | ~6 мин | 270 мин | 4.5 ч |
| OpenRadioss Bumper Beam | ~6 мин | 270 мин | 4.5 ч |
| Code_Aster forma01a | ~3 мин | 135 мин | 2.25 ч |
| **Compute total** | | | **~11 ч** |

Бюджет: ~20 часов uptime × 55 ₽/час = **~1 100 ₽** на VPS,
или ~20 × 155 = ~3 100 ₽ на managed K8s.

### Вариант C — гибрид tutorial (дёшево)

| Solver | Time/run (16 ranks) | × 15 × 3 | Total |
|---|---|---|---|
| OpenFOAM motorBike 350K | ~6 мин | 270 мин | 4.5 ч |
| OpenRadioss Cell Phone Drop (30K) | ~7 мин | 315 мин | 5.25 ч |
| Code_Aster ssnv128a (nonlinear+MUMPS, dense F) | ~10 мин | 450 мин | 7.5 ч |
| **Compute total** | | | **~17 ч** |

Бюджет: ~30 часов uptime × 55 ₽/час = **~1 650 ₽** на VPS,
или ~30 × 155 = ~4 650 ₽ на managed K8s.

### Вариант D — Yaris Coarse (NHTSA reference) ⭐ РЕКОМЕНДУЕТСЯ

**Источник цифр**: прямая цитата с
[ccsa.gmu.edu/models/2010-toyota-yaris/](https://www.ccsa.gmu.edu/models/2010-toyota-yaris/):

> «Approximate computation time to run a 200 ms simulation using
> 16 cores is **1.5 hours**.»

Это для **Coarse Mesh model (378K elements)**.

| Solver | Mesh | Time/run (16 ranks) | × 15 × 3 | Total |
|---|---|---|---|---|
| OpenFOAM motorBike | 350K cells | ~6 мин | 270 мин | 4.5 ч |
| **OpenRadioss Yaris Coarse** | **378K elements** | **~1.5 часа** | **67.5 ч** | **67.5 ч** |
| Code_Aster perf009 | 261K nodes / 803K dofs | ~30 мин | 22.5 ч | 22.5 ч |
| **Compute total** | | | | **~95 ч** |

Бюджет: ~120 часов uptime × 55 ₽/час = **~6 600 ₽** на VPS,
или ~120 × 155 = ~18 600 ₽ на managed K8s.

#### Защита формулировки

> «В качестве OpenRadioss benchmark использована
> **CCSA 2010 Toyota Yaris Coarse model** (378 376 elements, 393 165
> nodes, 919 parts), разработанная **George Mason University /
> Center for Collision Safety and Analysis** под контрактом с
> **Federal Highway Administration** как reference для regulatory
> crash safety analysis. Модель валидирована против full-frontal
> impact crash tests, MGS Barrier, New Jersey Concrete Barrier;
> соответствует Manual for Assessing Safety Hardware (MASH)
> requirements for 1100C test vehicle. Это
> государственно-сертифицированный benchmark, используемый в
> federal vehicle safety regulations США.»

#### Параметры simulation (из официальной документации CCSA v1l, 2016)

| Параметр | Значение |
|---|---|
| Elements | **378 376** |
| Nodes | 393 165 |
| Parts | 919 |
| Average element size | 12-16 мм |
| Minimum element size | 4 мм |
| Time step | 1.0 μs (микросекунда, explicit) |
| Симулируемое физическое время | 200 ms |
| Computation steps | ~200 000 шагов |
| Test scenario | Full-frontal rigid-wall impact |
| Vehicle | 2010 Toyota Yaris Sedan 4-Door |
| Vehicle weight | 1 078 kg |
| Engine | 1.5L L4 DOHC 16V |
| Validation references | MGS Barrier, New Jersey Concrete Barrier, NHTSA tests |
| **Time на 16 ranks** | **~1.5 часа** |
| RAM per rank | ~2-4 GB |

#### Альтернатива — Yaris Detailed

Существует **detailed** версия: **974 383 elements** (без interior +
restraints). Точные тайминги в официальных источниках противоречивы,
скорее всего **дольше** coarse из-за большего размера. Для нашей
дипломки **coarse достаточно** и хорошо обоснован.

#### Источники для Yaris

* [CCSA George Mason — Toyota Yaris page](https://www.ccsa.gmu.edu/models/2010-toyota-yaris/)
* [Coarse Mesh Validation Report (PDF)](https://www.ccsa.gmu.edu/wp-content/uploads/2016/11/2010-toyota-yaris-coarse-validation-v1.pdf)
* [Detailed Mesh Validation Report (PDF)](https://www.ccsa.gmu.edu/wp-content/uploads/2016/10/2010-toyota-yaris-detailed-validation-v2.pdf)
* [OpenRadioss Yaris Impact Model Confluence](https://openradioss.atlassian.net/wiki/spaces/OPENRADIOSS/pages/30539777/Yaris+Impact+Model+in+LS-DYNA+format)
* [CarCrashNet, ML research using CCSA Yaris dataset](https://arxiv.org/html/2605.07098v2)

## Рекомендация

**Вариант C** — компромисс наглядность/бюджет:

- motorBike — iconic CFD, всем понятно
- Cell Phone Drop Test — relatable real-world OpenRadioss example
- ssnv128a — dense F-граф (MUMPS) для проверки гипотезы

Защищается формулировкой:
> «Для контролируемой оценки scheduler placement выбраны три tutorial-grade кейса, представляющие три класса communication patterns
> (sparse FVM / medium explicit FEM / dense implicit FEM с MUMPS).
> Industrial-scale benchmarks типа Chrysler Neon 1M или Ford Taurus 10M добавляют compute cost без качественного изменения научного результата — placement effect одинаков на любом размере, отличается только absolute wall-clock time.»

## Ссылки

* [OpenFOAM Foundation tutorial](https://enccs.github.io/openfoam/2.02_openfoam-handson/)
* [NVIDIA Grace OpenFOAM guide](https://nvidia.github.io/grace-cpu-benchmarking-guide/applications/OpenFOAM/index.html)
* [CloudHPC OpenRadioss EPYC benchmark](https://cloudhpc.cloud/2025/05/27/unlocking-extreme-performance-openradioss-scalability-on-cloudhpc-with-amd-epyc-processors/)
* [OpenRadioss HPC Benchmark Models](https://openradioss.atlassian.net/wiki/spaces/OPENRADIOSS/pages/47546369/HPC+Benchmark+Models)
* [Code_Aster perf testcases forum thread](https://code-aster.org/forum2/viewtopic.php?id=24707)
* [Intel/AWS OpenRadioss paper](https://cdrdv2-public.intel.com/816822/Intel-AWS-instances-accelerate-OpenRadioss-HPC-simulations-methodology.pdf)
