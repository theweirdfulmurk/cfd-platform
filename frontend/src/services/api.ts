import { Simulation, Visualization } from '../types';

const API_BASE = '/api';

export const simulationAPI = {
  async createWithFile(name: string, type: 'cfd' | 'fea', file: File): Promise<Simulation> {
    const formData = new FormData();
    formData.append('name', name);
    formData.append('type', type);
    formData.append('file', file);

    const res = await fetch(`${API_BASE}/simulations`, {
      method: 'POST',
      body: formData,
    });
    if (!res.ok) {
      const error = await res.json();
      throw new Error(error.error || `Failed to create simulation: ${res.statusText}`);
    }
    return res.json();
  },

  async create(name: string, type: 'cfd' | 'fea', configPath: string): Promise<Simulation> {
    const res = await fetch(`${API_BASE}/simulations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, type, configPath }),
    });
    if (!res.ok) throw new Error(`Failed to create simulation: ${res.statusText}`);
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