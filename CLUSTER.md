# Кластер — план развёртывания

Промежуточное состояние от 2026-06-02. Есть открытые моменты.

## Выбор провайдера: Timeweb Cloud

| Параметр | Значение |
|---|---|
| Managed Kubernetes | да |
| Локации | СПб / Москва / Новосибирск — **3 разных города** |
| Cross-region latency | ~5-30 ms (реальная multi-region) |
| Intra-region latency | <1 ms |
| Bimodal latency contrast | ~50-100× (нужно для гипотезы) |
| Оплата | рубли, российская карта |
| Документация | базовая, на русском |

## Топология

| Регион | Узлов | Назначение |
|---|---|---|
| СПб | 6 | AZ-A |
| Москва | 6 | AZ-B |
| Новосибирск | 6 | AZ-C |
| **Итого** | **18** | 2× параллелизм для 45 запусков |

Worker spec (с учётом требований из [RELATED_WORK.md](RELATED_WORK.md)):
- **Dedicated CPU** (не burstable / не shared) — критично для воспроизводимости
- 2 vCPU dedicated, 4 GB RAM, 20 GB SSD
- ~1 500-2 000 ₽/мес за ноду

**Почему dedicated CPU обязательно:** Xie 2026 (arXiv:2603.22691)
показал что на burstable instances (AWS t3.xlarge с shared CPU)
разброс времени выполнения **до 2×** на одну и ту же задачу.
На non-burstable (c5.xlarge с dedicated CPU) — **std dev <2% от среднего**.
Воспроизводимость экспериментов требует dedicated CPU.

## Стоимость (оценка)

| Статья | Сумма/мес |
|---|---|
| 18 worker нод (2 vCPU dedicated, 4 GB) | ~27 000-36 000 ₽ |
| Master (Base tier) | ~2 000 ₽ |
| NFS ReadWriteMany storage (~50 GB) | ~500 ₽ |
| Egress (mpiP отчёты килобайтами) | ~0 ₽ (бесплатные 100 GB) |
| **Итого** | **~30 000-39 000 ₽/мес** |

**Если бюджет жмёт** — можно сократить до 12 нод (4+4+4) с
**sequential** прогоном 45 jobs за 22 часа вместо 11. Тогда:

| Альтернатива | Узлов | Compute | Стоимость/мес |
|---|---|---|---|
| Минимум | 9 (3+3+3) sequential | 22.5 ч | ~15-18 тыс ₽ |
| Средне | 12 (4+4+4) sequential | 22.5 ч | ~20-24 тыс ₽ |
| **Оптимум** ⭐ | **18 (6+6+6) parallel 2×** | **~11 ч** | **~30-39 тыс ₽** |

После окончания эксперимента — сносим project, оплата прекращается.
Реально используем 1 месяц аренды.

## TODO для подъёма кластера

1. **Mark**: регистрация на https://timeweb.cloud, привязать карту
2. **Mark**: получить API ключ в личном кабинете
3. **Mark**: передать API ключ (через `.env` локально, не коммитить)
4. **Я**: написать bash/terraform скрипт для развёртывания:
   - Создать managed K8s cluster
   - Node groups по регионам (4 + 4 + 4) с node labels `topology.kubernetes.io/zone={a,b,c}`
   - StorageClass с ReadWriteMany (NFS provisioner или их встроенный)
   - Установить MPI Operator (`mpi-operator v2beta1`)
   - Установить наш scheduler extender
   - Развернуть backend + frontend
5. **Я**: задеплоить тестовый MPIJob и убедиться что pods реально размещаются в разных регионах
6. **Я**: настроить scheduler config с 3 профилями (random / greedy / MM)

## Критические constraints для MPIJob deployment

Из обзора литературы (особенно Xie 2026):

1. **Никаких CPU limits в Pod spec**. Только `requests`.
   Hard limits через CFS bandwidth controller вызывают **78× slowdown**
   на tightly-coupled MPI через cascading stalls в `MPI_Allreduce`.
   Подробности: [EXPERIMENT.md](EXPERIMENT.md), секция «Burstable QoS».

2. **NFS ReadWriteMany обязательно**. У всех ranks должен быть доступ
   к одной shared simulation directory. PVC с ReadWriteOnce не подойдёт.
   Timeweb даёт managed NFS либо ставим Longhorn сами.

3. **Один rank на одну ноду** (1 MPI rank = 1 vCPU = 1 worker pod
   с antiAffinity по nodes). Без oversubscription, без HT-эффектов.

## Открытые вопросы (Mark, дозаполни перед регистрацией)

- [ ] Уточнить какой именно тариф Timeweb даёт dedicated CPU
      (Standard / Premium / CPU-optimized?)
- [ ] Есть ли у Timeweb HPA / spot для economy на debugging stages?
- [ ] Bandwidth между регионами — bottleneck для MPI? Smoke с iperf3 первым делом
- [ ] *доп. вопросы Mark*: 

## Что ещё нужно решить

- Способ хранения CSV-результатов: на NFS PVC или скачать локально через `kubectl cp`?
- Бюджет на отладку: реально использовать ~50% запасных запусков (debug runs)
