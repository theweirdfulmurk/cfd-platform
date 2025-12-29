import { useState, useEffect } from 'react';
import { Simulation } from '../types';
import { simulationAPI } from '../services/api';
import './SimulationList.css';

export function SimulationList() {
  const [simulations, setSimulations] = useState<Simulation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadSimulations();
    const interval = setInterval(loadSimulations, 5000);
    return () => clearInterval(interval);
  }, []);

  async function loadSimulations() {
    try {
      const data = await simulationAPI.list();
      setSimulations(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(id: string) {
    if (!confirm('Delete simulation?')) return;
    try {
      await simulationAPI.delete(id);
      setSimulations(sims => sims.filter(s => s.ID !== id));
    } catch (err) {
      alert('Failed to delete');
    }
  }

  if (loading) return <div className="loading">Loading...</div>;
  if (error) return <div className="error">{error}</div>;

  return (
    <div className="simulation-list">
      <h2>Simulations</h2>
      {simulations.length === 0 ? (
        <p>No simulations yet</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Status</th>
              <th>Created</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {simulations.map(sim => (
              <tr key={sim.ID}>
                <td>{sim.Name}</td>
                <td>{sim.Type.toUpperCase()}</td>
                <td>
                  <span className={`status status-${sim.Status}`}>
                    {sim.Status}
                  </span>
                </td>
                <td>{new Date(sim.CreatedAt).toLocaleString()}</td>
                <td>
                  <button onClick={() => handleDelete(sim.ID)}>Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}