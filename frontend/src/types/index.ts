export type SimulationType = 'cfd' | 'fea';

export type SimulationStatus = 'pending' | 'running' | 'completed' | 'failed';

export interface Simulation {
  ID: string;           
  Name: string;         
  Type: SimulationType;
  Status: SimulationStatus;
  ResultPath: string;
  CreatedAt: string;
  StartedAt?: string;
  CompletedAt?: string;
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