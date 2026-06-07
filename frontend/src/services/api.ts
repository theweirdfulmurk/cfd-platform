import { Simulation, SimulationType, SchedulerChoice, Visualization, FieldStats } from '../types';

const API_BASE = '/api';

export interface CreateSimulationParams {
  name: string;
  type: SimulationType;
  numProcs: number;
  scheduler: SchedulerChoice;
  file: File;
}

export const simulationAPI = {
  async createWithFile(params: CreateSimulationParams): Promise<Simulation> {
    const formData = new FormData();
    formData.append('name', params.name);
    formData.append('type', params.type);
    formData.append('np', String(params.numProcs));
    formData.append('scheduler', params.scheduler);
    formData.append('file', params.file);

    const res = await fetch(`${API_BASE}/simulations`, {
      method: 'POST',
      body: formData,
    });
    if (!res.ok) {
      const error = await res.json().catch(() => ({}));
      throw new Error(error.error || `Failed to create simulation: ${res.statusText}`);
    }
    return res.json();
  },

  async list(): Promise<Simulation[]> {
    const res = await fetch(`${API_BASE}/simulations`);
    if (!res.ok) throw new Error('Failed to fetch simulations');
    return res.json();
  },

  async get(id: string): Promise<Simulation> {
    const res = await fetch(`${API_BASE}/simulations/${id}`);
    if (!res.ok) throw new Error('Simulation not found');
    return res.json();
  },

  async delete(id: string): Promise<void> {
    const res = await fetch(`${API_BASE}/simulations/${id}`, { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete simulation');
  },

  // URL of the exported result surface (.vtp). The Visualizer fetches it as an
  // ArrayBuffer and parses it with vtk.js; a 404 means no export yet → fallback.
  surfaceURL(id: string): string {
    return `${API_BASE}/simulations/${id}/surface`;
  },

  // Real per-field numeric summary; null when the run has no export.
  async fieldStats(id: string): Promise<FieldStats | null> {
    const res = await fetch(`${API_BASE}/simulations/${id}/field-stats`);
    if (!res.ok) return null;
    return res.json();
  },

  // URL of the raw results archive (mpiP report etc.), zipped by the backend.
  resultsURL(id: string): string {
    return `${API_BASE}/simulations/${id}/results`;
  },
};

export const visualizationAPI = {
  async create(simulationId: string, resultPath: string): Promise<Visualization> {
    const res = await fetch(`${API_BASE}/visualizations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ simulationId, resultPath }),
    });
    if (!res.ok) throw new Error(`Failed to create visualization: ${res.statusText}`);
    return res.json();
  },

  async get(id: string): Promise<Visualization> {
    const res = await fetch(`${API_BASE}/visualizations/${id}`);
    if (!res.ok) throw new Error('Visualization not found');
    return res.json();
  },

  async getWebSocketURL(id: string): Promise<string> {
    const res = await fetch(`${API_BASE}/visualizations/${id}/ws-url`);
    if (!res.ok) throw new Error('Failed to get WebSocket URL');
    const data = await res.json();
    return data.wsUrl;
  },

  async delete(id: string): Promise<void> {
    const res = await fetch(`${API_BASE}/visualizations/${id}`, { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete visualization');
  },
};
