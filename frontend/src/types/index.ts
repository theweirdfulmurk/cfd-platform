export type SimulationType = 'openfoam' | 'openradioss' | 'code_aster';

export type SimulationStatus = 'pending' | 'running' | 'completed' | 'failed';

export type SchedulerChoice = 'default' | 'topology-aware';

export interface Simulation {
  ID: string;
  Name: string;
  Type: SimulationType;
  Status: SimulationStatus;
  NumProcs: number;
  SchedulerName: string;
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
