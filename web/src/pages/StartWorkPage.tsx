import { useEffect, useRef, useState } from 'preact/hooks';
import {
  ApiError,
  getProjects,
  getRunners,
  getWorkItem,
  isUncertainOutcome,
  launchWork,
  runtimeStorageKey,
} from '../api';
import { makeRequestId } from '../format';
import type { LaunchRequest, LaunchResult, Project, Runner } from '../types';
import { navigate, sessionPath } from '../router';
import { EmptyState, ErrorNotice, LoadingBlock } from '../components/ui';
import { CopyText } from '../components/CopyText';
import { useForegroundRefresh } from '../use-foreground-refresh';

interface Attempt {
  fingerprint: string;
  requestId: string;
}

const ATTEMPT_STORAGE_KEY = runtimeStorageKey('mctrl-launch-attempt');
const LAST_PROJECT_KEY = runtimeStorageKey('mctrl-last-project');
function lastProjectId(): string {
  try { return window.localStorage.getItem(LAST_PROJECT_KEY) ?? ''; } catch { return ''; }
}

function readAttempt(): Attempt | undefined {
  try {
    const value = window.sessionStorage.getItem(ATTEMPT_STORAGE_KEY);
    if (!value) return undefined;
    const parsed = JSON.parse(value) as Partial<Attempt>;
    if (typeof parsed.fingerprint !== 'string' || typeof parsed.requestId !== 'string') {
      return undefined;
    }
    return { fingerprint: parsed.fingerprint, requestId: parsed.requestId };
  } catch {
    return undefined;
  }
}

function writeAttempt(value: Attempt): void {
  try {
    window.sessionStorage.setItem(ATTEMPT_STORAGE_KEY, JSON.stringify(value));
  } catch {
    // The server-side request ID remains authoritative if storage is blocked.
  }
}

function clearAttempt(requestId?: string): void {
  try {
    if (requestId && readAttempt()?.requestId !== requestId) return;
    window.sessionStorage.removeItem(ATTEMPT_STORAGE_KEY);
  } catch {
    // Ignore storage failures.
  }
}

function requestedProjectId(): string {
  const query = window.location.hash.split('?')[1] ?? '';
  return new URLSearchParams(query).get('project') ?? '';
}

async function stableFingerprint(value: string): Promise<string> {
  if (globalThis.crypto?.subtle) {
    const digest = await globalThis.crypto.subtle.digest(
      'SHA-256',
      new TextEncoder().encode(value),
    );
    return Array.from(new Uint8Array(digest), (byte) =>
      byte.toString(16).padStart(2, '0'),
    ).join('');
  }
  // Non-cryptographic fallback for older browsers; it avoids persisting the
  // prompt itself while still keeping the common retry path idempotent.
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(16);
}

export function StartWorkPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [runners, setRunners] = useState<Runner[]>([]);
  const [projectId, setProjectId] = useState('');
  const [runnerId, setRunnerId] = useState('');
  const [prompt, setPrompt] = useState('');
  const [launchMode, setLaunchMode] = useState<'interactive' | 'task'>('interactive');
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState('');
  const [uncertain, setUncertain] = useState(false);
  const [result, setResult] = useState<LaunchResult>();
  const [following, setFollowing] = useState(false);
  const [followError, setFollowError] = useState('');
  const [followAttempt, setFollowAttempt] = useState(0);
  const attempt = useRef<Attempt | undefined>(readAttempt());
  const launching = useRef(false);
  const openInteractive = useRef(true);
  const mounted = useRef(true);

  const load = async () => {
    setLoading(true);
    setLoadError('');
    try {
      const [nextProjects, nextRunners] = await Promise.all([
        getProjects(),
        getRunners(),
      ]);
      if (!mounted.current) return;
      setProjects(nextProjects);
      setRunners(nextRunners);
      const requested = requestedProjectId();
      const selectedProjectId = nextProjects.some((project) => project.id === requested)
        ? requested
        : nextProjects.some(
        (project) => project.id === projectId,
      )
        ? projectId
        : nextProjects.some((project) => project.id === lastProjectId())
          ? lastProjectId()
          : (nextProjects[0]?.id ?? '');
      setProjectId(selectedProjectId);
      setRunnerId((current) => {
        const selectedProject = nextProjects.find(
          (project) => project.id === selectedProjectId,
        );
        const preferred = selectedProject?.default_runner;
        if (
          nextRunners.some(
            (runner) => runner.id === current && runner.available,
          )
        ) {
          return current;
        }
        return (
          nextRunners.find(
            (runner) => runner.id === preferred && runner.available,
          )?.id ??
          nextRunners.find((runner) => runner.available)?.id ??
          ''
        );
      });
    } catch (caught) {
      if (!mounted.current) return;
      setLoadError(
        caught instanceof Error
          ? caught.message
          : 'Could not load Projects and Runners.',
      );
    } finally {
      if (mounted.current) setLoading(false);
    }
  };

  useEffect(() => {
    mounted.current = true;
    void load();
    return () => { mounted.current = false; };
  }, []);

  // An accepted response can precede Session evidence. Follow only this Work
  // through read-only observations; never repeat POST to make a Session appear.
  useEffect(() => {
    const workId = result?.work_id;
    if (!workId || result.session_id || result.work?.archived_at || result.work?.recovery_status || ['EXITED', 'LAUNCH_FAILED'].includes(result.work?.state ?? '')) return;
    let cancelled = false;
    let timer: number | undefined;
    const started = Date.now();
    setFollowing(true);
    setFollowError('');
    const observe = async () => {
      let message = '';
      try {
        const item = await getWorkItem(workId);
        if (cancelled) return;
        if (item.id !== workId) throw new Error('The returned Work identity did not match.');
        setResult(current => current?.work_id === workId ? { ...current, work: item, session_id: item.session_id } : current);
        if (item.session_id && !item.archived_at && !item.recovery_status && item.state !== 'LAUNCH_FAILED') {
          navigate(sessionPath(item.session_id, openInteractive.current && item.state !== 'EXITED'));
          return;
        }
        if (item.archived_at || item.recovery_status || ['EXITED', 'LAUNCH_FAILED'].includes(item.state)) {
          setFollowing(false);
          return;
        }
      } catch (caught) {
        if (cancelled) return;
        message = caught instanceof Error ? caught.message : 'The Work status could not be read.';
      }
      if (Date.now() - started >= 30_000) {
        setFollowing(false);
        setFollowError(message || 'The Session is still being prepared. Check this Work again or return to Sessions.');
        return;
      }
      timer = window.setTimeout(() => void observe(), message ? 2000 : 500);
    };
    void observe();
    return () => { cancelled = true; window.clearTimeout(timer); };
  }, [result?.work_id, result?.session_id, result?.work?.state, followAttempt]);
  useForegroundRefresh(() => {
    if (result?.work_id && !result.session_id) setFollowAttempt(value => value + 1);
  });

  const selectedProject = projects.find((project) => project.id === projectId);
  const selectedRunner = runners.find((runner) => runner.id === runnerId);
  const agentRunner = runnerId === 'codex' || runnerId === 'opencode';
  const taskMode = agentRunner && launchMode === 'task';
  // The prompt is optional, and only the page thought otherwise. The API accepts
  // an empty prompt, and every runner guards its own prompt argument, so a plain
  // shell Work — or an agent Work the user intends to type into — does not have to
  // be given an opening line it does not want.
  const canSubmit =
    Boolean(projectId && runnerId) &&
    Boolean(selectedRunner?.available) &&
    (!taskMode || Boolean(prompt.trim())) &&
    !submitting;

  const submit = async (event?: Event) => {
    event?.preventDefault();
    if (!canSubmit || launching.current) return;
    launching.current = true;
    openInteractive.current = !taskMode;
    setSubmitting(true);
    setSubmitError('');
    setUncertain(false);

    const body: LaunchRequest = {
      request_id: '',
      project_id: projectId,
      runner_id: runnerId,
      prompt: agentRunner && !taskMode ? '' : prompt.trim(),
    };
    let requestStarted = false;
    try {
      const fingerprint = await stableFingerprint(
        JSON.stringify({
          project_id: body.project_id,
          runner_id: body.runner_id,
          prompt: body.prompt,
        }),
      );
      if (!mounted.current) return;
      const nextAttempt =
        attempt.current?.fingerprint === fingerprint
          ? attempt.current
          : { fingerprint, requestId: makeRequestId() };
      attempt.current = nextAttempt;
      writeAttempt(nextAttempt);
      body.request_id = nextAttempt.requestId;

      requestStarted = true;
      const nextResult = await launchWork(body);
      attempt.current = undefined;
      clearAttempt(body.request_id);
      try { window.localStorage.setItem(LAST_PROJECT_KEY, body.project_id); } catch { /* Optional convenience. */ }
      if (!mounted.current) return;
      setResult(nextResult);
      if (nextResult.session_id && !nextResult.work?.archived_at && !nextResult.work?.recovery_status && nextResult.work?.state !== 'LAUNCH_FAILED') {
        navigate(sessionPath(nextResult.session_id, openInteractive.current && nextResult.work?.state !== 'EXITED'));
      }
    } catch (caught) {
      const nextUncertain = requestStarted && isUncertainOutcome(caught);
      if (!nextUncertain) {
        attempt.current = undefined;
        clearAttempt(body.request_id);
      }
      if (!mounted.current) return;
      setSubmitError(
        caught instanceof ApiError
          ? caught.message
          : 'The launch result could not be confirmed.',
      );
      setUncertain(nextUncertain);
    } finally {
      launching.current = false;
      if (mounted.current) setSubmitting(false);
    }
  };

  if (result) {
    const failed = result.work?.state === 'LAUNCH_FAILED';
    const ended = result.work?.state === 'EXITED';
    const needsReview = Boolean(result.work?.archived_at || result.work?.recovery_status);
    return (
      <div class="page-stack narrow-page">
        <section class="success-card card">
          <div class={`success-mark${failed ? ' launch-failed-mark' : ''}`} aria-hidden="true">{failed ? '!' : following ? '…' : '✓'}</div>
          <div class="section-kicker">{failed ? 'Launch failed' : needsReview ? 'Work needs review' : 'Launch accepted'}</div>
          <h1>{failed ? 'This Work could not start' : needsReview ? 'Check this Work before continuing' : 'The Mac accepted this Work'}</h1>
          <p>
            The browser can disconnect without stopping Work that has started.
            tmux keeps the Session durable.
          </p>
          {following && <p role="status">Waiting for the Session. {openInteractive.current ? 'Your terminal will open automatically.' : 'Its output will open automatically.'}</p>}
          {failed && <p class="form-error" role="alert">{result.work?.error_message || result.work?.error_code || 'The process did not start. Review the launch settings before trying again.'}</p>}
          {ended && !result.session_id && <p>This Work has exited without an available Session. Review its record in Settings.</p>}
          {followError && <p class="form-error" role="alert">{followError}</p>}
          {result.work_id && (
            <div class="result-fact">
              <span>Work ID</span>
              <code>{result.work_id}</code>
            </div>
          )}
          <div class="button-stack">
            {result.session_id && (
              <a
                class="button button-primary"
                href={`#${sessionPath(result.session_id, openInteractive.current && !failed && !needsReview && result.work?.state !== 'EXITED')}`}
              >
                {openInteractive.current && !failed && !needsReview && !ended ? 'Open terminal' : 'View session'}
              </a>
            )}
            {!result.session_id && result.work_id && !failed && !needsReview && !ended && <button class="button button-secondary" type="button" disabled={following} onClick={() => setFollowAttempt(value => value + 1)}>Check Work status</button>}
            {failed && <button class="button button-secondary" type="button" onClick={() => { setResult(undefined); setFollowError(''); }}>Review launch settings</button>}
            {(needsReview || (ended && !result.session_id)) && <a class="button button-secondary" href="#/settings?section=recovery">Review Work records</a>}
            <a class="button button-secondary" href="#/">
              Back to Home
            </a>
          </div>
        </section>
      </div>
    );
  }

  return (
    <div class="page-stack">
      <header class="page-heading">
        <div class="section-kicker">Launch</div>
        <h1>Start Work</h1>
        <p>Choose a directory, then open a shell or coding harness on your Mac.</p>
      </header>

      {loading ? (
        <LoadingBlock label="Loading projects and runners…" />
      ) : loadError ? (
        <ErrorNotice message={loadError} onRetry={() => void load()} />
      ) : projects.length === 0 ? (
        <EmptyState
          title="No registered Projects"
          message="Register a Mac directory in Settings before launching work. Only registered paths can be launched."
          action={
            <a class="button button-primary" href="#/settings?section=projects&return=start">
              Register a Project
            </a>
          }
        />
      ) : (
        <form class="launch-form" onSubmit={submit}>
          <section class="form-section card">
            <div class="form-section-number">1</div>
            <div class="form-section-content">
              <div class="form-section-title">
                <h2>Working directory</h2>
                <a class="text-button" href="#/settings?section=projects&return=start">Add directory</a>
              </div>
              <label class="field">
                <span class="sr-only">Project</span>
                <select
                  value={projectId}
                  onChange={(event) => {
                    const id = event.currentTarget.value;
                    setProjectId(id);
                    const preferred = projects.find((project) => project.id === id)?.default_runner;
                    if (runners.some((runner) => runner.id === preferred && runner.available)) setRunnerId(preferred!);
                    setSubmitError('');
                  }}
                  disabled={submitting}
                >
                  {projects.map((project) => (
                    <option key={project.id} value={project.id}>
                      {project.name}
                    </option>
                  ))}
                </select>
              </label>
              {selectedProject && (
                <div class="selected-fact">
                  <CopyText label="Starting path" value={selectedProject.path} />
                  <p class="directory-hint">Opens in this directory. No cd command needed.</p>
                </div>
              )}
            </div>
          </section>

          <section class="form-section card">
            <div class="form-section-number">2</div>
            <div class="form-section-content">
              <div class="form-section-title">
                <h2>Runner</h2>
                <span>Installed on Mac</span>
              </div>
              <div class="runner-options" role="radiogroup" aria-label="Runner">
                {runners.map((runner) => (
                  <label
                    key={runner.id}
                    class={`runner-option${!runner.available ? ' unavailable' : ''}`}
                  >
                    <input
                      type="radio"
                      name="runner"
                      value={runner.id}
                      checked={runnerId === runner.id}
                      disabled={!runner.available || submitting}
                      onChange={() => {
                        setRunnerId(runner.id);
                        setSubmitError('');
                      }}
                    />
                    <span class="radio-indicator" aria-hidden="true" />
                    <span>
                      <strong>{runner.name}</strong>
                      <small>
                        {runner.available ? runner.id === 'shell' ? 'Shell, then any tool' : 'Interactive coding harness' : 'Not installed'}
                      </small>
                    </span>
                    <span class="availability-dot" aria-hidden="true" />
                  </label>
                ))}
              </div>
              {agentRunner && <div class="launch-mode" aria-label="How to use the harness">
                <h2>How do you want to work?</h2>
                <div class="mode-options">
                  <button class={launchMode === 'interactive' ? 'selected' : ''} type="button" aria-pressed={launchMode === 'interactive'} onClick={() => { setLaunchMode('interactive'); setSubmitError(''); }} disabled={submitting}>Open &amp; interact</button>
                  <button class={launchMode === 'task' ? 'selected' : ''} type="button" aria-pressed={launchMode === 'task'} onClick={() => { setLaunchMode('task'); setSubmitError(''); }} disabled={submitting}>Run a task</button>
                </div>
                <p>{taskMode ? 'Submit one task now, then return later to read its output.' : `Open ${selectedRunner?.name} and write messages from the phone terminal.`}</p>
                <small>Installed on Mac. Model and account access are checked by the harness when used.</small>
              </div>}
            </div>
          </section>

          {(!agentRunner || taskMode) && <section class="form-section card">
            <div class="form-section-number">3</div>
            <div class="form-section-content">
              <div class="form-section-title">
                <h2>{taskMode ? 'Task' : 'First shell command'}</h2>
                <span>{taskMode ? 'Required' : 'Optional'}</span>
              </div>
              <label class="field">
                <span class="sr-only">Prompt</span>
                <textarea
                  value={prompt}
                  onInput={(event) => {
                    setPrompt(event.currentTarget.value);
                    setSubmitError('');
                  }}
                  rows={7}
                  placeholder={taskMode ? 'Describe the task to run…' : 'Leave empty to open a shell. You can cd and start any harness there.'}
                  disabled={submitting}
                />
                <span class="field-counter">{prompt.length} characters</span>
              </label>
            </div>
          </section>}

          {submitError && (
            <ErrorNotice
              title={uncertain ? 'Launch outcome is uncertain' : 'Launch failed'}
              message={submitError}
              onRetry={uncertain ? () => void submit() : undefined}
            />
          )}

          {uncertain && (
            <div class="idempotency-note">
              <strong>No duplicate launch will be created.</strong>
              <p>
                A manual retry reuses request ID{' '}
                <code>{attempt.current?.requestId}</code>. The server can return
                the original Work instead of starting another process.
              </p>
            </div>
          )}

          <div class="sticky-submit">
            <div class="launch-summary">
              <span>{taskMode ? 'Run task in' : `Open ${selectedRunner?.name ?? 'terminal'} in`}</span>
              <strong>{selectedProject?.name ?? '—'}</strong>
            </div>
            <button class="button button-primary" type="submit" disabled={!canSubmit}>
              {submitting ? <span class="button-spinner" /> : null}
              {submitting ? 'Starting…' : 'Start Work'}
            </button>
            {/* A disabled button with no stated reason is the one control on the
                page that cannot explain itself. */}
            {!canSubmit && !submitting && (
              <p class="submit-blocked-reason">
                {selectedRunner && !selectedRunner.available
                  ? `${selectedRunner.name} is not installed on the Mac.`
                  : !projectId
                    ? 'Choose a directory.'
                    : taskMode && !prompt.trim()
                      ? 'Write a task, or choose Open & interact.'
                    : 'Choose a Runner.'}
              </p>
            )}
          </div>
        </form>
      )}
    </div>
  );
}
