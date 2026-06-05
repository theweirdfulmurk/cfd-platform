import { Simulation } from '../types';

/* Representative sample rows shown when the backend API is unreachable
 * (e.g. viewing the console without a running cluster). Marked in the UI as
 * demo data so it is never mistaken for a live run. */

const minsAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();

export const DEMO_SIMULATIONS: Simulation[] = [
  {
    ID: 'd-1',
    Name: 'bracket-mm-04',
    Type: 'code_aster',
    Status: 'running',
    NumProcs: 16,
    SchedulerName: 'mueller-merbach',
    ResultPath: '/results/d-1',
    CreatedAt: minsAgo(5),
    StartedAt: minsAgo(4),
  },
  {
    ID: 'd-2',
    Name: 'motorbike-greedy-02',
    Type: 'openfoam',
    Status: 'running',
    NumProcs: 16,
    SchedulerName: 'topology-aware',
    ResultPath: '/results/d-2',
    CreatedAt: minsAgo(3),
    StartedAt: minsAgo(2),
  },
  {
    ID: 'd-3',
    Name: 'yaris-mm-01',
    Type: 'openradioss',
    Status: 'completed',
    NumProcs: 32,
    SchedulerName: 'mueller-merbach',
    ResultPath: '/results/d-3',
    CreatedAt: minsAgo(41),
    StartedAt: minsAgo(40),
    CompletedAt: minsAgo(28),
  },
  {
    ID: 'd-4',
    Name: 'motorbike-random-05',
    Type: 'openfoam',
    Status: 'completed',
    NumProcs: 16,
    SchedulerName: 'random',
    ResultPath: '/results/d-4',
    CreatedAt: minsAgo(63),
    StartedAt: minsAgo(62),
    CompletedAt: minsAgo(54),
  },
  {
    ID: 'd-5',
    Name: 'bracket-random-03',
    Type: 'code_aster',
    Status: 'failed',
    NumProcs: 16,
    SchedulerName: 'random',
    ResultPath: '/results/d-5',
    CreatedAt: minsAgo(77),
    StartedAt: minsAgo(76),
    CompletedAt: minsAgo(73),
  },
  {
    ID: 'd-6',
    Name: 'yaris-greedy-02',
    Type: 'openradioss',
    Status: 'pending',
    NumProcs: 32,
    SchedulerName: 'topology-aware',
    ResultPath: '/results/d-6',
    CreatedAt: minsAgo(1),
  },
];
