import type { ManagedWork } from './types';

export function isActiveWork(work: ManagedWork): boolean {
  return !work.archived_at && ['ACCEPTED', 'STARTING', 'RUNNING'].includes(work.state);
}

export function activeWorkCounts(works: ManagedWork[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const work of works) {
    if (isActiveWork(work) && work.project_id) {
      counts[work.project_id] = (counts[work.project_id] ?? 0) + 1;
    }
  }
  return counts;
}
