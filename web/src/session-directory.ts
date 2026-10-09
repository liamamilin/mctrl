import type { Session } from './types';

export function sessionDirectory(session: Session): { path: string; label: string } {
  const work = session.managed_work ?? session.work;
  if (work?.child_cwd) return { path: work.child_cwd, label: 'Current path' };
  const path = session.cwd || session.active_pane?.cwd || '';
  const command = session.active_command || session.active_pane?.command;
  return { path, label: work && command === 'mctrl-runner' ? 'Starting path' : 'Current path' };
}
