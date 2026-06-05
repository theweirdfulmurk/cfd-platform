import { useEffect, useRef, useState } from 'react';
import { Simulation, SimulationType } from '../types';
import { StatusPill } from './StatusPill';
import { useToast } from './ToastProvider';
import { IconCube, IconDownload } from './icons';
import { SOLVER_LABEL, STATUS_META, algorithmTag, duration } from '../lib/format';
import './Visualizer.css';

import '@kitware/vtk.js/Rendering/Profiles/Geometry';
import vtkGenericRenderWindow from '@kitware/vtk.js/Rendering/Misc/GenericRenderWindow';
import vtkPlaneSource from '@kitware/vtk.js/Filters/Sources/PlaneSource';
import vtkMapper from '@kitware/vtk.js/Rendering/Core/Mapper';
import vtkActor from '@kitware/vtk.js/Rendering/Core/Actor';
import vtkColorTransferFunction from '@kitware/vtk.js/Rendering/Core/ColorTransferFunction';
import vtkColorMaps from '@kitware/vtk.js/Rendering/Core/ColorTransferFunction/ColorMaps';
import vtkDataArray from '@kitware/vtk.js/Common/Core/DataArray';

interface Field {
  label: string;
  unit: string;
  min: number;
  max: number;
  freq: number;
}

// Representative result fields per solver. With a real backend these would come
// from the exported .vtp (array names + real getRange()); here they label the
// synthetic demo field with a plausible name, units and numeric range.
const FIELDS: Record<SimulationType, Field[]> = {
  openfoam: [
    { label: 'Давление', unit: 'Па', min: -180, max: 2480, freq: 3.0 },
    { label: 'Скорость', unit: 'м/с', min: 0, max: 42, freq: 4.3 },
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

// Shared decimal precision for a field, so both ends of the legend match
// (e.g. 412 / 0 for stress, 1.7 / 0.0 for displacement, 0.21 / 0.00 for strain).
function decimalsFor(max: number): number {
  const a = Math.abs(max);
  if (a >= 100) return 0;
  if (a >= 1) return 1;
  return 2;
}

/** Render a synthetic warped surface coloured by a scalar so the viewport shows
 *  a real interactive 3D result without a cluster. `freq` varies the pattern so
 *  switching the field visibly recolours the model. */
function buildScene(container: HTMLDivElement, freq: number) {
  const grw = vtkGenericRenderWindow.newInstance({ background: [0.055, 0.06, 0.07] });
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

export function Visualizer({ sim }: { sim: Simulation }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [error, setError] = useState(false);
  const [fieldIdx, setFieldIdx] = useState(0);
  const { notify } = useToast();

  const fields = FIELDS[sim.Type];
  const field = fields[fieldIdx] ?? fields[0];
  const dec = decimalsFor(field.max);
  const algo = algorithmTag(sim.SchedulerName);

  function downloadResults() {
    // Demo: a results summary the user can inspect numerically. With a backend
    // this button would stream the full result archive from
    // GET /api/simulations/{id}/results instead.
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
    rows.push('', 'Поле,Единица,Минимум,Максимум');
    fields.forEach(f => rows.push(`${f.label},${f.unit},${f.min},${f.max}`));

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

  useEffect(() => {
    if (!containerRef.current) return;
    try {
      return buildScene(containerRef.current, field.freq);
    } catch {
      setError(true);
    }
  }, [field.freq]);

  return (
    <div className="viz">
      <div className="viz-stage">
        <div className="viz-canvas" ref={containerRef} />
        {error ? (
          <div className="viz-placeholder">
            <IconCube size={36} className="viz-glyph" />
            <p className="viz-ph-title">3D-просмотр недоступен</p>
            <p className="viz-ph-sub">Браузер не поддерживает WebGL.</p>
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
                {fields.map((f, i) => (
                  <option key={f.label} value={i}>{f.label}</option>
                ))}
              </select>
            </div>

            <div className="viz-legend" aria-hidden="true">
              <span className="viz-legend-num">{field.max.toFixed(dec)}</span>
              <span className="viz-legend-bar" />
              <span className="viz-legend-num">{field.min.toFixed(dec)}</span>
              <span className="viz-legend-unit">{field.unit}</span>
            </div>

            <div className="viz-caption">
              Демо: синтетическое поле «{field.label}». Вращайте мышью, колесо — масштаб.
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
