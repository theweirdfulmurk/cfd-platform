export type SimulationType = 'cfd' | 'fea';

export type SimulationStatus = 'pending' | 'running' | 'completed' | 'failed';

export interface Simulation {
  id: string;
  name: string;
  type: SimulationType;
  status: SimulationStatus;
  resultPath: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
}

export type VisualizationStatus = 'pending' | 'running' | 'ready' | 'failed';

export interface Visualization {
  id: string;
  simulationId: string;
  status: VisualizationStatus;
  podName: string;
  webSocketURL?: string;
  resultPath: string;
  createdAt: string;
}