import { useEffect, useMemo, useRef, useState } from 'react';
import { Simulation, SimulationType, FieldStat, FieldStats } from '../types';
import { StatusPill } from './StatusPill';
import { useToast } from './ToastProvider';
import { IconCube, IconDownload } from './icons';
import { SOLVER_LABEL, STATUS_META, algorithmTag, duration } from '../lib/format';
import { simulationAPI } from '../services/api';
import './Visualizer.css';

import '@kitware/vtk.js/Rendering/Profiles/Geometry';
import vtkGenericRenderWindow from '@kitware/vtk.js/Rendering/Misc/GenericRenderWindow';
import vtkPlaneSource from '@kitware/vtk.js/Filters/Sources/PlaneSource';
import vtkMapper from '@kitware/vtk.js/Rendering/Core/Mapper';
import vtkActor from '@kitware/vtk.js/Rendering/Core/Actor';
import vtkColorTransferFunction from '@kitware/vtk.js/Rendering/Core/ColorTransferFunction';
import vtkColorMaps from '@kitware/vtk.js/Rendering/Core/ColorTransferFunction/ColorMaps';
import vtkDataArray from '@kitware/vtk.js/Common/Core/DataArray';
import vtkXMLPolyDataReader from '@kitware/vtk.js/IO/XML/XMLPolyDataReader';

const BG: [number, number, number] = [0.055, 0.06, 0.07];

interface SynthField {
  label: string;
  unit: string;
  min: number;
  max: number;
  freq: number;
}

// Fallback fields: used only when the run has no exported surface (other
// solvers, or an OpenFOAM run without the post-solve export). They label the
// synthetic preview with a plausible name, unit and numeric range.
const FIELDS: Record<SimulationType, SynthField[]> = {
  openfoam: [
    // simpleFoam p is KINEMATIC pressure (p/ρ), unit м²/с² — not Pa.
    { label: 'Давление', unit: 'м²/с²', min: -420, max: 220, freq: 3.0 },
    { label: 'Скорость', unit: 'м/с', min: 0, max: 30, freq: 4.3 },
    { label: 'Турбулентная энергия', unit: 'м²/с²', min: 0, max: 6.4, freq: 5.2 },
  ],
  openradioss: [
    { label: 'Напряжение по Мизесу', unit: 'МПа', min: 0, max: 412, freq: 3.6 },
    { label: 'Перемещение', unit: 'мм', min: 0, max: 38, freq: 2.7 },
    { label: 'Пластическая деформация', unit: '—', min: 0, max: 0.21, freq: 4.8 },
  ],
  code_aster: [
    { label: 'Напряжение по Мизесу', unit: 'МПа', min: 0, max: 286, freq: 3.2 },
    { label: 'Перемещение', unit: 'мм', min: 0, max: 1.7, freq: 2.4 },
  ],
};

// Shared decimal precision for a field, so both ends of the legend match.
function decimalsFor(max: number): number {
  const a = Math.abs(max);
  if (a >= 100) return 0;
  if (a >= 1) return 1;
  return 2;
}

// Locate a named array in the polydata, preferring point data (smooth
// shading) and falling back to cell data (foamToVTK writes FV fields as
// cell data on the patch faces).
function findArray(pd: any, name: string): { array: any; location: 'point' | 'cell' } | null {
  const point = pd.getPointData().getArrayByName(name);
  if (point) return { array: point, location: 'point' };
  const cell = pd.getCellData().getArrayByName(name);
  if (cell) return { array: cell, location: 'cell' };
  return null;
}

// Range [min,max] of the field as rendered: magnitude for vectors, value for
// scalars. The single source of truth for the legend (matches the colour bar).
function fieldRange(pd: any, name: string): [number, number] {
  const found = findArray(pd, name);
  if (!found) return [0, 1];
  const comps = found.array.getNumberOfComponents();
  const r = found.array.getRange(comps > 1 ? -1 : 0);
  return [r[0], r[1]];
}

// Min/max/mean of the field (mean of magnitude for vectors) for the CSV export.
function fieldSummary(pd: any, name: string): { min: number; max: number; mean: number } {
  const found = findArray(pd, name);
  if (!found) return { min: 0, max: 0, mean: 0 };
  const comps = found.array.getNumberOfComponents();
  const data = found.array.getData() as ArrayLike<number>;
  const n = found.array.getNumberOfTuples();
  let min = Infinity;
  let max = -Infinity;
  let sum = 0;
  for (let i = 0; i < n; i++) {
    let v: number;
    if (comps > 1) {
      let s = 0;
      for (let c = 0; c < comps; c++) {
        const x = data[i * comps + c];
        s += x * x;
      }
      v = Math.sqrt(s);
    } else {
      v = data[i];
    }
    if (v < min) min = v;
    if (v > max) max = v;
    sum += v;
  }
  return { min, max, mean: n ? sum / n : 0 };
}

/** Render the REAL exported surface coloured by the selected field. */
function buildRealScene(
  container: HTMLDivElement,
  polydata: any,
  fieldName: string,
  range: [number, number],
) {
  const grw = vtkGenericRenderWindow.newInstance({ background: BG });
  grw.setContainer(container);
  const renderer = grw.getRenderer();
  const renderWindow = grw.getRenderWindow();

  const mapper = vtkMapper.newInstance({ interpolateScalarsBeforeMapping: true });
  mapper.setInputData(polydata);

  const found = findArray(polydata, fieldName);
  if (found) {
    const comps = found.array.getNumberOfComponents();
    const ctf = vtkColorTransferFunction.newInstance();
    ctf.applyColorMap(vtkColorMaps.getPresetByName('Cool to Warm'));
    if (comps > 1) ctf.setVectorModeToMagnitude();
    ctf.setMappingRange(range[0], range[1]);
    ctf.updateRange();

    mapper.setLookupTable(ctf as never);
    mapper.setScalarRange(range[0], range[1]);
    mapper.setColorByArrayName(fieldName);
    mapper.setColorModeToMapScalars();
    if (found.location === 'point') mapper.setScalarModeToUsePointFieldData();
    else mapper.setScalarModeToUseCellFieldData();
  }

  const actor = vtkActor.newInstance();
  actor.setMapper(mapper);
  renderer.addActor(actor);

  renderer.resetCamera();
  const cam = renderer.getActiveCamera();
  cam.azimuth(-35);
  cam.elevation(20);
  renderer.resetCameraClippingRange();
  grw.resize();
  renderWindow.render();

  const onResize = () => grw.resize();
  window.addEventListener('resize', onResize);
  return () => {
    window.removeEventListener('resize', onResize);
    grw.delete();
  };
}

/** Render a synthetic warped surface so the viewport still shows an interactive
 *  3D result when no exported surface exists. `freq` varies the pattern so
 *  switching the field visibly recolours the model. */
function buildSyntheticScene(container: HTMLDivElement, freq: number) {
  const grw = vtkGenericRenderWindow.newInstance({ background: BG });
  grw.setContainer(container);
  const renderer = grw.getRenderer();
  const renderWindow = grw.getRenderWindow();

  const plane = vtkPlaneSource.newInstance({
    xResolution: 96,
    yResolution: 96,
    origin: [-1, -1, 0],
    point1: [1, -1, 0],
    point2: [-1, 1, 0],
  });
  const pd = plane.getOutputData();
  const points = pd.getPoints();
  const coords = points.getData() as Float32Array;
  const nPts = points.getNumberOfPoints();
  const scalars = new Float32Array(nPts);

  for (let i = 0; i < nPts; i++) {
    const x = coords[i * 3];
    const y = coords[i * 3 + 1];
    const r2 = x * x + y * y;
    const z =
      0.26 * Math.exp(-r2 * 2.1) * Math.cos(x * freq) +
      0.12 * Math.sin(x * (freq + 1.2) + y * (freq - 0.6)) * Math.exp(-r2 * 1.0);
    coords[i * 3 + 2] = z;
    scalars[i] = z;
  }
  points.modified();
  pd.modified();
  pd.getPointData().setScalars(vtkDataArray.newInstance({ name: 'field', values: scalars }));

  let min = Infinity;
  let max = -Infinity;
  for (let i = 0; i < nPts; i++) {
    const v = scalars[i];
    if (v < min) min = v;
    if (v > max) max = v;
  }

  const ctf = vtkColorTransferFunction.newInstance();
  ctf.applyColorMap(vtkColorMaps.getPresetByName('Cool to Warm'));
  ctf.setMappingRange(min, max);
  ctf.updateRange();

  const mapper = vtkMapper.newInstance({ interpolateScalarsBeforeMapping: true });
  mapper.setInputData(pd);
  mapper.setLookupTable(ctf as never);
  mapper.setScalarRange(min, max);

  const actor = vtkActor.newInstance();
  actor.setMapper(mapper);

  renderer.addActor(actor);
  renderer.resetCamera();
  const cam = renderer.getActiveCamera();
  cam.azimuth(-32);
  cam.elevation(26);
  renderer.resetCameraClippingRange();
  grw.resize();
  renderWindow.render();

  const onResize = () => grw.resize();
  window.addEventListener('resize', onResize);
  return () => {
    window.removeEventListener('resize', onResize);
    grw.delete();
  };
}

type Mode = 'loading' | 'real' | 'fallback';

export function Visualizer({ sim }: { sim: Simulation }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [mode, setMode] = useState<Mode>('loading');
  const [webglError, setWebglError] = useState(false);
  const [stats, setStats] = useState<FieldStats | null>(null);
  const polyRef = useRef<any>(null);
  const [fieldIdx, setFieldIdx] = useState(0);
  const { notify } = useToast();

  const synthFields = FIELDS[sim.Type];
  const realFields = stats?.fields ?? null;
  const algo = algorithmTag(sim.SchedulerName);

  // Real-mode raw range, derived SYNCHRONOUSLY during render (not via post-render
  // effect state) so the legend never paints a stale frame showing the previous
  // field's range under the new field's scale. polyRef is set together with stats
  // (→ realFields), so realFields identity changing is the right recompute signal.
  const realRange: [number, number] = useMemo(() => {
    if (mode === 'real' && polyRef.current && realFields) {
      const f = realFields[fieldIdx] ?? realFields[0];
      return fieldRange(polyRef.current, f.name);
    }
    return [0, 1];
  }, [mode, fieldIdx, realFields]);

  // The active field's display attributes, unified across real/fallback so the
  // legend and caption don't care which mode we're in. In real mode the numeric
  // range comes live from the rendered .vtp array (realRange), scaled to the
  // engineering display unit (e.g. Pa→МПа); the colour map still uses the raw
  // range, so scaling only affects the printed numbers.
  const active: { label: string; unit: string; min: number; max: number } =
    mode === 'real' && realFields
      ? (() => {
          const f = realFields[fieldIdx] ?? realFields[0];
          const sc = f.scale ?? 1;
          return { label: f.label, unit: f.unit, min: realRange[0] * sc, max: realRange[1] * sc };
        })()
      : synthFields[fieldIdx] ?? synthFields[0];
  const dec = decimalsFor(Math.max(Math.abs(active.min), Math.abs(active.max)));

  // Try the real exported surface first; fall back to the synthetic preview.
  useEffect(() => {
    let cancelled = false;
    setMode('loading');
    setWebglError(false);
    setFieldIdx(0);
    (async () => {
      try {
        const res = await fetch(simulationAPI.surfaceURL(sim.ID));
        if (!res.ok) throw new Error('no surface');
        const buf = await res.arrayBuffer();
        const reader = vtkXMLPolyDataReader.newInstance();
        reader.parseAsArrayBuffer(buf);
        const pd = reader.getOutputData(0);
        if (!pd || pd.getNumberOfPoints() === 0) throw new Error('empty surface');
        const st = await simulationAPI.fieldStats(sim.ID);
        // 'real' mode REQUIRES non-empty stats: without it realFields is null and
        // the component silently renders the synthetic scene under the real-surface
        // caption. Treat missing/empty stats as a real-path failure → clean fallback.
        if (!st || !st.fields || st.fields.length === 0) throw new Error('no field stats');
        if (cancelled) return;
        polyRef.current = pd;
        setStats(st);
        setMode('real');
      } catch {
        if (!cancelled) {
          polyRef.current = null;
          setStats(null);
          setMode('fallback');
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [sim.ID]);

  // (Re)build the scene whenever the mode or selected field changes.
  useEffect(() => {
    if (!containerRef.current) return;
    if (mode === 'loading') return;
    try {
      if (mode === 'real' && polyRef.current && realFields) {
        const f = realFields[fieldIdx] ?? realFields[0];
        return buildRealScene(containerRef.current, polyRef.current, f.name, realRange);
      }
      const f = synthFields[fieldIdx] ?? synthFields[0];
      return buildSyntheticScene(containerRef.current, f.freq);
    } catch {
      setWebglError(true);
    }
  }, [mode, fieldIdx, realRange]);

  function downloadResults() {
    const rows = [
      'Параметр,Значение',
      `Название,${sim.Name}`,
      `Решатель,${SOLVER_LABEL[sim.Type]}`,
      `Режим распределения,${algo.full}`,
      `Процессов MPI,${sim.NumProcs}`,
      `Статус,${STATUS_META[sim.Status].label}`,
    ];
    if (sim.Status === 'completed') {
      rows.push(`Длительность,${duration(sim.StartedAt, sim.CompletedAt)}`);
    }
    if (mode === 'real' && realFields && polyRef.current) {
      if (stats?.cells) rows.push(`Ячеек на поверхности,${stats.cells}`);
      rows.push('', 'Поле,Единица,Минимум,Максимум,Среднее');
      realFields.forEach((f: FieldStat) => {
        const sc = f.scale ?? 1;
        const s = fieldSummary(polyRef.current, f.name);
        const mn = s.min * sc, mx = s.max * sc, mean = s.mean * sc;
        const d = decimalsFor(Math.max(Math.abs(mn), Math.abs(mx)));
        rows.push(
          `${f.label},${f.unit},${mn.toFixed(d)},${mx.toFixed(d)},${mean.toFixed(d)}`,
        );
      });
    } else {
      rows.push('', 'Поле,Единица,Минимум,Максимум');
      synthFields.forEach(f => rows.push(`${f.label},${f.unit},${f.min},${f.max}`));
    }

    const csv = '﻿' + rows.join('\n');
    const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = `${sim.Name}-summary.csv`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    notify('Сводка результата скачана', 'success');
  }

  const fieldLabels = mode === 'real' && realFields
    ? realFields.map(f => f.label)
    : synthFields.map(f => f.label);

  return (
    <div className="viz">
      <div className="viz-stage">
        <div className="viz-canvas" ref={containerRef} />
        {webglError ? (
          <div className="viz-placeholder">
            <IconCube size={36} className="viz-glyph" />
            <p className="viz-ph-title">3D-просмотр недоступен</p>
            <p className="viz-ph-sub">Браузер не поддерживает WebGL.</p>
          </div>
        ) : mode === 'loading' ? (
          <div className="viz-placeholder">
            <IconCube size={36} className="viz-glyph" />
            <p className="viz-ph-title">Загрузка результата…</p>
          </div>
        ) : (
          <>
            <div className="viz-field">
              <select
                className="viz-field-select"
                value={fieldIdx}
                onChange={e => setFieldIdx(Number(e.target.value))}
                aria-label="Поле результата"
              >
                {fieldLabels.map((label, i) => (
                  <option key={label} value={i}>{label}</option>
                ))}
              </select>
            </div>

            <div className="viz-legend" aria-hidden="true">
              <span className="viz-legend-num">{active.max.toFixed(dec)}</span>
              <span className="viz-legend-bar" />
              <span className="viz-legend-num">{active.min.toFixed(dec)}</span>
              <span className="viz-legend-unit">{active.unit}</span>
            </div>

            <div className="viz-caption">
              {mode === 'real'
                ? `Поле «${active.label}» на поверхности модели. Вращайте мышью, колесо — масштаб.`
                : `Репрезентативная визуализация поля «${active.label}». Вращайте мышью, колесо — масштаб.`}
            </div>
          </>
        )}
      </div>

      <div className="viz-meta">
        <StatusPill status={sim.Status} />
        <span className="viz-meta-item">{SOLVER_LABEL[sim.Type]}</span>
        <span className="viz-meta-item">{algo.full}</span>
        <span className="viz-meta-item mono">MPI {sim.NumProcs}</span>
        {sim.Status === 'completed' && (
          <span className="viz-meta-item mono">
            {duration(sim.StartedAt, sim.CompletedAt)}
          </span>
        )}
        <button className="viz-download" onClick={downloadResults}>
          <IconDownload size={15} />
          Скачать результаты
        </button>
      </div>
    </div>
  );
}
