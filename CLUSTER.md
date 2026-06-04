# Кластер — план развёртывания

Состояние от 2026-06-04. **Финальный выбор**: Vast.ai EPYC 9654 (m:42009).

## TL;DR

| Параметр | Значение |
|---|---|
| Платформа | **Vast.ai** (cloud GPU/CPU marketplace) |
| Offer ID | **m:42009** (Тайвань, host:51010) |
| MPI ranks per job | **16** (main) + **32** (scaling demo для Yaris) |
| Машина | **AMD EPYC 9654** (Genoa, 4-е поколение, 2022) |
| Cores available | **384/384 exclusive** (≈192 phys cores, 2-socket) |
| RAM | **515/515 GB exclusive** |
| Storage | 4127 GB Intel SSDPF2K (NVMe enterprise, 6.6 GB/s) |
| Network | 1 Gbps up / 2 Gbps down, 498 ports |
| Reliability | 99.76% verified |
| Multi-AZ | через **tc qdisc эмуляцию** (3 логические зоны 8+8+8 нод) |
| Multi-rank | 16 (3 зоны 6+5+5) + 32 (3 зоны 12+10+10) |
| Оплата | **почасовая** $1.103/hr (no minimum) |
| Реальное uptime | ~44 часа (compute + setup + smoke + buffer) |
| **Бюджет** | **~$50 (≈4 600 ₽)** ($30 + $20 запас) |

## Эволюция плана

Зафиксирована для будущей памяти и для главы 3 «Архитектура эксперимента».

### Шаг 1: 8 ranks, multi-region managed K8s

**Идея**: 8 ranks × 3 разных регионов России → multi-region кластер для real
cross-region latency.

**Что выяснилось**: managed K8s ни у одного провайдера не поддерживает
multi-region (etcd consensus требует low latency между master нодами).
Это **глобальная** проблема, не Timeweb-специфичная.

### Шаг 2: 8 ranks, Selectel multi-AZ Москва

**Идея**: Selectel даёт multi-zone managed K8s — 3 разных датацентра в
Москве (ru-6a/b/c), latency 0.5-1 ms cross-AZ. ~6 200 ₽ за 40 часов.

**Что выяснилось**: на 8 ranks эффект placement слабый (Xie 2026 показал
3% на 4 ranks vs 20% на 16 ranks). 8 ranks помещается на любой
современный CPU → distributed setup не оправдан.

### Шаг 3: переход на 16 ranks

**Обоснование**: Xie 2026 main scaling experiment = 16 ranks. На 16 ranks
эффект scheduler в 7 раз ярче чем на 4 ranks. 16 ranks не помещается на
типичный потребительский CPU → distributed обязателен.

### Шаг 4: VPS с 24 phys cores + kind + tc qdisc

**Прозрение**: если эмулируем multi-AZ через `tc qdisc`, то многонодовый
кластер не обязателен — достаточно одного большого VPS с 24+ физическими
ядрами + kind + cgroup cpuset + tc qdisc.

**Кандидаты проверены**:
- **Timeweb Dedicated CPU 24+96 GB** — изначально ~2 800 ₽ за 40 часов, но
  выяснилось «недостаточно ресурсов в Москве» в момент заказа.
- **HostKey ru.v4-epyc3** — overkill (32 cores + 768 GB) за 99 000 ₽/мес.
  Reality check: их «почасовая оплата» = минимум 1 месяц, деньги не
  возвращаются. Не подходит.
- **Cloud.ru Evolution Bare Metal** — pay-as-you-allocate ~13 491 ₽/мес,
  но bare metal через enterprise канал (требует менеджера / юр.лицо).
- **ITsoft Ryzen 9 9950X** — 32 500 ₽/мес, только monthly billing,
  гарантированно работает но дорого для 40 часов.
- **Selectel Configurator** — daily billing, 100+ конфигураций.

Все провайдеры либо «нет ресурсов», либо «минимум месяц», либо «через
менеджера». Тупик для **точечной** 40-часовой аренды.

### Шаг 5: Vast.ai — финальный выбор ⭐

**Прозрение**: Vast.ai = peer-to-peer marketplace для GPU/CPU rental,
работает по реальной почасовке без минимума. У Mark уже есть $30 баланс.

**Что доступно**: после фильтров (verified, vms_enabled, reliability ≥95%)
найден **m:42009** — экстремально мощная машина в Тайване:
- AMD EPYC 9654 (Genoa, 96 cores per chip, 2-socket = 192 phys cores)
- 515 GB RAM
- 4 TB Intel enterprise NVMe
- Verified, 99.76% reliability
- $1.103/hr

**Почему именно эта**:
1. **Exclusive machine** — 384/384 CPU и 515/515 RAM полностью наши, нет
   noisy neighbors (критично для measurements).
2. **Современный CPU** (Zen 4 Genoa, 2022) — в 2× быстрее старых Xeon.
3. **VM mode поддерживается** — можем запускать privileged операции
   (kind cluster, tc qdisc, cgroup cpuset).
4. **Hourly billing** без минимума.
5. **Бюджет**: $50 (с $20 запасом) укладывается в реалистичные затраты.

## Финальная конфигурация Vast.ai

### Template — официальный VM-шаблон Vast.ai

🔴 **Грабли (2026-06-04)**: НЕ кастомный шаблон с `VM Image Path: vastai/kvm:...`
и `Launch Mode: Interactive shell server` — это запускает kvm-образ как обычный
**Docker-контейнер** (непривилегированный: нет NET_ADMIN, `tc netem → Operation
not permitted`, cgroup read-only) → multi-AZ невозможен. По docs.vast.ai
Docker-инстансам нельзя выдать cap-add/privileged (доступны только env/hostname/
ports). Нужен **настоящий VM-режим**.

✅ Брать готовый шаблон **«Ubuntu 22.04 VM»** (Templates → VM):

| Параметр | Значение |
|---|---|
| Template | **«Ubuntu 22.04 VM»** (официальный, не кастомный) |
| Launch Mode | **SSH** (у VM единственный режим) |
| Disk Space | **130 GB** |
| Extra Filters | `vms_enabled=true` (+ verified, reliability2>=0.95) |
| SSH key | добавить в Account **до** создания инстанса |

VM = своё ядро → root + NET_ADMIN (`tc netem`), nested containers (kind),
cgroup cpuset, /dev/kvm. Проверка перед деплоем (должно вывести `VM OK`):

```bash
ssh -p <port> root@<ip> 'ls /dev/kvm && [ "$(systemd-detect-virt)" != docker ] \
  && tc qdisc add dev lo root netem delay 1ms && tc qdisc del dev lo root \
  && echo "VM OK"'
```

### Выбранный offer

```
m:42009, host:51010
AMD EPYC 9654 @ 2.4 GHz (Genoa)
384/384 CPU exclusive (192 phys cores, 2-socket)
515/515 GB RAM exclusive
4127 GB NVMe (Intel SSDPF2K, 6.6 GB/s read)
1 Gbps up / 2 Gbps down, 498 ports
verified, reliability 99.76%
Max duration: 24 days
Location: Taiwan
Price: $1.103/hr
```

## Multi-AZ через tc qdisc внутри VM

После запуска VM делаем kind cluster с 24 (для N=16) или 36 (для N=32)
нод-контейнерами, разбитых на 3 логические зоны.

### Для N=16 (main experiment)

```
24 нод-контейнера = 16 worker + 1 launcher + 7 запас (choice space):

8 нод "zone-a" → cpuset cores 0-7,    latency 0 ms (внутри зоны)
8 нод "zone-b" → cpuset cores 8-15,   latency 5 ms к zone-a/c
8 нод "zone-c" → cpuset cores 16-23,  latency 10 ms к zone-a/b
```

### Для N=32 (scaling demo на Yaris)

```
36 нод-контейнера = 32 worker + 1 launcher + 3 запас:

12 нод "zone-a" → cpuset cores 0-11,   latency 0 ms
12 нод "zone-b" → cpuset cores 12-23,  latency 5 ms
12 нод "zone-c" → cpuset cores 24-35,  latency 10 ms
```

192 phys cores с большим запасом покрывают обе конфигурации.

## Защита (формулировка для дипломки)

> «Эксперимент проведён на dedicated виртуальной машине под управлением
> KVM на хосте с процессором AMD EPYC 9654 (Genoa, 4-е поколение, 2022),
> предоставляющим 192 физических ядра и 515 GB RAM в exclusive режиме без
> shared resources. Multi-AZ топология эмулируется через Linux Traffic
> Control (`tc qdisc netem`) с тремя логическими зонами и controlled
> latency contrast 5-10 мс. Этот подход обеспечивает reproducible
> measurements, не зависящие от изменений в cloud infrastructure provider —
> стандартная академическая методология для исследования scheduling
> алгоритмов.»

## Setup pipeline

После RENT и provisioning Vast.ai VM:

1. SSH в VM (~3-5 мин wait для VM provisioning)
2. Запуск `scripts/cluster-up.sh`:
   - Установка Docker + kind + kubectl
   - Setup swap 32 GB (safety net для MUMPS)
   - kind cluster config: 24-36 нод-контейнеров с node labels по зонам
   - cgroup cpuset isolation (1 phys core на ноду)
   - tc qdisc на bridge interface (multi-AZ эмуляция)
   - Install MPI Operator + scheduler extender + backend
3. Smoke test первого MPIJob через backend (~30 мин)
4. Запуск `experiment/run_benchmark.py` — полный прогон 54 + 18 jobs
5. Скачать mpiP отчёты + результаты, удалить VM

## Бюджет

| Этап | Время | Стоимость |
|---|---|---|
| Setup + smoke | 3-4 ч | $3.3-4.4 |
| Compute (N=16, 54 runs) | 27 ч | $29.8 |
| Compute (N=32, 18 runs Yaris) | 12 ч | $13.2 |
| Debugging buffer | 2-3 ч | $2.2-3.3 |
| **Total ожидаемое** | **~44 ч** | **$48-50** |

⚠️ Из $30 budget хватит на **27 часов** — превышение **~$20**.

**Рекомендация**: положить дополнительные **$20** на Vast.ai счёт
перед стартом для безопасности (top up до ~$50).

Если бюджет строго $30 — отказаться от N=32 scaling demo и оставить
только N=16 main experiment (~$30 укладывается).

## TODO

1. **Mark**: положить $20 запаса на Vast.ai (опционально, для buffer)
2. **Mark**: нажать RENT на m:42009
3. **Mark**: дождаться provisioning (~3-5 мин), копировать SSH details
4. **Я**: написать `scripts/cluster-up.sh` под VM EPYC 9654
5. **Я**: deploy + smoke test первого MPIJob
6. **Я**: запуск полного 54-run эксперимента (N=16) + 18-run scaling
   (N=32 для Yaris)
7. **Я**: анализ результатов (t-test, bootstrap CI, one-sided Wilcoxon),
   таблицы и графики для главы 5 диплома

## Открытые вопросы

- [x] Какая платформа? → **Vast.ai m:42009 EPYC 9654**
- [x] N=16 или N=32? → **Сценарий B: N=16 main + N=32 scaling для Yaris**
- [ ] Latency contrast 5/10 мс или экспериментировать с другими
      значениями (10/30/100 мс) для H2 гипотезы?
- [ ] Mark кладёт $20 запаса или стартуем строго с $30?
