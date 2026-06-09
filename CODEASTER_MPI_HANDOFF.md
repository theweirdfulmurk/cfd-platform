# Промпт для нового Claude: запустить Code_Aster cross-pod MPI под kubeflow

Скопируй всё ниже в свежий чат Claude Code.

---

## Задача

Заставить **Code_Aster 15.5.2** считать **мультинодово (cross-pod MPI)** под
kubeflow MPI Operator на kind-кластере, чтобы параллельный solve завершался
штатно (`Code_Aster MPI exits normally` / `ARRET NORMAL`), а ранги были
распределены по worker-подам. Сейчас два других решателя (OpenFOAM, OpenRadioss)
работают cross-pod, а Code_Aster — нет.

## Доступ и окружение

- VM (kind-кластер "cfd", 37 нод): `ssh -p 36123 root@211.21.106.81`
  (ServerAliveInterval=15 желателен — sshd иногда рвёт).
- Образ решателя: `ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest`
  на хосте VM (docker). Пересобран на **OpenMPI 4.1.6** (из исходников,
  `--enable-mpi1-compatibility`) + **ScaLAPACK 2.1.0** из исходников.
  `mpirun` = `/aster/openmpi/bin/mpirun` (4.1.6), libmpi.so.40.
- Бэкенд (Go), команда запуска codeaster:
  `~/Documents/code/cfd-platform/backend/internal/infrastructure/k8s/simulation_manager.go`
  (на VM: `/root/cfd-platform/...`). Ищи `SimTypeCodeAster` и переменную `run`.
- Тестовая фикстура на VM: `/root/ca_probe/mesh.med` (2D осесимметричная сетка,
  TRIA6) + `/root/ca_simple_study.comm` (простой линейно-упругий кейс) +
  `/root/study.export`.
- **Тестировать дёшево можно в одном контейнере** (без кластера), локальным
  `mpirun -np 4` — параллельный путь воспроизводится так же:
  ```
  docker run --rm -v /root/ca_probe:/case --entrypoint bash \
    ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest -lc '...'
  ```

## Что УЖЕ работает (не трогать, это победы)

- OpenMPI 4.1.6 собран, mpi4py есть в `/aster/petsc/lib/mpi4py`.
- Ранги **связываются в один MPI-мир** (все 4 печатают `Fin de lecture` —
  читают сетку), METIS-партиционирование кейса проходит
  (`DISTRIBUTION=_F(METHODE='SOUS_DOMAINE', PARTITIONNEUR='METIS')`).
- **Single-container последовательно** code_aster считает кейс чисто
  (`python3 fort.1` с префиксом `from code_aster.Commands import *;
  from code_aster.Cata.Syntax import _F; from math import *`, источив
  `/aster/aster/share/aster/profile.sh`).
- mpi4py находится в ранге, если прокинуть `-x PYTHONPATH` в mpirun (profile.sh
  делает `PYTHONPATH=/aster/aster/lib64/aster:${PYTHONPATH}:.` — полагается, что
  во входящем PYTHONPATH уже есть `/aster/petsc/lib`).

## ДВА режима отказа (суть проблемы)

**Режим A — прямой запуск `mpirun -np N python3 fort.1`** (code_aster = ранг,
БЕЗ форка): MPI_Init проходит, все ранги читают сетку, затем **SEGFAULT** в
операторе `op0018_` через `boost::python translate_exception` (вложенные
`ErrorCpp<6..1>`, null-deref по адресу 0x70). Гипотеза: без супервизора
run_aster трансляция исключения C++→Python падает, **маскируя реальную ошибку**
(саму ошибку не видно). Стек:
```
op0018_ -> execop_ -> ... boost::python translate_exception(ErrorCpp<...>)
  -> handle_exception -> PyErr_SetObject -> Segmentation fault (139)
```

**Режим B — `run_aster export` (канонический лаунчер)**: форкает code_aster
подпроцессом (`python3 -X faulthandler -- ./study.comm.changed.py --last
--link=... --memory ... --tpmax ... --numthreads 1`), и форк **не может
MPI_Init**:
```
PMIX ERROR: UNPACK-PAST-END in unpack.c:117
*** An error occurred in MPI_Init *** on a NULL communicator
*** Local abort before MPI_INIT completed
```
Это известный баг OpenMPI: **дочерний процесс MPI-ранга наследует
`PMIX_*/OMPI_*` и не может инициализировать свой MPI** —
https://github.com/open-mpi/ompi/issues/3158 . А если запустить `run_aster`
БЕЗ внешнего mpirun (чтобы он сам сделал mpiexec) → **рекурсия**:
`Open MPI does not support recursive calls of mpirun`.

## Конфигурационные находки (важно)

- `run_aster` есть: `/aster/aster/bin/run_aster` (не на PATH; добавь
  `/aster/aster/bin` в PATH).
- `/aster/aster/share/aster/config.json` (НЕ yaml — yaml пустой):
  - **НЕТ `mpi_get_rank`** → `get_procid()` падает (`run(None, shell=True)`).
    Чинится добавлением `"mpi_get_rank": "echo $OMPI_COMM_WORLD_RANK"`.
  - `mpiexec` в СТАРОМ asrun-формате `%(mpi_nbcpu)s --hostfile %(mpi_hostfile)s
    %(program)s` → run_aster хочет Python-format `{mpi_nbcpu}` / `{program}`
    (см. доку), иначе `(` ломает sh.
  - Параметры можно править прямо в config.json (python json), но
    `CONFIG_PARAMETERS_*` env в РАНТАЙМЕ не подхватываются (только на configure).
- Супервизор-обёртка, которую генерит run_aster: `study.comm.changed.py`,
  запускается как `python3 -X faulthandler -- ./study.comm.changed.py --last
  --link="F::mmed::<mesh>::D::20" --memory <MB> --tpmax <s> --numthreads 1`.
- Формат `.export` (пример `/aster/aster/share/aster/tests/forma01a.export`):
  ```
  P time_limit 600
  P memory_limit 2048
  P mpi_nbcpu 4
  P mpi_nbnoeud 1
  F comm study.comm D 1
  F mmed mesh.med D 20
  ```
- run_aster --help: `run_aster [options] [EXPORT]`; есть `--env` (только
  подготовить wrkdir, не запускать; требует `-w/--wrkdir`) и `-w WRKDIR`.

## Самая перспективная НЕДОДЕЛАННАЯ идея

Совместить «без форка» + «супервизор»:
1. `run_aster --env --wrkdir <dir> study.export` → подготовить wrkdir +
   сгенерить `study.comm.changed.py` (БЕЗ запуска).
2. `cd <dir> && mpirun -np 4 python3 -X faulthandler -- ./study.comm.changed.py
   --last --link="F::mmed::<dir>/mesh.med::D::20" --memory 2048 --tpmax 600
   --numthreads 1` → code_aster = прямой ранг (нет форка → нет PMIX-бага) +
   супервизор (чистая обработка исключений → должна снять op0018-крэш).

У меня `run_aster --env` не довёл подготовку (упирался в формат config.json
mpiexec `%()s` и/или в get_procid/рекурсию). **Начни с того, чтобы заставить
`run_aster --env` чисто сгенерить `study.comm.changed.py`**, затем гони обёртку
напрямую под mpirun. Если заработает в одном контейнере (`mpirun -np 4 ...
study.comm.changed.py`) — потом проброси в бэкенд (kubeflow mpirun уже
разносит по подам; нужен лишь общий wrkdir на PVC или пер-под подготовка).

## Другие зацепки

- Разобраться, ПОЧЕМУ режим A крэшит в boost (op0018): это маска реальной
  ошибки. Попробуй `ulimit -s unlimited` перед запуском (стек?), либо запусти
  под супервизором (см. выше), чтобы УВИДЕТЬ настоящую ошибку (`<F>` UTMESS).
- Форумы code_aster: запуск через run_aster на SLURM-кластере
  (forum.code-aster.org, поиск «run_aster slurm mpiexec cluster»); salome_meca
  multi-node MPI.
- Воркэраунд OMPI #3158: возможно, перед форком code_aster надо unset
  `PMIX_*`, `OMPI_*`, `PMI_*` И затем дать подпроцессу заново подключиться —
  но это сломает связь с MPI-джобом, поэтому правильнее НЕ форкать (идея выше).

## Критерий успеха

`mpirun -np 4 <…>` (локально в контейнере или cross-pod) → code_aster
**завершает параллельный solve** на простом кейсе (`/root/ca_simple_study.comm`,
линейно-упругий, MUMPS, METIS) → `Code_Aster MPI exits normally` + таблица
результатов без segfault/PMIX-ошибок. Потом — то же cross-pod через бэкенд
(MPIJob), ранги на разных worker-подах.

## Правила окружения (важно)

- Таймед-бенчмарк-прогоны — только с явного разрешения владельца; смоук/тесты
  одиночных джобов — можно.
- Снос образов решателей с kind-нод заблокирован (диск тесный, 485ГБ); если
  нужен полный re-load — спрашивай владельца. Для тестов хватает ОДНОГО
  контейнера (`docker run`), кластер не нужен.
- Кластер сейчас: 29 нод закордонены (раскордонить:
  `kubectl uncordon <node>`), codeaster-образ загружен только на ~7 нод зоны a
  (worker3–9). Для контейнерных тестов это неважно.

---

# ПРИЛОЖЕНИЕ: дословные артефакты и команды воспроизведения

## A. Текущая backend-команда запуска codeaster (РЕЖИМ A, прямой python3 fort.1)

В `simulation_manager.go`, `solverCommand()`, ветка `SimTypeCodeAster`,
переменная `run` (это то, что сейчас задеплоено и крэшит в op0018/boost):

```go
run = fmt.Sprintf("for h in $(awk '{print $1}' /etc/mpi/hostfile 2>/dev/null); do n=0; until getent hosts \"$h\" >/dev/null 2>&1 || [ $n -ge 120 ]; do sleep 1; n=$((n+1)); done; done; mpirun --allow-run-as-root --mca routed radix --mca plm_rsh_no_tree_spawn 1 --mca oob_tcp_if_include eth0 --mca btl self,tcp --mca pml ob1 --mca btl_tcp_if_include eth0 -x PATH -x LD_LIBRARY_PATH -x PYTHONPATH -np %d bash -lc '. /aster/aster/share/aster/profile.sh; rm -rf /tmp/ca && mkdir -p /tmp/ca && cd /tmp/ca; { echo \"from code_aster.Commands import *\"; echo \"from code_aster.Cata.Syntax import _F\"; echo \"from math import *\"; cat %s; } > fort.1; ln -sf %s fort.20; python3 fort.1 --memjeveux=2048 --tpmax=3600 --rep_outils=/aster/asrun/outils --rep_mat=/aster/aster/share/aster/materiau --rep_dex=/aster/aster/share/aster/datg --numthreads=1'", np, caseComm, caseMesh)
```
Где `caseComm=<caseDir>/study.comm`, `caseMesh=<caseDir>/mesh.med`,
`caseDir=/pvc/simulations/<simID>`. Цель — заменить этот `python3 fort.1`
на корректный супервизор-запуск (см. идею выше), сохранив прокидку по подам.
DNS-wait в начале (`for h in ... getent hosts`) — оставить, он нужен против
гонки резолвинга воркеров (без него mpirun рандомно падает на старте).

Для openfoam/radioss (РАБОТАЮТ) команды в той же функции — можно подсмотреть
рабочие MCA-флаги.

## B. Текущий config.json (БЕЗ mpi_get_rank, mpiexec в старом формате)

`/aster/aster/share/aster/config.json`:
```json
{
  "FC": "mpif90",
  "FCFLAGS": ["-fPIC","-fdefault-double-8","-fdefault-integer-8","-fdefault-real-8","-Wimplicit-interface","-Wintrinsic-shadow","-fno-aggressive-loop-optimizations","-ffree-line-length-none"],
  "addmem": "4096",
  "exectool": {},
  "mpiexec": "mpirun -np %(mpi_nbcpu)s --hostfile %(mpi_hostfile)s %(program)s --allow-run-as-root",
  "only-proc0": 0,
  "parallel": 1,
  "python": "python3",
  "python_interactive": "python3",
  "tmpdir": "/tmp",
  "version_sha1": "179508b6116cd2688a224e619049c2108fbc001e",
  "version_tag": "15.5.2"
}
```
Проблемы: (1) НЕТ `mpi_get_rank` → `get_procid()` в run.py делает
`run(None, shell=True)` и падает; (2) `mpiexec` в asrun-формате `%(...)s`,
run_aster хочет `{mpi_nbcpu}`/`{program}` (Python str.format). `%(mpi_hostfile)s`
тоже проблемный — hostfile run_aster не передаёт.

Минимальный патч для тестов (внутри контейнера, рантайм):
```python
python3 -c 'import json;f="/aster/aster/share/aster/config.json";d=json.load(open(f));d["mpi_get_rank"]="echo $OMPI_COMM_WORLD_RANK";json.dump(d,open(f,"w"))'
```

## C. Простой тестовый кейс (РЕШАЕТСЯ single-container, ПАДАЕТ в параллели)

`/root/ca_simple_study.comm` — линейно-упругий, на той же сетке, без трещин:
```python
DEBUT(CODE=_F(NIV_PUB_WEB="INTERNET"))
MA = LIRE_MAILLAGE(FORMAT="MED", PARTITIONNEUR="SANS")
MO = AFFE_MODELE(MAILLAGE=MA,
                 AFFE=_F(TOUT="OUI", PHENOMENE="MECANIQUE", MODELISATION="AXIS"),
                 DISTRIBUTION=_F(METHODE="SOUS_DOMAINE", PARTITIONNEUR="METIS"))
ACIER = DEFI_MATERIAU(ELAS=_F(E=200000.0, NU=0.3))
CM = AFFE_MATERIAU(MAILLAGE=MA, AFFE=_F(TOUT="OUI", MATER=ACIER))
CH = AFFE_CHAR_MECA(MODELE=MO,
                    DDL_IMPO=(_F(GROUP_MA="AXI", DX=0.0),
                              _F(GROUP_MA="INF", DY=0.0),
                              _F(GROUP_MA="SUP", DY=0.01)))
RESU = MECA_STATIQUE(MODELE=MO, CHAM_MATER=CM, EXCIT=_F(CHARGE=CH),
                     SOLVEUR=_F(METHODE="MUMPS"))
FIN()
```
Группы сетки: `AXI, INF, SUP, EXT, FOND, LEV_INF, LEV_SUP` (есть в mesh.med).
`.export` (`/root/study.export`): `P mpi_nbcpu 4`, `F comm study.comm D 1`,
`F mmed mesh.med D 20` (+ time_limit/memory_limit).

## D. Команды воспроизведения (copy-paste, ОДИН контейнер, без кластера)

**Базовая обёртка** (всё гонять внутри):
```
docker run --rm -v /root/ca_simple_study.comm:/in/study.comm \
  -v /root/ca_probe/mesh.med:/in/mesh.med -v /root/study.export:/in/study.export \
  -v /root:/out --entrypoint bash \
  ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest -lc '
  . /aster/aster/share/aster/profile.sh
  export PATH=/aster/aster/bin:$PATH
  <ТВОИ КОМАНДЫ>
'
```

**D1. Single-container последовательно — РАБОТАЕТ** (контроль, что образ ок):
```
mkdir -p /tmp/t && cd /tmp/t
{ echo "from code_aster.Commands import *"; echo "from code_aster.Cata.Syntax import _F"; echo "from math import *"; cat /in/study.comm; } > fort.1
ln -sf /in/mesh.med fort.20
python3 fort.1 --memjeveux=2048 --tpmax=600 --rep_outils=/aster/asrun/outils --rep_mat=/aster/aster/share/aster/materiau --rep_dex=/aster/aster/share/aster/datg --numthreads=1 2>&1 | tail -8
# Ожидается: "Code_Aster MPI exits normally", "ARRET NORMAL DANS FIN"
```

**D2. РЕЖИМ A — параллельно прямым python3 fort.1 → SEGFAULT op0018/boost:**
```
mkdir -p /tmp/t && cd /tmp/t
{ echo "from code_aster.Commands import *"; echo "from code_aster.Cata.Syntax import _F"; echo "from math import *"; cat /in/study.comm; } > fort.1
ln -sf /in/mesh.med fort.20
mpirun --allow-run-as-root --mca btl self,tcp -np 4 python3 fort.1 --memjeveux=2048 --tpmax=600 --rep_outils=/aster/asrun/outils --rep_mat=/aster/aster/share/aster/materiau --rep_dex=/aster/aster/share/aster/datg --numthreads=1 2>&1 | tail -20
# Ранги читают сетку (Fin de lecture x4), потом segfault в boost translate_exception (op0018)
```

**D3. РЕЖИМ B — run_aster (с патчем config) → PMIX fork-баг / рекурсия:**
```
python3 -c 'import json;f="/aster/aster/share/aster/config.json";d=json.load(open(f));d["mpi_get_rank"]="echo $OMPI_COMM_WORLD_RANK";json.dump(d,open(f,"w"))'
mkdir -p /stage && cd /stage && cp /in/study.comm /in/mesh.med /in/study.export .
mpirun --allow-run-as-root --mca btl self,tcp -np 4 run_aster study.export 2>&1 | tail -20
# Форкнутый code_aster: "PMIX UNPACK-PAST-END" / "MPI_Init on NULL communicator"
```

**D4. ПЕРСПЕКТИВНАЯ ИДЕЯ (НЕДОДЕЛАНО) — --env + прямой запуск обёртки:**
```
python3 -c 'import json;f="/aster/aster/share/aster/config.json";d=json.load(open(f));d["mpi_get_rank"]="echo $OMPI_COMM_WORLD_RANK";json.dump(d,open(f,"w"))'
mkdir -p /stage && cd /stage && cp /in/study.comm /in/mesh.med /in/study.export .
run_aster --env --wrkdir /work study.export    # ДОЛЖЕН подготовить /work + study.comm.changed.py, но у меня падал
ls /work
WRAP=$(ls /work/*.changed.py | head -1)
cd /work && mpirun --allow-run-as-root --mca btl self,tcp -np 4 \
  python3 -X faulthandler -- ./$(basename $WRAP) --last \
  --link="F::mmed::/work/mesh.med::D::20" --memory 2048 --tpmax 600 --numthreads 1 2>&1 | tail -20
# ЦЕЛЬ: code_aster = прямой ранг (без форка) + супервизор → должно решиться.
# У меня run_aster --env не дошёл до генерации обёртки (формат mpiexec / recursion).
# Возможно надо ещё чинить config.json mpiexec в {mpi_nbcpu}/{program} формат,
# либо обойти --env и собрать обёртку study.comm.changed.py вручную (изучи,
# что run_aster кладёт в неё: /aster/aster/lib64/aster/run_aster/*.py).
```

## E. Хронология применённых фиксов (состояние образа/бэкенда)

1. Пересборка образа: apt OpenMPI 2.1.1 → **OpenMPI 4.1.6 из исходников**
   (`--enable-mpi1-compatibility --enable-orterun-prefix-by-default
   --without-verbs --without-ucx`) + **ScaLAPACK 2.1.0 из исходников**
   (apt scalapack/blacs были ABI-привязаны к 2.1.1). Dockerfile:
   `docker/codeaster/Dockerfile`. Это убрало старый зависон MPI_Init.
2. Бэкенд: `-x PYTHONPATH` в mpirun (иначе ранг не находит mpi4py: profile.sh
   не добавляет /aster/petsc/lib, только наследует).
3. Бэкенд: prepend `from math import *` (тестовые .comm используют pi/sin).
4. Бэкенд: DNS-wait перед mpirun (гонка резолвинга воркеров).
5. Бэкенд: убран флаг `orte_keep_fqdn_hostnames` (ломал DNS), MCA
   `routed radix --btl self,tcp --pml ob1`.

После 1–5: **ранги связываются, читают сетку, METIS партиционирует** — но
solve падает в op0018/boost (режим A). Это и есть граница.

## F. Что НЕ помогло (не повторяй)

- Просто апгрейд OpenMPI (2.1.1→4.1.6) crash op0018 НЕ убрал (он не про версию).
- MCA-тюнинг (routed direct/radix, btl self,tcp, pml ob1) — меняет симптом
  старого 2.1.1, но op0018-крэш на 4.1.6 не лечит.
- `CONFIG_PARAMETERS_mpi_get_rank` env в рантайме НЕ подхватывается (только
  на configure-стадии) — правь config.json напрямую.
- `mpirun run_aster` (внешний mpirun) → форк-PMIX баг; `run_aster` (свой
  mpiexec) → рекурсия mpirun. Ни один «в лоб» не работает.
