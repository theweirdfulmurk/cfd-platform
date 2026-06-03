# Кластер — план развёртывания

Состояние от 2026-06-03. Обновлено под N=16 ranks и dedicated phys cores.

## TL;DR

| Параметр | Значение |
|---|---|
| MPI ranks per job | **16** (обосновано в [EXPERIMENT.md](EXPERIMENT.md)) |
| Минимум физических ядер | **24** (1 rank = 1 phys core + запас на launcher и scheduler choice space) |
| Multi-AZ | через **tc qdisc эмуляцию** (3 логические зоны 8+8+8) |
| Тип CPU | **dedicated physical cores**, не HT-threads |
| Оплата | **почасовая** (платим за uptime, не за месяц) |
| Реальное uptime | ~40 часов (3 дня cycle с отладкой) |

## Эволюция плана

Зафиксирована для будущей памяти и для главы 3 «Архитектура эксперимента».

### Шаг 1: 8 ranks, multi-region managed K8s

**Идея**: 8 ranks × 3 разных регионов России → multi-region кластер для real cross-region latency.

**Что выяснилось**: managed K8s ни у одного провайдера не поддерживает multi-region (etcd consensus требует low latency между master нодами). Это **глобальная** проблема, не Timeweb-специфичная.

### Шаг 2: 8 ranks, Selectel multi-AZ Москва

**Идея**: Selectel даёт multi-zone managed K8s — 3 разных датацентра в Москве (ru-6a/b/c), latency 0.5-1 ms cross-AZ.

**Цена**: ~6 200 ₽ за 40 часов uptime (18 нод 2 vCPU + 8 GB).

**Что выяснилось**:
- На 8 ranks эффект placement слабый (Xie 2026 показал 3% на 4 ranks vs 20% на 16 ranks)
- 8 ranks помещается на любой современный CPU → distributed setup не оправдан
- Multi-AZ latency 0.5-1 ms даёт слабый contrast (5×) — выигрыш placement скрывается в шуме

### Шаг 3: переход на 16 ranks

**Обоснование**:
- **Xie 2026**: 16 ranks на 4 worker nodes = их главный заявленный результат, 20% wall-clock speedup
- **На 16 ranks эффект scheduler в 7 раз ярче** чем на 4 ranks (proven empirically)
- 16 ranks **не помещается** на типичную потребительскую машину (M1: 8 cores, типичный CPU: 8-12 cores) → distributed обязателен
- Защита сильнее: «scaling experiment в production-relevant range»

**Что меняется в плане**:
- Узлов нужно минимум **16 worker + 1 launcher = 17**
- Для качественной дискриминации алгоритмов scheduler — нужно **24+ узлов** (8 свободных = реальный choice space)

### Шаг 4: VPS с 24 phys cores vs managed K8s

**Прозрение**: если эмулируем multi-AZ через `tc qdisc`, то **многонодовый кластер не обязателен** — достаточно одного большого VPS с 24+ физическими ядрами + kind + cgroup cpuset + tc qdisc.

Преимущества:
- В **3.5 раза дешевле**
- **Физически чище** — 1 rank = 1 phys core (без HT-sharing)
- Полный контроль над tc qdisc настройками
- Воспроизводимость (всё на одной машине)

Недостатки:
- Self-installed kind setup (+2-3 часа в первый день)
- На защите чуть менее впечатляюще («kind + tc» vs «реальный multi-AZ»)

## Два варианта инфраструктуры

### Вариант A — VPS с 24 phys cores + kind + tc ⭐

**Провайдер**: [Timeweb Cloud Dedicated CPU](https://timeweb.cloud/blog/novaya-linejka-tarifov-dedicated-cpu).

**Конфигурация**:
- 24 dedicated physical cores (не HT-threads)
- 64 GB RAM (2-3 GB на «ноду»-контейнер × 24 + system)
- 200 GB NVMe (наши 4 образа × 24 = много места под Docker слои)
- Локация: Москва или СПб
- Цена: 1 250 ₽/мес за phys core + базовая конфигурация
- **Итого**: ~40 000 ₽/мес или **~55 ₽/час**

**Setup** (~2-3 часа первый раз):
1. Заказ VPS, Ubuntu 22.04
2. Docker, kind, kubectl
3. kind cluster config: 24 ноды-контейнера с node labels `topology.kubernetes.io/zone={a,b,c}`
4. cgroup cpuset: каждый контейнер закреплён на 1 phys core
5. tc qdisc на bridge interface: latency 5-10 ms между ноды разных зон
6. Install MPI Operator + наш scheduler extender
7. Deploy backend + frontend

**Multi-AZ через tc qdisc**:
```
8 нод "zone-a" → cpuset cores 0-7,    нет delay
8 нод "zone-b" → cpuset cores 8-15,   tc qdisc 5 ms к zone-a и zone-c
8 нод "zone-c" → cpuset cores 16-23,  tc qdisc 10 ms к zone-a и zone-b
```

**Цена 40 часов uptime**: ~2 200 ₽

**Защита** (формулировка для дипломки):
> «Эксперимент проведён на однородной vCPU-инфраструктуре (24 dedicated physical cores) с эмуляцией multi-AZ топологии через Linux Traffic Control (`tc qdisc netem`). Этот подход обеспечивает controlled и reproducible variability latency contrast, не зависящую от изменений в cloud infrastructure provider — стандартная академическая методология для исследования scheduling алгоритмов.»

### Вариант B — Selectel managed K8s 24 нод multi-AZ

**Конфигурация**:
- 24 cloud node × (2 vCPU dedicated + 4 GB RAM)
- 3 группы по 8 нод в зонах ru-6a, ru-6b, ru-6c
- Multi-zone master (3 master nodes в 3 AZ), SLA 99.98%
- Latency cross-AZ ~0.5-1 ms реальная

**Setup** (~30 мин):
1. Создать кластер через Selectel UI
2. Скачать kubeconfig
3. Install MPI Operator + наш scheduler extender
4. Deploy backend + frontend
5. tc qdisc можно опционально добавить для усиления contrast

**Multi-AZ — реальный**:
- 3 разных датацентра физически
- Latency 0.5-1 ms реальная
- Можно добавить tc qdisc сверху для усиления (опционально)

**Цена 40 часов uptime**: ~8 000 ₽ (с учётом 2 vCPU на ноду через HT, не 24 phys cores)

**Защита**:
> «Эксперимент проведён на 24-нодном Kubernetes-кластере в 3 разных датацентрах Москвы (Selectel ru-6a/b/c) с реальным multi-AZ latency contrast.»

## Сравнительная таблица

| | Вариант A: VPS + kind | Вариант B: Selectel managed |
|---|---|---|
| Цена 40 ч uptime | **~2 200 ₽** | ~8 000 ₽ |
| Physical cores per node | **1 phys core (1:1)** | HT-shared (через 2 vCPU = 1 phys + 1 HT) |
| Multi-AZ | tc qdisc эмуляция | реальный + опционально tc |
| Setup time (первый раз) | 2-3 часа | 30 мин |
| Контроль | полный | ограничен managed |
| Воспроизводимость | очень высокая (1 машина) | высокая |
| Защита (формулировка) | «kind + tc qdisc, standard academic» | «real multi-AZ в 3 ДЦ» |
| Std dev (ожидаем) | <2% (real phys cores) | <2-3% (dedicated 2 vCPU) |

## Рекомендация

**Вариант A — Timeweb VPS 24 phys cores + kind + tc**.

Причины:
1. **3.5× дешевле** (1 800 ₽ экономии vs B)
2. **Физически чище**: 1 rank = 1 phys core, никакого HT-sharing
3. **Большая свобода** настройки tc qdisc — можно testing разные latency contrast values
4. **Воспроизводимость** для других researchers выше — стандартный setup
5. Защищается **standard academic** методологией

Когда брать Вариант B:
- Если защита очень требует «real multi-AZ» (научрук специально просит)
- Если не хочется делать self-installed kind

## TODO для Варианта A (Timeweb VPS)

1. **Mark**: регистрация на Timeweb Cloud (уже сделано)
2. **Mark**: заказ VPS Dedicated CPU 24 phys cores + 64 GB RAM + 200 GB NVMe в Москве
3. **Mark**: SSH доступ — передать ключ или через .env локально
4. **Я**: написать setup script (`scripts/cluster-up.sh`):
   - Ubuntu setup
   - Docker, kind, kubectl install
   - kind cluster config с 24 нодами по 3 зонам
   - cgroup cpuset скрипт
   - tc qdisc setup скрипт
   - Install MPI Operator
   - Deploy наш scheduler + backend
5. **Я**: smoke test первого MPIJob через backend на этом кластере
6. **Я**: документация по добавлению tc qdisc вариаций для запуска benchmark

## TODO для Варианта B (Selectel managed K8s)

1. **Mark**: создать кластер через Selectel UI (3 группы 8+8+8 в ru-6a/b/c)
2. **Mark**: скачать kubeconfig
3. **Я**: install MPI Operator + наш scheduler
4. **Я**: deploy backend + frontend
5. **Я** (опционально): tc qdisc сверху для усиления contrast если нужно

## Что меняется в коде/конфигах при N=16

| Файл | Изменение |
|---|---|
| `backend/.../simulation_manager.go` | ✅ ничего — `NumProcs` уже параметр |
| `frontend/.../CreateSimulation.tsx` | default `np=4` → `np=16` |
| `k8s/examples/openfoam-mpijob.yaml` | `replicas: 8` → `replicas: 16` |
| `experiment/run_benchmark.py` | `NUM_PROCS = 8` → `NUM_PROCS = 16` |
| BENCHMARKS.md | пересчитать compute time для 16 ranks |
| EXPERIMENT.md | заменить N=8 на N=16 в обосновании |

## Открытые вопросы

- [ ] Какой вариант берём — A или B?
- [ ] Если A — подтвердить что Timeweb принимает оплату удобной картой
- [ ] Тестовый запуск отдельного VPS на 1 час чтобы проверить что dedicated CPU действительно реализуется как заявлено
- [ ] Решить про вариабельность tc qdisc latency (5, 10, 30 ms?) для дополнительной серии запусков

## Финальный выбор

**Жду подтверждения Mark.**
