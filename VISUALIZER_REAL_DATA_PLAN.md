# План: реальные данные во вьювере и в «Скачать результаты»

Заменить синтетический vtk.js‑демо на **настоящий результат расчёта** — реальную
поверхность модели, раскрашенную реальным полем (давление/скорость), с легендой
из настоящего `getRange()`; кнопку «Скачать результаты» — на выгрузку реальных
чисел. Сначала **OpenFOAM** (motorBike); OpenRadioss/Code_Aster — позже (у
codeaster путь ещё не валидирован), для них остаётся честный fallback.

Выбран вариант: **«реальные данные, по‑настоящему»** (см. диалог 2026‑06‑05).

---

## 0. Критическая находка (почему «ретрофит без пересчёта» ≠ «совсем без счёта»)

Уже посчитанный прогон **4b0ce57f** на общем PVC содержит `processor*/{0,constant}`,
`postProcessing/` (силы), но **НЕ содержит времени с полями** (`processor0/50` нет).
Причина: smoke собран с `endTime=50`, а `writeInterval` остался `100` →
simpleFoam отработал, но ни разу не записал снапшот p/U. Считалось честно (mpiP
есть), сохранять было нечего.

Следствие: вытащить поле напрямую из 4b0ce57f **нельзя — его нет**. НО полный
пересчёт (стейджинг + snappy + env‑грабли) НЕ нужен:
- сетка готова и закэширована: `/root/stage/motorBike_meshed` (constant/polyMesh,
  353 554 ячейки, 16‑way scotch);
- нужен **один дешёвый досчёт на готовой сетке** с `writeInterval ≤ endTime`,
  который запишет одно время с полем. Serial simpleFoam ~200–500 итераций на этой
  сетке = ~2–8 мин, поле физически то же, что в кластерном прогоне (декомпозиция
  не меняет сошедшееся решение). Это «showcase‑солв», а не «пересчёт всего».

Также: в тарбол бенчмарка надо выставить `writeInterval` так, чтобы прогоны
писали финальное поле (иначе нечего экспортировать). Тайминговую метрику это не
трогает — запись идёт ПОСЛЕ счёта, вне mpiP‑региона.

---

## 1. Экспорт результата (solver / one‑off пост‑обработка)

Из решённого кейса получить **одну лёгкую поверхность** (патчи модели), несущую
реальное поле — не весь объём (350K ячеек), а поверхность (~десятки тысяч
треугольников → файл в единицы МБ, быстро отдать и отрисовать).

OpenFOAM:
```
reconstructPar -latestTime                # склеить processor*/ → единое время
foamToVTK -latestTime -ascii \            # экспорт VTK
          -fields '(p U k)' \
          -surfaceFields                  # только поверхности патчей
# → VTK/<patch>/<patch>_<time>.vtp (или legacy .vtk)
```
Альтернатива без reconstruct: functionObject `surfaces` (тип `patch`, поверхности
`motorBike.*`) с `surfaceFormat vtk`, исполняемый на `writeTime` — пишет
поверхность сразу в параллельном прогоне.

Результат экспорта → положить туда, откуда бэкенд его отдаёт:
`/pvc/simulations/<id>/surface.vtp` (+ `stats.json` с реальными именами полей и
min/max/mean — для легенды и CSV).

- **Для 4b0ce57f (ретрофит):** один serial showcase‑солв на закэшированной сетке
  пишет поле → foamToVTK поверхности → `surface.vtp` + `stats.json` →
  `kubectl cp` в case‑PVC `/pvc/simulations/4b0ce57f/`. Без re‑stage, без snappy.
- **Для будущих прогонов (бенчмарк):** добавить экспорт в solverCommand ПОСЛЕ
  тайминг‑региона (после mpiP‑capture), либо делать экспорт только для одного
  «витринного» прогона на солвер, чтобы не пухнуть по диску и не трогать тайминги.

Формат: брать тот, что читает vtk.js — `vtkXMLPolyDataReader` для `.vtp`
(ascii/appended) либо `vtkPolyDataReader` для legacy ascii `.vtk`. Проверить, что
foamToVTK даёт совместимый (при необходимости — `-poly`/`-ascii`).

---

## 2. Бэкенд: эндпоинты раздачи

`backend/internal/...` — новые маршруты (nginx уже проксирует `/api/` → бэкенд,
менять nginx не нужно):

- `GET /api/simulations/{id}/surface` → стримит `surface.vtp` из PVC
  (`http.ServeContent`/`io.Copy`, `Content-Type: application/octet-stream`), 404
  если нет (тогда фронт идёт в fallback).
- `GET /api/simulations/{id}/results` → реальная числовая сводка из `stats.json`
  (имена полей + min/max/mean), питает CSV и легенду. 404 → фронт берёт текущую
  параметрическую сводку.
- **Заодно**: починить `DELETE /api/simulations/{id}` — сейчас 500, если MPIJob
  уже удалён (`mpijobs ... not found`); делать delete идемпотентным (NotFound =
  ок). Маленький, но реальный robustness‑фикс (всплыл при чистке 244c1b27).

Сборка: `cfd-platform-backend:local` → `kind load` → `rollout restart`. В памяти
репозитория in‑memory, поэтому рестарт бэкенда **сотрёт список прогонов** — делать
ретрофит‑экспорт ПОСЛЕ передеплоя бэкенда (или мириться с очисткой списка).

---

## 3. Фронт: Visualizer.tsx

При открытии модалки результата:
1. `fetch('/api/simulations/{id}/surface')`.
2. **200** → распарсить vtk.js‑ридером, построить mapper по реальному скаляру,
   colormap «Cool to Warm», `setScalarRange(realRange)`, числа легенды из
   `array.getRange()`, единицы — из карты полей. Реальная геометрия + реальное
   поле + реальная легенда. Селектор полей — из массивов, что реально есть в
   полидате (p, |U|, k).
3. **404** (radioss/codeaster или прогон без экспорта) → текущий синтетический
   рендер, но подпись честная: «репрезентативная визуализация поля», не «Демо».
4. `downloadResults()`: `fetch('/api/simulations/{id}/results')` → CSV из реальных
   min/max/mean; нет — текущая параметрическая сводка (помечена как сводка
   параметров, не результат).

Сборка: `cfd-platform-frontend:local` → `kind load` → `rollout restart` →
`fepf` port‑forward уже на :8090.

---

## 4. Последовательность ретрофита 4b0ce57f (когда дадут «делай», без re‑stage)

1. One‑off контейнер на `/root/stage/motorBike_meshed`: выставить
   `writeInterval ≤ endTime`, serial `simpleFoam` ~200–500 итераций → пишет поле;
   `foamToVTK -latestTime -surfaceFields -fields '(p U)'` → `surface.vtp` +
   собрать `stats.json` (имена/диапазоны).
2. Передеплоить бэкенд (новые эндпоинты + фикс delete).
3. `kubectl cp surface.vtp stats.json` → backend‑pod → `/pvc/simulations/4b0ce57f/`.
4. Передеплоить фронт (реальный ридер).
5. Открыть 4b0ce57f → реальный мотоцикл + реальное давление → скрин. Кнопка
   «Скачать результаты» отдаёт реальные числа.

---

## 5. Скоуп / риски

- Только OpenFOAM в первой итерации; radioss/codeaster — честный fallback
  (codeaster ещё не валидирован E2E).
- Размер `.vtp`: только поверхность/патчи (НЕ объём) → единицы МБ. Полный объём
  не отдавать.
- Совместимость формата с vtk.js‑ридером — проверить на реальном файле foamToVTK.
- **Целостность бенчмарка**: экспорт ОБЯЗАН быть вне mpiP‑тайминга (пост‑солв),
  иначе раздувает `mpi_time`. Лучше — экспорт только для витринных прогонов, а 54+18
  тайминговых гонять без экспорта.
- Рестарт бэкенда чистит in‑memory список прогонов — учитывать порядок шагов.

---

## 6. Метрика → фронт по солверам (поля, формат, единицы)

Что реально считается, как пишется в выходной файл, как мапится в легенду.
Решение по единицам (2026-06-06): **сырые физ. значения в .vtp + `scale` для
отображения**; OpenFOAM `p` показываем честно в **м²/с²** (кинематич., scale 1).

| Солвер / кейс | Поле | Файл результата | Конвертер → .vtp | Массив | Реальн. ед. | scale | Подпись |
|---|---|---|---|---|---|---|---|
| OpenFOAM motorBike | давление (кинем.) | time-dir `p` | `foamToVTK` ✓ | `p` | м²/с² | 1 | м²/с² |
| | скорость | `U` | `foamToVTK` | `U` (vec) | м/с | 1 | м/с |
| | турб. энергия | `k` | `foamToVTK` | `k` | м²/с² | 1 | м²/с² |
| OpenRadioss Yaris | напряж. Мизеса | Anim `…A001` | `anim_to_vtk` (в `/opt/OpenRadioss/exec`) | по записи | ед. модели | =от модели | МПа |
| | пласт. деформация | Anim | `anim_to_vtk` | — | — | 1 | — |
| | перемещение | Anim (деф. коорд.) | `anim_to_vtk` | — | мм (модель) | 1 | мм |
| Code_Aster SimJEB | напряж. Мизеса | `.rmed` (MED) `SIEQ_*` комп. `VMIS` | meshio MED→vtu | `SIEQ_*_VMIS` | Па (SI) | 1e-6 | МПа |
| | перемещение | `.rmed` `DEPL` | meshio MED→vtu | `DEPL` | м (SI) | 1e3 | мм |

Заметки:
- **OpenFOAM `p` кинематическое** (`dimensions [0 2 -2 …]`), НЕ Па. Числа: торможение
  ≈ +200, разрежение до −300…−600 м²/с².
- **Code_Aster — SI**: напряжения в Па (~1e8), перемещения в м (~1e-4). Поэтому
  scale 1e-6 / 1e3, иначе легенда «МПа/мм» врёт в 1e6 / 1e3 раз.
- **OpenRadioss единицы зависят от модели** (LS-DYNA: `mm-ms-kg`→ГПа либо
  `mm-s-т`→МПа). Прочитать из control-карт при стейджинге Yaris (timestep 1 мкс).
- Реальный экспорт пока **только OpenFOAM** (`foamToVTK`). radioss/codeaster —
  fallback (синтетика, честная подпись), пока не дописан `anim_to_vtk` / meshio
  MED→vtu путь. Для них в `.vtp` тоже держим сырые ед., `scale` в `stats.json`
  даёт МПа/мм.
- Фронт: `FieldStat.scale` (опц., default 1); легенда/CSV = `getRange()×scale`,
  цветовая карта по сырому диапазону (scale на картинку не влияет).

## 7. Аудит контракта данных (2026-06-06) — 16 багов найдено и ПОЧИНЕНО

Многоагентный аудит цепочки export→stats.json/surface.vtp→backend→vtk.js
(6 ревьюеров + адверсариальная верификация каждой находки): 29 находок, 16
подтверждено, 13 отклонено (path-traversal не эксплуатируем при chi v5; winding/
backface — vtk.js без culling, нормали к камере; meshio валидирует блоки). Все 16
исправлены и перепроверены (go build/vet, tsc, py_compile, runtime-тест конвертера).

HIGH:
- export_surface.sh захардкодил p/U/k → если foamToVTK не записал поле (ламинар без
  k) — имя в stats.json не резолвится. ФИКС: emit только поля, реально присутствующие
  в surface.vtp (grep Name=).
- result_to_vtp.py: смешанная shell+solid сетка теряла ВЕСЬ solid (have_shell
  short-circuit). ФИКС: всегда skin solids ВМЕСТЕ с shells (проверено: tri+tet→5 граней).
- export_radioss_surface.sh: glob *A### ловил только 3 цифры → A999 вместо A1000+.
  ФИКС: числовая сортировка по суффиксу (sed+sort -n), 3+ цифр.
- DownloadResults портил ZIP-поток (respondError ПОСЛЕ коммита 200 писал JSON в
  середину zip). ФИКС: собираю zip в bytes.Buffer, статус коммичу только при успехе.
- Frontend: surface 200 + stats 404 → mode='real', realFields=null → синтетика под
  реальной подписью. ФИКС: throw на пустых stats → чистый fallback.

MEDIUM:
- result_to_vtp.py: NaN/inf → литералы 'nan'/'inf' в Float32 → vtk.js getRange=NaN.
  ФИКС: np.nan_to_num на points+полях (проверено: inf/nan→0, токенов нет).
- export_radioss_surface.sh: basename splice без кавычек в docker -c. ФИКС: позиционный $1, "/case/$1".
- FieldStats глотал io.Copy error → 200+обрезанный JSON. ФИКС: ServeContent + errors.Is.
- Delete оставлял orphan surface.vtp/stats.json/results/extraction-Job. ФИКС:
  os.RemoveAll case+results + DeleteExtractionJob.
- Легенда: stale realRange (флаш диапазона прошлого поля × новый scale). ФИКС:
  useMemo (синхронно в render) вместо setRealRange в post-render эффекте.
- fieldStats null-vs-throw асимметрия → покрыто фиксом throw-на-null.

LOW:
- интерьерные узлы solid в getRange → ФИКС: компактизация до узлов поверхности + remap.
- Surface/FieldStats/DownloadResults: любой os.Open err → 404 (маскирует 5xx). ФИКС: errors.Is(ErrNotExist)?404:500.
- realRange post-render flash (= тот же useMemo-фикс).
- webglError залипал на весь mount. ФИКС: setWebglError(false) в load-эффекте.

## Статус: КОД НАПИСАН + АУДИРОВАН, НЕ ЗАДЕПЛОЕН (2026-06-06)

Реализация по плану написана и проходит проверки (`go build`+`go vet` rc=0,
фронт `tsc --noEmit` rc=0). В кластере **ничего не пересобрано/не задеплоено** —
ждём команды.

Изменённые/новые файлы:
- `backend/internal/infrastructure/k8s/simulation_manager.go` — `DeleteJob`
  идемпотентный (NotFound = ok), фикс того 500.
- `backend/internal/delivery/http/simulation_handler.go` — хендлеры `Surface`
  (отдаёт `surface.vtp`) и `FieldStats` (`stats.json`) + const `simulationsRoot`.
- `backend/cmd/server/main.go` — маршруты `/{simId}/surface`, `/{simId}/field-stats`.
- `frontend/src/types/index.ts` — `FieldStat`/`FieldStats` (числа выводятся live
  из .vtp, поэтому в типе только name/label/unit).
- `frontend/src/services/api.ts` — `surfaceURL`, `fieldStats`, `resultsURL`.
- `frontend/src/components/Visualizer.tsx` — реальный .vtp‑ридер (vtk.js
  XMLPolyDataReader), легенда из реального `getRange`, CSV из реальных min/max/
  mean, честный fallback (loading → real → синтетика «репрезентативная»).
- `experiment/export_surface.sh` — OpenFOAM пост‑солв экспорт поверхности +
  stats.json (НЕ в тайминговом mpirun; для витрин).
- `experiment/retrofit_motorbike_viz.sh` — досчёт motorBike на кэш‑сетке (был для
  4b0ce57f; VM снесена → теперь просто свежий прогон).
- `experiment/result_to_vtp.py` — универсальный конвертер MED/legacy‑VTK →
  surface.vtp PolyData + stats.json. vtk.js умеет ТОЛЬКО PolyData (есть
  XMLPolyDataReader; нет UnstructuredGrid‑ридера), а meshio пишет только UG —
  поэтому: meshio читает → извлекаем поверхность (shell‑ячейки как есть; solid →
  skin граней, используемых один раз) → руками пишем .vtp. Fuzzy‑match полей,
  cell→point усреднение с учётом глобального индекса ячейки, vmis‑компонент / |вектор|.
  **Провалидирован на синтетике** (тет‑куб → ровно 12 граней skin; shell‑треуг.;
  .vtp прошёл XML‑проверку: счётчики/connectivity/offsets/длины полей).
- `experiment/export_radioss_surface.sh` — `anim_to_vtk` (в radioss‑образе) на
  последнем Anim‑состоянии → legacy VTK → `result_to_vtp.py`. Единицы напряжения
  через `STRESS_UNIT`/`STRESS_SCALE` (модель‑зависимы: mm‑ms‑kg→ГПа / mm‑s‑т→МПа).
- `experiment/export_codeaster_surface.sh` — meshio читает `.rmed` напрямую →
  `result_to_vtp.py` (skin тет‑сетки, нодальный von Mises SIEQ_NOEU + |DEPL|).
  Требует от `.comm`: `CALC_CHAMP(CRITERES='SIEQ_NOEU')` + `IMPR_RESU` DEPL/SIEQ_NOEU.
- Backend `Surface`/`FieldStats` уже solver‑agnostic — отдаёт любой surface.vtp+
  stats.json из PVC; фронт тоже (читает .vtp + stats.json). Поэтому новых правок
  backend/frontend для radioss/codeaster НЕ нужно.

**Остаётся (только на реальных данных, нужна VM):** прогнать `anim_to_vtk` на
реальном Yaris Anim (уточнить точное имя бинаря + имена массивов von Mises/EPSP +
единицы из unit‑system модели); прогнать meshio на реальном codeaster `.rmed`
(проверить имена полей DEPL/SIEQ_NOEU и компонент VMIS). Логика skin/.vtp уже
проверена синтетикой — на реальных данных ожидаются только правки fuzzy‑match
имён / единиц.

Решение по интеграции: экспорт — **всегда отдельный пост‑шаг**, в `solverCommand`
НЕ встроен, чтобы 54 тайминговых рана остались чистыми по `mpi_time`.

### Порядок деплоя (когда дадут «делай»)
1. Сборка `cfd-platform-backend:local` → `kind load` → `rollout restart`
   (рестарт чистит in‑memory список прогонов — поэтому ретрофит‑cp ПОСЛЕ).
2. Ретрофит: `docker run` openfoam с кэш‑сеткой + смонтированными скриптами →
   `/root/stage/export/{surface.vtp,stats.json}`.
3. `kubectl cp` артефактов в backend‑pod `/pvc/simulations/4b0ce57f/`.
4. Сборка `cfd-platform-frontend:local` → `kind load` → `rollout restart`.
5. Открыть 4b0ce57f → реальный мотоцикл + реальное давление → скрин.
