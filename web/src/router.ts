import { useEffect, useState } from 'preact/hooks';

export type Route =
  | { name: 'home' }
  | { name: 'pair'; pairingToken?: string }
  | { name: 'pair-console'; pairingToken?: string }
  | { name: 'link'; linkToken?: string }
  | { name: 'start' }
  | { name: 'settings' }
  | { name: 'session'; sessionId: string }
  | { name: 'terminal'; sessionId: string };

function decode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

export function parseRoute(hash: string): Route {
  const rawPath = hash.replace(/^#/, '');
  const [pathPart, queryPart = ''] = rawPath.split('?');
  const path = pathPart ?? '';
  const query = new URLSearchParams(queryPart);
  if (path.startsWith('token=')) {
    return { name: 'pair', pairingToken: decode(path.slice('token='.length)) };
  }
  const parts = path.split('/').filter(Boolean);

  if (parts[0] === 'pair') return { name: 'pair' };
  if (parts[0] === 'pair-console') {
    return { name: 'pair-console', pairingToken: query.get('token') ?? undefined };
  }
  if (parts[0] === 'link') {
    return { name: 'link', linkToken: query.get('token') ?? undefined };
  }
  if (parts[0] === 'start') return { name: 'start' };
  if (parts[0] === 'settings') return { name: 'settings' };

  if (parts[0] === 'sessions' && parts[1]) {
    const sessionId = decode(parts[1]);
    if (parts[2] === 'terminal') return { name: 'terminal', sessionId };
    return { name: 'session', sessionId };
  }

  return { name: 'home' };
}

export function useHashRoute(): Route {
  const [route, setRoute] = useState<Route>(() =>
    parseRoute(window.location.hash),
  );

  useEffect(() => {
    const onHashChange = () => setRoute(parseRoute(window.location.hash));
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  return route;
}

export function navigate(path: string): void {
  const next = path.startsWith('#') ? path : `#${path}`;
  if (window.location.hash === next) {
    window.dispatchEvent(new HashChangeEvent('hashchange'));
    return;
  }
  window.location.hash = next;
}

export function replaceRoute(path: string): void {
  const next = path.startsWith('#') ? path : `#${path}`;
  const url = `${window.location.pathname}${window.location.search}${next}`;
  window.history.replaceState(null, '', url);
  window.dispatchEvent(new HashChangeEvent('hashchange'));
}

export function sessionPath(sessionId: string, terminal = false): string {
  return `/sessions/${encodeURIComponent(sessionId)}${terminal ? '/terminal' : ''}`;
}
