import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { getHost, getSessionPreview, getSessions } from '../api';
import type { Host, Preview, Session } from '../types';
import { sessionPath } from '../router';
import {
  EmptyState,
  ErrorNotice,
  LoadingBlock,
  StatusDot,
} from '../components/ui';

function SessionCard({
  session,
  preview,
}: {
  session: Session;
  preview?: Preview;
}) {
  const command = session.active_command || session.active_pane?.command;
  const cwd = session.cwd || session.active_pane?.cwd;
  const recentLines = preview?.lines.slice(-2) ?? [];

  return (
    <a class="session-card" href={`#${sessionPath(session.id)}`}>
      <div class="session-card-topline">
        <div>
          <h3>{session.name}</h3>
          {cwd && <p class="session-path">{cwd}</p>}
        </div>
        <span class="chevron" aria-hidden="true">
          ›
        </span>
      </div>

      <div class="session-meta">
        {command ? (
          <span class="command-chip">
            <span class="command-dot" aria-hidden="true" />
            {command}
          </span>
        ) : (
          <span class="muted-label">Command unavailable</span>
        )}
        <span>
          {session.windows} {session.windows === 1 ? 'window' : 'windows'}
        </span>
        <span>
          {session.panes} {session.panes === 1 ? 'pane' : 'panes'}
        </span>
      </div>

      {recentLines.length > 0 && (
        <div class="session-preview" aria-label="Recent output preview">
          {recentLines.map((line, index) => (
            <div key={`${index}-${line}`}>{line || ' '}</div>
          ))}
        </div>
      )}
    </a>
  );
}

export function HomePage() {
  const [host, setHost] = useState<Host>();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [previews, setPreviews] = useState<Record<string, Preview>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const loadId = useRef(0);

  const load = useCallback(async (background = false) => {
    const currentLoad = ++loadId.current;
    if (!background) {
      setLoading(true);
      setPreviews({});
    }
    setError('');

    try {
      const [nextHost, nextSessions] = await Promise.all([
        getHost(),
        getSessions(),
      ]);
      if (currentLoad !== loadId.current) return;
      setHost(nextHost);
      setSessions(nextSessions);
      setLoading(false);

      const entries = await Promise.all(
        nextSessions.map(async (session) => {
          try {
            return [session.id, await getSessionPreview(session.id, 20)] as const;
          } catch {
            return [session.id, undefined] as const;
          }
        }),
      );
      if (currentLoad !== loadId.current) return;
      setPreviews(
        Object.fromEntries(
          entries.filter(
            (entry): entry is readonly [string, Preview] => entry[1] !== undefined,
          ),
        ),
      );
    } catch (caught) {
      if (currentLoad !== loadId.current) return;
      setError(
        caught instanceof Error
          ? caught.message
          : 'Mac is currently unreachable.',
      );
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load(true);
    }, 10_000);
    return () => {
      window.clearInterval(timer);
      loadId.current += 1;
    };
  }, [load]);

  return (
    <div class="page-stack">
      <section class="host-hero" aria-labelledby="host-title">
        <div class="section-kicker">Host</div>
        <div class="host-hero-row">
          <div>
            <h1 id="host-title">{host?.name ?? 'Your Mac'}</h1>
            {host?.hostname && <p class="host-hostname">{host.hostname}</p>}
          </div>
          {host && <StatusDot status={host.status} pulse={host.status === 'online'} />}
        </div>
        <div class="host-facts">
          <span>
            <StatusDot status={host?.status ?? 'unknown'} />
            {host
              ? host.status.charAt(0).toUpperCase() + host.status.slice(1)
              : 'Checking'}
          </span>
          <span class="fact-divider" />
          <span>{host?.remote_ready ? 'Remote ready' : 'Remote not ready'}</span>
        </div>
      </section>

      <a class="start-work-cta" href="#/start">
        <span class="start-work-icon" aria-hidden="true">
          +
        </span>
        <span>
          <strong>Start Work</strong>
          <small>Launch a registered project</small>
        </span>
        <span class="cta-arrow" aria-hidden="true">
          ›
        </span>
      </a>

      <section class="section-block" aria-labelledby="sessions-title">
        <div class="section-heading">
          <div>
            <div class="section-kicker">Observe &amp; resume</div>
            <h2 id="sessions-title">
              Sessions
              {sessions.length > 0 && <span>{sessions.length}</span>}
            </h2>
          </div>
          <button
            class="icon-button"
            type="button"
            onClick={() => void load()}
            disabled={loading}
            aria-label="Refresh sessions"
            title="Refresh sessions"
          >
            ↻
          </button>
        </div>

        {loading ? (
          <LoadingBlock label="Loading sessions…" />
        ) : error ? (
          <ErrorNotice message={error} onRetry={() => void load()} />
        ) : sessions.length === 0 ? (
          <EmptyState
            title="No sessions yet"
            message="Start Work from a registered Project, or create a tmux session on the Mac."
            action={
              <a class="button button-secondary" href="#/start">
                Start Work
              </a>
            }
          />
        ) : (
          <div class="session-list">
            {sessions.map((session) => (
              <SessionCard
                key={session.id}
                session={session}
                preview={previews[session.id]}
              />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
