import { useState, useRef, DragEvent } from 'react';
import { simulationAPI } from '../services/api';
import { SimulationType, SchedulerChoice } from '../types';
import { useToast } from './ToastProvider';
import { IconPlay, IconUpload } from './icons';
import './CreateSimulation.css';

const SOLVER_OPTIONS: { value: SimulationType; label: string }[] = [
  { value: 'openfoam', label: 'OpenFOAM — аэро- и гидродинамика' },
  { value: 'openradioss', label: 'OpenRadioss — динамика и удар' },
  { value: 'code_aster', label: 'Code_Aster — прочностной анализ' },
];

// User-facing load-distribution modes. Each maps to a real placement algorithm;
// the technical name is kept in the description for the defense, not the label.
const MODE_OPTIONS: { value: SchedulerChoice; label: string; desc: string }[] = [
  {
    value: 'mueller-merbach',
    label: 'Точный',
    desc: 'Размещает процессы под минимум обмена данными между ними: дольше готовит, но быстрее считает.',
  },
  {
    value: 'topology-aware',
    label: 'Сбалансированный',
    desc: 'Учитывает топологию кластера на лету — хороший компромисс скорости и качества.',
  },
  {
    value: 'random',
    label: 'Быстрый',
    desc: 'Случайное размещение без оптимизации — мгновенный старт.',
  },
  {
    value: 'default',
    label: 'Стандартный (Kubernetes)',
    desc: 'Планировщик Kubernetes по умолчанию, без учёта топологии.',
  },
];

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function CreateSimulation({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('');
  const [type, setType] = useState<SimulationType>('openfoam');
  const [parallel, setParallel] = useState(true);
  const [numProcs, setNumProcs] = useState<number>(16);
  const [mode, setMode] = useState<SchedulerChoice>('mueller-merbach');
  const [file, setFile] = useState<File | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [loading, setLoading] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const { notify } = useToast();

  const modeDesc = MODE_OPTIONS.find(m => m.value === mode)?.desc;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!file) {
      notify('Сначала выберите архив расчёта (.tar.gz)', 'error');
      return;
    }
    setLoading(true);
    try {
      await simulationAPI.createWithFile({
        name,
        type,
        numProcs: parallel ? numProcs : 1,
        scheduler: parallel ? mode : 'default',
        file,
      });
      setName('');
      setFile(null);
      notify('Расчёт запущен', 'success');
      onCreated();
    } catch (err) {
      notify(err instanceof Error ? err.message : 'Не удалось запустить расчёт', 'error');
    } finally {
      setLoading(false);
    }
  }

  function onDrop(e: DragEvent<HTMLLabelElement>) {
    e.preventDefault();
    setDragOver(false);
    const dropped = e.dataTransfer.files?.[0];
    if (dropped) setFile(dropped);
  }

  return (
    <form className="run-form" onSubmit={handleSubmit}>
      <div className="form-head">
        <h2 className="form-title">Новый расчёт</h2>
        <p className="form-sub">Запустите инженерный расчёт на кластере.</p>
      </div>

      <div className="field">
        <label htmlFor="sim-name">Название</label>
        <input
          id="sim-name"
          type="text"
          className="control"
          value={name}
          onChange={e => setName(e.target.value)}
          required
          placeholder="motorbike-01"
          autoComplete="off"
        />
      </div>

      <div className="field">
        <label htmlFor="sim-solver">Решатель</label>
        <div className="select-wrap">
          <select
            id="sim-solver"
            className="control"
            value={type}
            onChange={e => setType(e.target.value as SimulationType)}
          >
            {SOLVER_OPTIONS.map(o => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="field">
        <span className="field-label" id="case-label">Архив расчёта</span>
        <label
          className={`dropzone${dragOver ? ' is-drag' : ''}${file ? ' has-file' : ''}`}
          onDragOver={e => { e.preventDefault(); setDragOver(true); }}
          onDragLeave={() => setDragOver(false)}
          onDrop={onDrop}
          aria-labelledby="case-label"
        >
          <input
            ref={fileInput}
            type="file"
            accept=".tar.gz,.tgz"
            className="visually-hidden"
            onChange={e => setFile(e.target.files?.[0] || null)}
            required
          />
          <IconUpload size={18} className="dropzone-icon" />
          {file ? (
            <span className="dropzone-file">
              <span className="mono dropzone-name">{file.name}</span>
              <span className="dropzone-size">{formatBytes(file.size)}</span>
            </span>
          ) : (
            <span className="dropzone-prompt">
              Перетащите <code className="mono">.tar.gz</code> или <span className="link-ish">выберите</span>
            </span>
          )}
        </label>
      </div>

      <div className="field">
        <div className="toggle-row">
          <span className="toggle-text">
            <span className="toggle-name">Параллелизация вычислений</span>
            <span className="toggle-hint">
              Распределяет расчёт по нескольким процессам кластера — быстрее для крупных задач.
            </span>
          </span>
          <button
            type="button"
            role="switch"
            aria-checked={parallel}
            aria-label="Параллелизация вычислений"
            className="switch"
            data-on={parallel ? '' : undefined}
            onClick={() => setParallel(p => !p)}
          >
            <span className="switch-knob" />
          </button>
        </div>
      </div>

      {parallel && (
        <>
          <div className="field">
            <label htmlFor="sim-np">Число процессов</label>
            <input
              id="sim-np"
              type="number"
              className="control mono"
              min={2}
              max={1024}
              value={numProcs}
              onChange={e => setNumProcs(parseInt(e.target.value) || 2)}
              required
            />
          </div>

          <div className="field">
            <label htmlFor="sim-mode">Режим распределения нагрузки</label>
            <div className="select-wrap">
              <select
                id="sim-mode"
                className="control"
                value={mode}
                onChange={e => setMode(e.target.value as SchedulerChoice)}
              >
                {MODE_OPTIONS.map(o => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
            </div>
            {modeDesc && <p className="field-desc">{modeDesc}</p>}
          </div>
        </>
      )}

      <button type="submit" className="btn-primary" disabled={loading}>
        {loading ? (
          <><span className="spinner" aria-hidden="true" />Загрузка…</>
        ) : (
          <><IconPlay size={13} />Запустить расчёт</>
        )}
      </button>
    </form>
  );
}
