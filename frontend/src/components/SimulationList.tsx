import {
  useState,
  useEffect,
  useCallback,
  useRef,
  useMemo,
  lazy,
  Suspense,
  KeyboardEvent,
  MouseEvent,
} from 'react';
import { Simulation, SimulationStatus } from '../types';
import { simulationAPI } from '../services/api';
import { DEMO_SIMULATIONS } from '../lib/demoData';
import { StatusPill } from './StatusPill';
import { useToast } from './ToastProvider';
import { Modal } from './Modal';
import { IconTrash, IconRefresh, IconSearch, IconX, IconEye } from './icons';

// vtk.js is heavy; load the viewer only when a result is opened.
const Visualizer = lazy(() =>
  import('./Visualizer').then(m => ({ default: m.Visualizer }))
);
import { SOLVER_LABEL, algorithmTag, duration, relativeTime, STATUS_META } from '../lib/format';
import './SimulationList.css';

type SortKey = 'recent' | 'name';
const STATUS_FILTERS: ('all' | SimulationStatus)[] = [
  'all',
  'running',
  'pending',
  'completed',
  'failed',
];

function rowTime(sim: Simulation): { value: string; label: string } {
  switch (sim.Status) {
    case 'running':
      return { value: duration(sim.StartedAt ?? sim.CreatedAt), label: 'идёт' };
    case 'completed':
      return { value: duration(sim.StartedAt ?? sim.CreatedAt, sim.CompletedAt), label: 'всего' };
    case 'failed':
      return { value: duration(sim.StartedAt ?? sim.CreatedAt, sim.CompletedAt), label: 'длительность' };
    default:
      return { value: relativeTime(sim.CreatedAt), label: 'в очереди' };
  }
}

export function SimulationList() {
  const [sims, setSims] = useState<Simulation[]>([]);
  const [loading, setLoading] = useState(true);
  const [isDemo, setIsDemo] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [vizSim, setVizSim] = useState<Simulation | null>(null);
  const [, tick] = useState(0);

  const [query, setQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | SimulationStatus>('all');
  const [sort, setSort] = useState<SortKey>('recent');

  const everConnected = useRef(false);
  const { notify } = useToast();

  const load = useCallback(async () => {
    try {
      const data = await simulationAPI.list();
      everConnected.current = true;
      setSims(data);
      setIsDemo(false);
      setError(null);
    } catch (err) {
      if (everConnected.current) {
        // Backend was reachable before, so this is a real outage, not an
        // empty environment. Keep the last data and surface the error.
        setError(err instanceof Error ? err.message : 'Связь с кластером потеряна');
      } else {
        // Never connected (e.g. viewing without a cluster): show demo data.
        setSims(DEMO_SIMULATIONS);
        setIsDemo(true);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const poll = setInterval(load, 5000);
    return () => clearInterval(poll);
  }, [load]);

  useEffect(() => {
    const t = setInterval(() => tick(n => n + 1), 1000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    if (!selected && sims.length) setSelected(sims[0].ID);
  }, [sims, selected]);

  async function confirmDelete(id: string) {
    setConfirmId(null);
    const target = sims.find(s => s.ID === id);
    if (isDemo) {
      setSims(s => s.filter(x => x.ID !== id));
      return;
    }
    try {
      await simulationAPI.delete(id);
      setSims(s => s.filter(x => x.ID !== id));
      notify(`Удалён ${target?.Name ?? 'расчёт'}`, 'success');
    } catch (err) {
      notify(err instanceof Error ? err.message : 'Не удалось удалить расчёт', 'error');
    }
  }

  function onRowKey(e: KeyboardEvent, id: string) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      setSelected(id);
    }
  }

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    const filtered = sims.filter(s => {
      if (statusFilter !== 'all' && s.Status !== statusFilter) return false;
      if (q && !s.Name.toLowerCase().includes(q)) return false;
      return true;
    });
    const sorted = [...filtered];
    sorted.sort((a, b) => {
      if (sort === 'name') return a.Name.localeCompare(b.Name);
      return new Date(b.CreatedAt).getTime() - new Date(a.CreatedAt).getTime();
    });
    return sorted;
  }, [sims, query, statusFilter, sort]);

  const running = sims.filter(s => s.Status === 'running').length;
  const filtersActive = query.trim() !== '' || statusFilter !== 'all';

  return (
    <div className="sim-list">
      <div className="list-head">
        <h2 className="list-title">
          Расчёты
          {sims.length > 0 && <span className="count mono">{sims.length}</span>}
        </h2>
        {running > 0 && (
          <span className="running-note">
            <span className="running-dot" />
            {running} выполняется
          </span>
        )}
      </div>

      {error && (
        <div className="conn-note" role="alert">
          <span>{error}. Повтор каждые 5 с.</span>
          <button className="conn-retry" onClick={() => load()}>
            <IconRefresh size={13} /> Повторить
          </button>
        </div>
      )}
      {isDemo && !error && (
        <div className="demo-note">
          Показаны демо-данные: кластер недоступен из этого окна.
        </div>
      )}

      {!loading && sims.length > 0 && (
        <div className="toolbar">
          <div className="search">
            <IconSearch size={15} className="search-icon" />
            <input
              type="text"
              className="search-input"
              placeholder="Поиск по имени…"
              value={query}
              onChange={e => setQuery(e.target.value)}
              aria-label="Поиск расчётов по имени"
            />
            {query && (
              <button className="search-clear" onClick={() => setQuery('')} aria-label="Очистить поиск">
                <IconX size={13} />
              </button>
            )}
          </div>
          <div className="seg" role="group" aria-label="Фильтр по статусу">
            {STATUS_FILTERS.map(s => (
              <button
                key={s}
                className={`seg-btn${statusFilter === s ? ' is-active' : ''}`}
                onClick={() => setStatusFilter(s)}
              >
                {s === 'all' ? 'Все' : STATUS_META[s].label}
              </button>
            ))}
          </div>
          <div className="select-wrap sort-wrap">
            <select
              className="control sort-select"
              value={sort}
              onChange={e => setSort(e.target.value as SortKey)}
              aria-label="Сортировка"
            >
              <option value="recent">Сначала новые</option>
              <option value="name">По имени (А–Я)</option>
            </select>
          </div>
        </div>
      )}

      {loading ? (
        <ul className="rows" aria-hidden="true">
          {[0, 1, 2, 3].map(i => (
            <li key={i} className="row row-skeleton">
              <span className="row-status">
                <span className="sk sk-pill" />
              </span>
              <span className="sk-id">
                <span className="sk sk-name" />
                <span className="sk sk-sub" />
              </span>
              <span className="sk sk-data" />
            </li>
          ))}
        </ul>
      ) : sims.length === 0 ? (
        <div className="empty">
          <p className="empty-title">Расчётов пока нет</p>
          <p className="empty-help">
            Загрузите архив задачи в блоке <strong>Новый расчёт</strong> и выберите
            решатель, чтобы запустить первый расчёт на кластере.
          </p>
        </div>
      ) : visible.length === 0 ? (
        <div className="empty">
          <p className="empty-title">Ничего не найдено по фильтрам</p>
          <button className="btn-ghost" onClick={() => { setQuery(''); setStatusFilter('all'); }}>
            Сбросить фильтры
          </button>
        </div>
      ) : (
        <ul className="rows">
          {visible.map(sim => {
            const algo = algorithmTag(sim.SchedulerName);
            const time = rowTime(sim);
            const isSel = selected === sim.ID;
            const isConfirming = confirmId === sim.ID;
            return (
              <li key={sim.ID}>
                <div
                  className={`row${isSel ? ' is-selected' : ''}`}
                  role="button"
                  tabIndex={0}
                  aria-pressed={isSel}
                  onClick={() => setSelected(sim.ID)}
                  onKeyDown={e => onRowKey(e, sim.ID)}
                >
                  <span className="row-status">
                    <StatusPill status={sim.Status} />
                  </span>
                  <div className="row-id">
                    <div className="row-name">{sim.Name}</div>
                    <div className="row-sub">
                      {SOLVER_LABEL[sim.Type]} <span className="dot-sep">·</span> {algo.full}
                    </div>
                  </div>

                  {isConfirming ? (
                    <div className="row-confirm" onClick={e => e.stopPropagation()}>
                      <span className="confirm-q">Удалить?</span>
                      <button className="confirm-yes" onClick={() => confirmDelete(sim.ID)}>
                        Удалить
                      </button>
                      <button className="confirm-no" onClick={() => setConfirmId(null)}>
                        Отмена
                      </button>
                    </div>
                  ) : (
                    <>
                      <div className="row-data">
                        <span className="metric mono" title="Процессов MPI">
                          <span className="metric-k">MPI</span> {sim.NumProcs}
                        </span>
                        <span className="metric mono" title={time.label}>
                          {time.value}
                        </span>
                      </div>
                      <div className="row-actions">
                        {sim.Status === 'completed' && (
                          <button
                            className="row-view"
                            onClick={(e: MouseEvent) => { e.stopPropagation(); setVizSim(sim); }}
                            title="Открыть результат"
                            aria-label={`Открыть результат ${sim.Name}`}
                          >
                            <IconEye size={15} />
                            <span className="row-view-label">Результат</span>
                          </button>
                        )}
                        <button
                          className="row-del"
                          onClick={(e: MouseEvent) => { e.stopPropagation(); setConfirmId(sim.ID); }}
                          title="Удалить расчёт"
                          aria-label={`Удалить ${sim.Name}`}
                        >
                          <IconTrash size={15} />
                        </button>
                      </div>
                    </>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}

      {!loading && sims.length > 0 && (
        <div className="list-foot">
          <button className="btn-ghost" onClick={() => load()}>
            <IconRefresh size={14} />
            Обновить
          </button>
          {filtersActive && (
            <span className="foot-count mono">
              {visible.length} / {sims.length}
            </span>
          )}
        </div>
      )}

      {vizSim && (
        <Modal
          title={<>Результат · <span className="mono">{vizSim.Name}</span></>}
          onClose={() => setVizSim(null)}
        >
          <Suspense fallback={<div className="viz-fallback">Загрузка 3D-движка…</div>}>
            <Visualizer sim={vizSim} />
          </Suspense>
        </Modal>
      )}
    </div>
  );
}
