import { useEffect, useRef, useState } from 'preact/hooks';
import {
  ApiError,
  getProjects,
  getRunners,
  isUncertainOutcome,
  launchWork,
  runtimeStorageKey,
} from '../api';
import { makeRequestId } from '../format';
import type { LaunchRequest, LaunchResult, Project, Runner } from '../types';
import { navigate, sessionPath } from '../router';
import { EmptyState, ErrorNotice, LoadingBlock } from '../components/ui';

interface Attempt {
  fingerprint: string;
  requestId: string;
}

const ATTEMPT_STORAGE_KEY = runtimeStorageKey('mctrl-launch-attempt');

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

function clearAttempt(): void {
  try {
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
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState('');
  const [uncertain, setUncertain] = useState(false);
  const [result, setResult] = useState<LaunchResult>();
  const attempt = useRef<Attempt | undefined>(readAttempt());

  const load = async () => {
    setLoading(true);
    setLoadError('');
    try {
      const [nextProjects, nextRunners] = await Promise.all([
        getProjects(),
        getRunners(),
      ]);
      setProjects(nextProjects);
      setRunners(nextRunners);
      const requested = requestedProjectId();
      const selectedProjectId = nextProjects.some(
        (project) => project.id === projectId,
      )
        ? projectId
        : nextProjects.some((project) => project.id === requested)
          ? requested
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
      setLoadError(
        caught instanceof Error
          ? caught.message
          : 'Could not load Projects and Runners.',
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const selectedProject = projects.find((project) => project.id === projectId);
  const selectedRunner = runners.find((runner) => runner.id === runnerId);
  const canSubmit =
    Boolean(projectId && runnerId && prompt.trim()) &&
    Boolean(selectedRunner?.available) &&
    !submitting;

  const submit = async (event?: Event) => {
    event?.preventDefault();
    if (!canSubmit) return;

    const body: LaunchRequest = {
      request_id: '',
      project_id: projectId,
      runner_id: runnerId,
      prompt: prompt.trim(),
    };
    const fingerprint = await stableFingerprint(
      JSON.stringify({
        project_id: body.project_id,
        runner_id: body.runner_id,
        prompt: body.prompt,
      }),
    );
    const nextAttempt =
      attempt.current?.fingerprint === fingerprint
        ? attempt.current
        : { fingerprint, requestId: makeRequestId() };
    attempt.current = nextAttempt;
    writeAttempt(nextAttempt);
    body.request_id = nextAttempt.requestId;

    setSubmitting(true);
    setSubmitError('');
    setUncertain(false);

    try {
      const nextResult = await launchWork(body);
      setResult(nextResult);
      attempt.current = undefined;
      clearAttempt();
      if (nextResult.session_id) {
        navigate(sessionPath(nextResult.session_id));
      }
    } catch (caught) {
      setSubmitError(
        caught instanceof ApiError
          ? caught.message
          : 'The launch result could not be confirmed.',
      );
      const nextUncertain = isUncertainOutcome(caught);
      setUncertain(nextUncertain);
      if (!nextUncertain) {
        attempt.current = undefined;
        clearAttempt();
      }
    } finally {
      setSubmitting(false);
    }
  };

  if (result) {
    return (
      <div class="page-stack narrow-page">
        <section class="success-card card">
          <div class="success-mark" aria-hidden="true">
            ✓
          </div>
          <div class="section-kicker">Launch accepted</div>
          <h1>The Mac accepted this Work</h1>
          <p>
            The browser can disconnect without stopping Work that has started.
            tmux keeps the Session durable.
          </p>
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
                href={`#${sessionPath(result.session_id)}`}
              >
                Open session
              </a>
            )}
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
        <p>Choose a registered target. The Mac owns the session and process.</p>
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
            <a class="button button-primary" href="#/settings">
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
                <h2>Project</h2>
                <span>Required</span>
              </div>
              <label class="field">
                <span class="sr-only">Project</span>
                <select
                  value={projectId}
                  onChange={(event) => {
                    setProjectId(event.currentTarget.value);
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
                  <span class="selected-fact-label">Registered path</span>
                  <code>{selectedProject.path || 'Path unavailable'}</code>
                </div>
              )}
            </div>
          </section>

          <section class="form-section card">
            <div class="form-section-number">2</div>
            <div class="form-section-content">
              <div class="form-section-title">
                <h2>Runner</h2>
                <span>Available on Mac</span>
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
                        {runner.available ? 'Ready to launch' : 'Not installed'}
                      </small>
                    </span>
                    <span class="availability-dot" aria-hidden="true" />
                  </label>
                ))}
              </div>
            </div>
          </section>

          <section class="form-section card">
            <div class="form-section-number">3</div>
            <div class="form-section-content">
              <div class="form-section-title">
                <h2>Prompt</h2>
                <span>Multiline</span>
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
                  placeholder="Describe the work to run on the Mac…"
                  disabled={submitting}
                  required
                />
                <span class="field-counter">{prompt.length} characters</span>
              </label>
            </div>
          </section>

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
              <span>Launching on</span>
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
                    ? 'Choose a Project.'
                    : !runnerId
                      ? 'Choose a Runner.'
                      : 'Describe the work in the Prompt to enable launch.'}
              </p>
            )}
          </div>
        </form>
      )}
    </div>
  );
}
