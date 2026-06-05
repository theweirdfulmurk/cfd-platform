import { SimulationStatus } from '../types';
import { STATUS_META } from '../lib/format';
import './StatusPill.css';

/** Status is always a dot plus a word, never color alone (color-vision safe,
 *  and it survives a grayscale screenshot). Running pulses. */
export function StatusPill({ status }: { status: SimulationStatus }) {
  const meta = STATUS_META[status];
  return (
    <span className={`pill pill-${meta.tone}`} role="status">
      <span className="pill-dot" data-pulse={meta.pulse ? '' : undefined} />
      {meta.label}
    </span>
  );
}
