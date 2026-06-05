import { SimulationStatus, SimulationType } from '../types';

export type Tone = 'lime' | 'cyan' | 'green' | 'red';

export interface StatusMeta {
  label: string;
  tone: Tone;
  pulse: boolean;
}

export const STATUS_META: Record<SimulationStatus, StatusMeta> = {
  pending: { label: 'В очереди', tone: 'cyan', pulse: false },
  running: { label: 'Выполняется', tone: 'lime', pulse: true },
  completed: { label: 'Завершён', tone: 'green', pulse: false },
  failed: { label: 'Ошибка', tone: 'red', pulse: false },
};

export const SOLVER_LABEL: Record<SimulationType, string> = {
  openfoam: 'OpenFOAM',
  openradioss: 'OpenRadioss',
  code_aster: 'Code_Aster',
};

/** User-facing load-distribution mode derived from the k8s scheduler name.
 *  Mirrors the friendly modes in the run form (Точный / Сбалансированный / …). */
export function algorithmTag(schedulerName: string): { short: string; full: string } {
  const s = (schedulerName || '').toLowerCase();
  if (s.includes('mueller') || s.includes('merbach') || s === 'mm') {
    return { short: 'точный', full: 'Точный' };
  }
  if (s.includes('random')) return { short: 'быстрый', full: 'Быстрый' };
  if (s.includes('topology') || s.includes('greedy')) {
    return { short: 'сбаланс.', full: 'Сбалансированный' };
  }
  return { short: 'станд.', full: 'Стандартный' };
}

/** mm:ss (or h:mm:ss) elapsed between two ISO timestamps; `to` defaults to now. */
export function duration(fromISO?: string, toISO?: string): string {
  if (!fromISO) return '—';
  const from = new Date(fromISO).getTime();
  const to = toISO ? new Date(toISO).getTime() : Date.now();
  let secs = Math.max(0, Math.round((to - from) / 1000));
  const h = Math.floor(secs / 3600);
  secs -= h * 3600;
  const m = Math.floor(secs / 60);
  const s = secs - m * 60;
  const pad = (n: number) => String(n).padStart(2, '0');
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

/** Relative time for created timestamps ("3 мин назад"). */
export function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.round(diff / 60000);
  if (mins < 1) return 'только что';
  if (mins < 60) return `${mins} мин назад`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs} ч назад`;
  return `${Math.round(hrs / 24)} дн назад`;
}
