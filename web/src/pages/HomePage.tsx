import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { getHost, getProjects, getSessionPreview, getSessions, getWork } from '../api';
import type { Host, ManagedWork, Preview, Project, Session } from '../types';
import { sessionPath } from '../router';
import { recentSession } from '../session-history';
import { isActiveWork } from '../work-status';
import { CopyText } from '../components/CopyText';
import { sessionDirectory } from '../session-directory';
import { previewContent } from '../preview-text';
import { useForegroundRefresh } from '../use-foreground-refresh';
import {
  EmptyState,
  ErrorNotice,
  LoadingBlock,
  StatusDot,
} from '../components/ui';

// mctrl already knows whether a Managed Work is alive, so the Home list says so.
// Without it every card looks identical and "is it still running?" costs a tap
// into the Session page to answer. External tmux Sessions have no Work, so they
// get no chip rather than a misleading one.
function WorkStateChip({ work }: { work: ManagedWork }) {
  const state = work.state ?? '';
  if (work.archived_at) {
    return <span class="work-state-chip exited">Archived</span>;
  }
  if (work.recovery_status && !['EXITED', 'LAUNCH_FAILED'].includes(state)) {
    return <span class="work-state-chip failed">Needs review</span>;
  }
  const tone =
    state === 'RUNNING'
      ? 'running'
      : state === 'LAUNCH_FAILED'
        ? 'failed'
        : state === 'EXITED'
          ? 'exited'
          : 'pending';
  const detail =
    state === 'EXITED'
      ? work.exit_code === undefined || work.exit_code === null
        ? 'EXITED'
        : `EXITED · code ${work.exit_code}`
      : work.error_code
        ? `${state} · ${work.error_code}`
        : state;
  return (
    <span class={`work-state-chip ${tone}`} title={`Managed Work is ${state}`}>
      {detail || 'Managed Work'}
    </span>
  );
}

function SessionCard({
  session,
  preview,
  projects,
}: {
  session: Session;
  preview?: Preview;
  projects: Project[];
}) {
  const command = session.active_command || session.active_pane?.command;
  const cwd = sessionDirectory(session).path;
  const recentLines = previewContent(preview?.lines ?? []).slice(-2);
  const work = session.managed_work ?? session.work;
  const project = projects.find(project => project.id === work?.project_id);

  return (
    <a class="session-card" href={`#${sessionPath(session.id)}`}>
      <div class="session-card-topline">
        <div>
          <h3>{project?.name ?? session.name}</h3>
          {project && <p class="session-human-context">{work?.runner_id} · {session.name}</p>}
          <div class="session-badges">
            <p className="session-origin-badge compact">
              {work ? 'Managed' : 'External tmux'}
            </p>
            {work && <WorkStateChip work={work} />}
          </div>
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
  const [projects, setProjects] = useState<Project[]>([]);
  const [attentionCount, setAttentionCount] = useState(0);
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

      const [projectResult, workResult] = await Promise.allSettled([getProjects(), getWork()]);
      if (currentLoad !== loadId.current) return;
      if (projectResult.status === 'fulfilled') setProjects(projectResult.value);
      if (workResult.status === 'fulfilled') setAttentionCount(workResult.value.filter(work => isActiveWork(work) && work.recovery_status).length);

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
  useForegroundRefresh(() => void load(true));
  const recent = recentSession(sessions);
  const recentProject = projects.find(project => project.id === (recent?.managed_work ?? recent?.work)?.project_id);
  const recentWork = recent?.managed_work ?? recent?.work;
  const recentEnded = ['EXITED', 'LAUNCH_FAILED'].includes(recentWork?.state ?? '') || recent?.active_pane?.dead;

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

      {recent && !error && <a class="resume-card card" href={`#${sessionPath(recent.id, !recentEnded)}`}>
        <span class="section-kicker">{recentEnded ? 'Review last session' : 'Continue last session'}</span>
        <strong>{recentProject?.name ?? recent.name}</strong>
        <span>{recentProject ? `${(recent.managed_work ?? recent.work)?.runner_id} · ` : ''}{sessionDirectory(recent).path}</span>
        <span class="resume-action">{recentEnded ? 'View output' : 'Open terminal'} ›</span>
      </a>}

      {attentionCount > 0 && <a class="attention-link" href="#/settings?section=recovery">{attentionCount} Work {attentionCount === 1 ? 'needs' : 'need'} review · Inspect records ›</a>}

      <a class="start-work-cta" href="#/start">
        <span class="start-work-icon" aria-hidden="true">
          +
        </span>
        <span>
          <strong>Start Work</strong>
          <small>Open a shell or coding harness</small>
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
                projects={projects}
              />
            ))}
          </div>
        )}
      </section>
      <details class="desktop-handoff card">
        <summary>Continue work from your Mac</summary>
        <p>In WezTerm or another Mac terminal, go to your project directory and run this command. Then open your harness as usual. Its session will appear here automatically.</p>
        {host?.desktop_start_command && <CopyText label="Mac start command" value={host.desktop_start_command} />}
        <p>Run it again in the same directory to continue the same session. To leave it running, press Ctrl+B, then D. A program already running outside tmux needs a new session. Each Session page has a command to continue on your Mac.</p>
      </details>
    </div>
  );
}
