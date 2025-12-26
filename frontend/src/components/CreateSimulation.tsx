import { useState } from 'react';
import { simulationAPI } from '../services/api';
import './CreateSimulation.css';

export function CreateSimulation({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('');
  const [type, setType] = useState<'cfd' | 'fea'>('cfd');
  const [configPath, setConfigPath] = useState('');
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      await simulationAPI.create(name, type, configPath);
      setName('');
      setConfigPath('');
      onCreated();
    } catch (err) {
      alert('Failed to create simulation');
    } finally {
      setLoading(false);
    }
  }

  return (
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
          <option value="cfd">CFD (OpenFOAM)</option>
          <option value="fea">FEA (CalculiX)</option>
        </select>
      </div>
      <div className="form-group">
        <label>Config Path</label>
        <input
          type="text"
          value={configPath}
          onChange={e => setConfigPath(e.target.value)}
          required
          placeholder="motorBike"
        />
      </div>
      <button type="submit" disabled={loading}>
        {loading ? 'Creating...' : 'Create'}
      </button>
    </form>
  );
}