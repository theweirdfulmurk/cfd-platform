# F-graph extraction для трёх решателей

Состояние от 2026-06-03. Детальный план init-container'ов для backend.

Каждый решатель требует своего способа извлечь communication graph
(F[i][j] — сколько данных rank i обменивается с rank j) из его
output. После извлечения backend кладёт `<jobID>.edgelist` в shared
PVC, откуда scheduler extender читает граф для placement-решений.

## OpenFOAM (sparse F-graph)

**Что используем**: встроенный `decomposePar` + парсинг файлов `boundary`.

### Pipeline

```bash
cd /pvc/simulations/<jobID>
decomposePar -force
# теперь имеем processor0/, processor1/, ..., processor15/
# с файлами constant/polyMesh/boundary внутри
extract-openfoam-graph 16 . > /scheduler-graphs/<jobID>.edgelist
```

### Что парсим

Файлы `processor*/constant/polyMesh/boundary` содержат блоки
`processorN` где написано «процессор A граничит с процессором B на
nFaces=1500». Это и есть F[A][B].

### Что готово

| Компонент | Где |
|---|---|
| Парсер boundary | `pkg/decomp/openfoam.go` |
| Edge-list writer | `pkg/decomp/edgelist.go` |
| `decomposePar` | встроен в `opencfd/openfoam-default:2306` base image |

### Что нужно дописать

- Go бинарь `extract-openfoam-graph` (~30 строк) — обёртка над `pkg/decomp.OpenFOAM()`
- Добавить в `docker/openfoam/Dockerfile`: `COPY --from=builder extract-openfoam-graph /usr/local/bin/`

### Риски

| Риск | Mitigation |
|---|---|
| Кейс не использует Scotch (а simple), нет cross-processor faces | проверка перед запуском, error message |
| Filename mismatches (`polyMesh/boundary` vs `polyMesh/boundary.gz`) | проверка обоих вариантов |

Оценка: 30-60 минут разработки + smoke test.

## OpenRadioss (medium F-graph)

**Что используем**: sed-patched Starter генерирует METIS graph file →
`gpmetis` партиционирует → парсим результат.

### Подтверждение source

Source: [`starter/source/spmd/domain_decomposition/grid2mat.F`](https://github.com/OpenRadioss/OpenRadioss/blob/main/starter/source/spmd/domain_decomposition/grid2mat.F), line 2220:

```fortran
IDB_METIS = 0    ! line 2220 — hardcoded default

IF(IDB_METIS == 1) THEN
C write graph for Metis debug                ! intentional debug feature
    WRITE(CHLEVEL,'(I1)') IDDLEVEL
    OPEN(99, file="input.graph"//CHLEVEL, FORM='FORMATTED', RECL=8192)
    write(99, *) nelem, nedges, "010", ncond
    do i = 1, nelem
      write(99, *) iwd(...), adjncy(xadj(i):xadj(i+1)-1)
    end do
    CLOSE(99)
END IF
```

**Подтверждённые факты**:
- Sed-patch (`IDB_METIS = 0` → `1`) — **единственный** способ включить
  graph dump (нет CLI flag, проверил `get_metis_arguments.F90`)
- Имя выходного файла — **`input.graph0`** (для IDDLEVEL=0, top-level mesh)
- Формат — **стандартный METIS** (header `nelem nedges "010" ncond` +
  vertex weights + adjacency), полностью совместим с `gpmetis`
- Это **намеренная debug capability** разработчиков OpenRadioss
  (комментарий `C write graph for Metis debug`), не undocumented hack

### Pipeline

```bash
cd /pvc/simulations/<jobID>
starter_linux64_gf -np 16 -input *.rad
# теперь имеем input.graph0 (METIS format), 378K строк ≈ 40 MB
gpmetis input.graph0 16
# теперь имеем input.graph0.part.16 с partition assignments
extract-radioss-graph input.graph0 input.graph0.part.16 \
    > /scheduler-graphs/<jobID>.edgelist
```

### Что готово

| Компонент | Где |
|---|---|
| Sed-patch в build | `docker/openradioss/Dockerfile` (уже добавил в первой итерации) |
| METIS parser | `pkg/decomp/metis.go` (15 тестов) |
| Edge-list writer | `pkg/decomp/edgelist.go` |

### Что нужно дописать

- Go бинарь `extract-radioss-graph` (~50 строк) — обёртка над `pkg/decomp.METIS()`
- Добавить `metis` (с `gpmetis`) в `docker/openradioss/Dockerfile`:
  ```dockerfile
  RUN dnf install -y metis && dnf clean all
  ```
- Bake binary в runtime stage

### Риски

| Риск | Mitigation |
|---|---|
| Starter падает с включённым `IDB_METIS=1` на реальном Yaris Coarse | проверить на smoke — но это просто extra write, основной flow не трогает |
| Размер `input.graph0` для 378K elements ≈ 40 MB | OK для I/O |
| `gpmetis` в Rocky 9 dnf — какая версия | METIS 5.x с 2013, формат стабильный |
| RAM use Starter на extraction stage | check на smoke |

Оценка: 60-90 минут разработки + smoke test.

## Code_Aster (dense F-graph)

**Что планируем использовать**: `medpartitioner --create-boundary-faces`
из MEDCoupling → парсим joints из MED файлов через MEDLoader Python API.

### Что у нас уже есть

- `scripts/extract_codeaster_graph.py` (Python скрипт) — использует
  MEDLoader API
- Скрипт уже в `docker/codeaster/Dockerfile` (мы добавили его при
  vendor'инге aethereng)

### Pipeline (план)

```bash
cd /pvc/simulations/<jobID>
medpartitioner --create-boundary-faces input_dir --ndomains=16 --output-dir=parts
# теперь имеем parts/part_0.med, parts/part_1.med, ...
python3 /opt/scripts/extract_codeaster_graph.py parts/ \
    > /scheduler-graphs/<jobID>.edgelist
```

### Что нужно проверить (ниже в этом файле)

- `medpartitioner` есть в нашем codeaster image?
- Флаг `--create-boundary-faces` корректный?
- MED файлы после partitioning действительно содержат joints?
- MEDLoader Python API имеет методы `getJoints`, `getCorrespondence`?

Эта часть **рискованнее** двух предыдущих — Python скрипт никогда не
запускался на реальных данных.

### Подтверждение source MEDCoupling

**medpartitioner флаги** ([Salome MEDCoupling Tools doc](https://docs.salome-platform.org/latest/dev/MEDCoupling/developer/tools.html)):
- `--input-file=<string>` — путь к входному .med
- `--output-file=<string>` — префикс выходных файлов
- `--ndomains=<number>` — число партиций
- **`--create-boundary-faces`** — «creates the necessary faces so that
  **faces joints are created in the output files**»
- `--plain-master` — формат master file
- `--family-splitting`, `--distributed`, `--mesh-only`, `--meshname`

**API joints подтверждена** в [SalomePlatform/medcoupling source](https://github.com/SalomePlatform/medcoupling):

| Класс | Ключевые методы |
|---|---|
| `MEDFileMesh` | `getJoints() → MEDFileJoints`, `getNumberOfJoints()` |
| `MEDFileJoints` | `getJointsNames() → vector<string>` |
| `MEDFileJoint` | `getStepAtPos(i) → MEDFileJointOneStep` |
| `MEDFileJointOneStep` | `getNumberOfCorrespondences()`, `getCorrespondenceAtPos(i)` |
| `MEDFileJointCorrespondence` | `getIsNodal() → bool`, `getCorrespondence() → DataArrayIdType` |

Полная цепочка для нашего скрипта:

```python
from MEDLoader import MEDFileMesh

for part_file in glob.glob("parts/*.med"):
    mesh = MEDFileMesh.New(part_file)
    joints = mesh.getJoints()          # MEDFileJoints
    for jname in joints.getJointsNames():
        joint = joints.getJointAtPos(...)  # MEDFileJoint
        # joint has subdomain id of remote neighbour
        step = joint.getStepAtPos(0)   # MEDFileJointOneStep
        for i in range(step.getNumberOfCorrespondences()):
            corr = step.getCorrespondenceAtPos(i)
            if corr.getIsNodal():
                arr = corr.getCorrespondence()
                # arr.getNumberOfTuples() = N shared nodes between domains
                F[my_subdomain][neighbour] += arr.getNumberOfTuples()
```

**Python bindings** (SWIG) для всех этих классов есть в
`src/MEDLoader/Swig/MEDLoaderCommon.i` — то есть API **доступно из
Python** напрямую, не нужно C++ wrapper.

### Что подтверждено и что осталось проверить

| Что | Состояние |
|---|---|
| `medpartitioner` поддерживает `--create-boundary-faces` | ✅ официальная документация |
| Этот флаг пишет **joints** в output MED | ✅ официальная документация |
| MEDFileMesh API имеет `getJoints()` | ✅ source code SalomePlatform/medcoupling |
| MEDFileJointCorrespondence имеет `getIsNodal()` и `getCorrespondence()` | ✅ source code |
| Python bindings существуют для всех Joint классов | ✅ SWIG `.i` файлы |
| **`medpartitioner` физически установлен в нашем codeaster image** | ⚠️ нужно проверить smoke |
| **MEDLoader Python модуль на PYTHONPATH в codeaster image** | ⚠️ нужно проверить smoke |
| Точные имена партиционированных MED файлов (`part_<i>.med`?) | ⚠️ нужно проверить smoke |

### Риски

| Риск | Mitigation |
|---|---|
| `medpartitioner` не установлен в нашем codeaster image | `make bootstrap` строит MEDCoupling — должен быть; проверка `which medpartitioner` в smoke |
| `MEDLoader` не на PYTHONPATH | проверить `python3 -c 'from MEDLoader import MEDFileMesh'` в smoke; если нет — добавить путь в `LD_LIBRARY_PATH`/`PYTHONPATH` через ENV |
| `getJointAtPos` или похожий метод не существует (API name mismatch) | в источнике видим `getJointsNames` (с s), `getStepAtPos` — точные имена методов |
| Joint references субдомен по string-имени, не int | парсим имя из конвенции `joint_<i>_<j>` |

### Pipeline после подтверждения

```bash
cd /pvc/simulations/<jobID>
# Code_Aster берёт mesh из export файла
INPUT_MED=$(grep -E '^F libr' *.export | awk '{print $4}')

medpartitioner \
    --input-file=$INPUT_MED \
    --output-file=parts/part \
    --ndomains=16 \
    --create-boundary-faces \
    --plain-master

# теперь parts/part_0.med ... parts/part_15.med
python3 /opt/scripts/extract_codeaster_graph.py \
    --parts-dir parts/ \
    --ndomains 16 \
    > /scheduler-graphs/<jobID>.edgelist
```

### Оценка

| Этап | Время |
|---|---|
| Адаптация существующего `extract_codeaster_graph.py` под реальные API имена методов | 30 мин (API уже соответствует source) |
| Smoke test на forma01a или perf009 в codeaster image (проверка `medpartitioner` + MEDLoader) | 1 час |
| Отладка filename mismatches / API edge cases | 1-2 часа |
| **Итого** | **3-4 часа** |

Это **сильно меньше** чем я раньше говорил (4+ часа). Source code
подтверждает что подход работает — главное проверить что
наши Docker образы действительно содержат `medpartitioner` и
MEDLoader Python module.

## Что общее для всех трёх

### Shared PVC `scheduler-graphs`

Новый PVC в `k8s/10-storage.yaml`:
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: scheduler-graphs
  namespace: cfd-platform
spec:
  accessModes: [ReadWriteMany]
  resources:
    requests:
      storage: 1Gi
```

Mount:
- В **backend pod** (init container записывает edgelist)
- В **scheduler-extender pod** (читает edgelist при размещении)

### Init container в MPIJob launcher

Backend формирует MPIJob spec с **init container** в launcher pod
template. Init container запускается **до** основного solver, в нём:
1. Распакован tar.gz (это делает основной flow backend)
2. Запускается solver-specific extraction
3. Edgelist лежит в `/scheduler-graphs/<jobID>.edgelist`
4. Main launcher container стартует только после успеха init

Backend `simulation_manager.go` имеет helper `solverInitContainer(t SimType, simID, np)` который выбирает правильный init по решателю.

### Failure modes

| Что | Что делает backend |
|---|---|
| Init container failed (extraction не получилась) | MPIJob фейлится; backend отмечает sim как `failed` |
| Edgelist пустой (нет cross-rank communication) | scheduler fallback на random; warning в log |
| Scheduler не находит edgelist для job | random placement; warning в log |

## Порядок реализации

1. **OpenFOAM extraction** (самый простой) — 1 час
2. **Backend MPIJob init container template** — 30 минут
3. **Shared PVC scheduler-graphs** — 15 минут
4. **OpenRadioss extraction** (sed-patch уже есть) — 90 минут
5. **Code_Aster extraction** (рисковая) — 2-4 часа
6. Smoke test всех трёх на kind кластере — 2-4 часа

**Итого по backend: 8-12 часов**, потом deployment.
