import { useState } from 'react';
import { simulationAPI } from '../services/api';
import { SimulationType, SchedulerChoice } from '../types';
import { Toast } from './Toast';
import './CreateSimulation.css';

const TYPE_LABELS: Record<SimulationType, string> = {
  openfoam: 'OpenFOAM (CFD, sparse graph)',
  openradioss: 'OpenRadioss (explicit FEM, sparse graph)',
  code_aster: 'Code_Aster (implicit FEM + MUMPS, dense graph)',
};

export function CreateSimulation({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('');
  const [type, setType] = useState<SimulationType>('openfoam');
  const [numProcs, setNumProcs] = useState<number>(4);
  const [scheduler, setScheduler] = useState<SchedulerChoice>('topology-aware');
  const [file, setFile] = useState<File | null>(null);
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!file) {
      setToast({ message: 'Please select a .tar.gz archive', type: 'error' });
      return;
    }

    setLoading(true);
    try {
      await simulationAPI.createWithFile({ name, type, numProcs, scheduler, file });
      setName('');
      setFile(null);
      setToast({ message: 'Simulation created', type: 'success' });
      onCreated();
    } catch (err) {
      setToast({
        message: err instanceof Error ? err.message : 'Failed to create simulation',
        type: 'error',
      });
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
      <form className="create-simulation" onSubmit={handleSubmit}>
        <h2>Create Simulation</h2>

        <div className="form-group">
          <label>Name</label>
          <input
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            required
            placeholder="motorbike-hpc-S"
          />
        </div>

        <div className="form-group">
          <label>Solver</label>
          <select value={type} onChange={e => setType(e.target.value as SimulationType)}>
            {(Object.keys(TYPE_LABELS) as SimulationType[]).map(t => (
              <option key={t} value={t}>{TYPE_LABELS[t]}</option>
            ))}
          </select>
        </div>

        <div className="form-group">
          <label>MPI ranks</label>
          <input
            type="number"
            min={1}
            max={1024}
            value={numProcs}
            onChange={e => setNumProcs(parseInt(e.target.value) || 1)}
            required
          />
        </div>

        <div className="form-group">
          <label>Scheduler</label>
          <select value={scheduler} onChange={e => setScheduler(e.target.value as SchedulerChoice)}>
            <option value="topology-aware">Topology-aware (ours)</option>
            <option value="default">Default kube-scheduler</option>
          </select>
        </div>

        <div className="form-group">
          <label>Case archive (.tar.gz)</label>
          <input
            type="file"
            accept=".tar.gz,.tgz"
            onChange={e => setFile(e.target.files?.[0] || null)}
            required
          />
        </div>

        <button type="submit" disabled={loading}>
          {loading ? 'Uploading…' : 'Submit'}
        </button>
      </form>
    </>
  );
}
