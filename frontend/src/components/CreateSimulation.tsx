import { useState } from 'react';
import { simulationAPI } from '../services/api';
import { Toast } from './Toast';
import './CreateSimulation.css';

export function CreateSimulation({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('');
  const [type, setType] = useState<'cfd' | 'fea'>('cfd');
  const [file, setFile] = useState<File | null>(null);
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!file) {
      setToast({ message: 'Please select a file', type: 'error' });
      return;
    }
    
    setLoading(true);
    try {
      await simulationAPI.createWithFile(name, type, file);
      setName('');
      setFile(null);
      setToast({ message: 'Simulation created successfully!', type: 'success' });
      onCreated();
    } catch (err) {
      setToast({ 
        message: err instanceof Error ? err.message : 'Failed to create simulation',
        type: 'error'
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
            placeholder="My Simulation"
          />
        </div>
        <div className="form-group">
          <label>Type</label>
          <select value={type} onChange={e => setType(e.target.value as 'cfd' | 'fea')}>
            <option value="cfd">CFD (OpenFOAM .tar.gz)</option>
            <option value="fea">FEA (CalculiX .inp)</option>
          </select>
        </div>
        <div className="form-group">
          <label>File ({type === 'cfd' ? '.tar.gz' : '.inp'})</label>
          <input
            type="file"
            accept={type === 'cfd' ? '.tar.gz' : '.inp'}
            onChange={e => setFile(e.target.files?.[0] || null)}
            required
          />
        </div>
        <button type="submit" disabled={loading}>
          {loading ? 'Uploading...' : 'Create'}
        </button>
      </form>
    </>
  );
}