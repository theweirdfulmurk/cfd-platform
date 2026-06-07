export type SimulationType = 'openfoam' | 'openradioss' | 'code_aster';

export type SimulationStatus = 'pending' | 'running' | 'completed' | 'failed';

export type SchedulerChoice = 'default' | 'random' | 'topology-aware' | 'mueller-merbach';

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

// Real result field descriptor written by the post-solve export step
// (experiment/export_surface.sh) and served from GET /simulations/{id}/field-stats.
// `name` matches the array name in surface.vtp so the viewer can colour by it;
// the numeric range/mean are derived live from the .vtp (single source of truth,
// so the legend always matches what is actually rendered).
export interface FieldStat {
  name: string;
  label: string;
  unit: string;
  // Display multiplier applied to the raw .vtp values for the legend/CSV
  // (default 1). The .vtp keeps raw solver units; `scale` converts to the
  // engineering unit shown: Code_Aster Pa→МПа (1e-6), m→мм (1e3); OpenFOAM
  // kinematic pressure stays м²/с² (1).
  scale?: number;
}

export interface FieldStats {
  fields: FieldStat[];
  cells?: number;
  surface?: string;
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
