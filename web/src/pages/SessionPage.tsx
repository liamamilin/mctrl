import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  getSession,
  getSessionPreview,
  getWork,
} from '../api';
import { displayValue, formatDate, stateLabel } from '../format';
import type { ManagedWork, Preview, Session } from '../types';
import { sessionPath } from '../router';
import { ErrorNotice, LoadingBlock } from '../components/ui';

function Fact({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string | number | boolean | undefined;
  mono?: boolean;
}) {
  return (
    <div class="fact">
      <span>{label}</span>
      <strong class={mono ? 'mono' : undefined}>
        {displayValue(value)}
      </strong>
    </div>
  );
}

function WorkFacts({ work }: { work: ManagedWork }) {
  return (
    <section class="work-card card" aria-labelledby="work-title">
      <div class="card-title-row">
        <div>
          <div class="section-kicker">Associated process facts</div>
          <h2 id="work-title">Managed Work</h2>
        </div>
        <span class={`state-badge state-${work.state.toLowerCase()}`}>
          {stateLabel(work.state)}
        </span>
      </div>
      <div class="fact-grid">
        <Fact label="Work ID" value={work.id} mono />
        <Fact label="Runner" value={work.runner_id} mono />
        <Fact label="Runner PID" value={work.runner_pid} mono />
        <Fact label="Child PID" value={work.child_pid} mono />
        <Fact label="Keep awake" value={work.keep_awake} />
        <Fact label="Launch stage" value={work.launch_stage} />
        <Fact label="Prompt delivery" value={work.prompt_delivery} />
        <Fact label="Recovery status" value={work.recovery_status} />
        <Fact label="Started" value={formatDate(work.started_at ?? work.created_at)} />
        <Fact label="Exit code" value={work.exit_code} mono />
        <Fact label="Exit signal" value={work.exit_signal} mono />
      </div>
      {work.termination_reason && (
        <div class="inline-message warning">
          Termination reason: {work.termination_reason}
        </div>
      )}
      {work.error_code && (
        <div class="inline-message error" role="alert">
          {work.error_code}: {work.error_message ?? 'Launch or delivery error'}
        </div>
      )}
      {(work.state === 'EXITED' || work.state === 'LAUNCH_FAILED') &&
        work.project_id && (
          <a
            class="button button-secondary button-full start-another-work"
            href={`#/start?project=${encodeURIComponent(work.project_id)}`}
          >
            Start another Work
            <span aria-hidden="true">›</span>
          </a>
        )}
    </section>
  );
}

export function SessionPage({ sessionId }: { sessionId: string }) {
  const [session, setSession] = useState<Session>();
  const [preview, setPreview] = useState<Preview>();
  const [work, setWork] = useState<ManagedWork>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [previewError, setPreviewError] = useState('');
  const loadId = useRef(0);

  const load = useCallback(async (background = false) => {
    const currentLoad = ++loadId.current;
    if (!background) {
      setLoading(true);
      setPreviewError('');
    }
    setError('');

    const [sessionResult, previewResult, workResult] = await Promise.allSettled([
      getSession(sessionId),
      getSessionPreview(sessionId, 100),
      getWork(),
    ]);

    if (currentLoad !== loadId.current) return;
    if (sessionResult.status === 'rejected') {
      setError(
        sessionResult.reason instanceof Error
          ? sessionResult.reason.message
          : 'Could not load this Session.',
      );
      setLoading(false);
      return;
    }

    const nextSession = sessionResult.value;
    setSession(nextSession);
    if (previewResult.status === 'fulfilled') {
      setPreview(previewResult.value);
    } else {
      setPreviewError(
        previewResult.reason instanceof Error
          ? previewResult.reason.message
          : 'Recent output is unavailable.',
      );
    }

    if (workResult.status === 'fulfilled') {
      const associated =
        nextSession.managed_work ??
        nextSession.work ??
        workResult.value.find(
          (item) =>
            item.session_id === nextSession.id ||
            (item.session_name && item.session_name === nextSession.name),
        );
      setWork(associated);
    }
    setLoading(false);
  }, [sessionId]);

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

  const command = session?.active_command || session?.active_pane?.command;
  const cwd = session?.cwd || session?.active_pane?.cwd;

  return (
    <div class="page-stack">
      <header class="detail-header">
        <a class="back-link" href="#/">
          <span aria-hidden="true">‹</span> Sessions
        </a>
        <div class="detail-title-row">
          <div>
            <div class="section-kicker">Session detail</div>
            <h1>{session?.name ?? 'Session'}</h1>
          </div>
          <button
            class="icon-button"
            type="button"
            onClick={() => void load()}
            disabled={loading}
            aria-label="Refresh Session"
          >
            ↻
          </button>
        </div>
      </header>

      {loading ? (
        <LoadingBlock label="Loading Session facts…" />
      ) : error ? (
        <ErrorNotice message={error} onRetry={() => void load()} />
      ) : session ? (
        <>
          <section class="card session-facts-card">
            <div class="fact-grid">
              <Fact label="Session name" value={session.name} />
              <Fact label="Active command" value={command || 'Unavailable'} mono />
              <Fact label="Working directory" value={cwd || 'Unavailable'} mono />
              <Fact label="Windows" value={session.windows} mono />
              <Fact label="Panes" value={session.panes} mono />
              <Fact label="Attached" value={session.attached} />
            </div>
            <a
              class="button button-primary button-full open-terminal-button"
              href={`#${sessionPath(session.id, true)}`}
            >
              Open Terminal
              <span aria-hidden="true">›</span>
            </a>
          </section>

          {work && <WorkFacts work={work} />}

          <section class="preview-card card" aria-labelledby="preview-title">
            <div class="card-title-row">
              <div>
                <div class="section-kicker">Plain text</div>
                <h2 id="preview-title">Recent output</h2>
              </div>
              <span class="muted-label">
                {preview?.captured_at ? formatDate(preview.captured_at) : '100 lines'}
              </span>
            </div>
            {previewError ? (
              <ErrorNotice
                title="Preview unavailable"
                message={previewError}
                onRetry={() => void load()}
              />
            ) : preview && preview.lines.length > 0 ? (
              <pre class="output-preview">{preview.lines.join('\n')}</pre>
            ) : (
              <p class="empty-copy">No recent output was returned.</p>
            )}
          </section>
        </>
      ) : null}
    </div>
  );
}
