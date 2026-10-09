import { runtimeStorageKey } from './api';
import type { Session } from './types';

const key = runtimeStorageKey('mctrl-recent-session');
interface RecentSession { id: string; name: string; workId?: string }
export function rememberSession(session: Session): void {
  try {
    const item: RecentSession = { id: session.id, name: session.name, workId: (session.managed_work ?? session.work)?.id };
    window.localStorage.setItem(key, JSON.stringify(item));
  } catch { /* Resume is a convenience; blocked storage does not affect Work. */ }
}
export function recentSession(sessions: Session[]): Session | undefined {
  try {
    const value = JSON.parse(window.localStorage.getItem(key) ?? 'null') as RecentSession | null;
    // tmux IDs can be reused after a server restart. Match the name and, for
    // Managed Work, the Work identity before offering a resume action.
    return value ? sessions.find(session => session.id === value.id && session.name === value.name && (session.managed_work ?? session.work)?.id === value.workId) : undefined;
  } catch { return undefined; }
}
