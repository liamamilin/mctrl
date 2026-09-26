import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  createDeviceLink,
  createProject,
  deleteProject,
  getDeviceHistory,
  getDevices,
  getHost,
  getProjects,
  getRunners,
  RUNTIME_PROFILE,
  revokeDevice,
  updateTerminalSize,
} from '../api';
import { formatDate } from '../format';
import type { Host, PairedDevice, Project, Runner } from '../types';
import { ErrorNotice, LoadingBlock, StatusDot, TransportNotice } from '../components/ui';

// Mirrors the Mac's accepted range so the form cannot submit a value the daemon
// will reject.
const TERMINAL_SIZE_LIMITS = {
  minCols: 20,
  maxCols: 500,
  minRows: 10,
  maxRows: 200,
} as const;

export function SettingsPage() {
  const [host, setHost] = useState<Host>();
  const [devices, setDevices] = useState<PairedDevice[]>([]);
  const [deviceHistory, setDeviceHistory] = useState<PairedDevice[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [runners, setRunners] = useState<Runner[]>([]);
  const [projectName, setProjectName] = useState('');
  const [projectPath, setProjectPath] = useState('~');
  const [projectRunner, setProjectRunner] = useState('shell');
  const [projectSaving, setProjectSaving] = useState(false);
  const [projectRemoving, setProjectRemoving] = useState('');
  const [projectError, setProjectError] = useState('');
  const [projectNotice, setProjectNotice] = useState('');
  const [terminalCols, setTerminalCols] = useState('240');
  const [terminalRows, setTerminalRows] = useState('60');
  const [terminalSizeSaving, setTerminalSizeSaving] = useState(false);
  const [terminalSizeError, setTerminalSizeError] = useState('');
  const [terminalSizeNotice, setTerminalSizeNotice] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [revoking, setRevoking] = useState('');
  const [creatingLink, setCreatingLink] = useState(false);
  const [browserLink, setBrowserLink] = useState('');
  const [browserLinkExpiry, setBrowserLinkExpiry] = useState('');
  const [browserLinkSeconds, setBrowserLinkSeconds] = useState(0);
  const [linkCopied, setLinkCopied] = useState(false);
  const [linkNotice, setLinkNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const loadId = useRef(0);
  const browserLinkInput = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    const currentLoad = ++loadId.current;
    setLoading(true);
    setError('');
    setActionError('');
    setBrowserLink('');
    setBrowserLinkExpiry('');
    setBrowserLinkSeconds(0);
    setLinkCopied(false);
    setLinkNotice('');
    setProjectError('');
    setProjectNotice('');

    try {
      const [nextHost, nextDevices, nextHistory, nextProjects, nextRunners] =
        await Promise.all([
          getHost(),
          getDevices(),
          getDeviceHistory(),
          getProjects(),
          getRunners(),
        ]);
      if (currentLoad !== loadId.current) return;
      setHost(nextHost);
      if (nextHost.terminal_full_size) {
        setTerminalCols(String(nextHost.terminal_full_size.cols));
        setTerminalRows(String(nextHost.terminal_full_size.rows));
      }
      setDevices(nextDevices);
      setDeviceHistory(nextHistory);
      setProjects(nextProjects);
      setRunners(nextRunners);
      setProjectRunner((current) =>
        nextRunners.some((runner) => runner.id === current && runner.available)
          ? current
          : nextRunners.find((runner) => runner.available)?.id ?? '',
      );
    } catch (caught) {
      if (currentLoad !== loadId.current) return;
      setError(
        caught instanceof Error
          ? caught.message
          : 'Could not load Settings.',
      );
    } finally {
      if (currentLoad === loadId.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    return () => {
      loadId.current += 1;
    };
  }, [load]);

  useEffect(() => {
    if (!browserLinkExpiry) return;
    const update = () => {
      const expiresAt = Date.parse(browserLinkExpiry);
      setBrowserLinkSeconds(
        Number.isFinite(expiresAt)
          ? Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000))
          : 0,
      );
    };
    update();
    const timer = window.setInterval(update, 1000);
    return () => window.clearInterval(timer);
  }, [browserLinkExpiry]);

  const saveTerminalSize = async (event: Event) => {
    event.preventDefault();
    const cols = Number(terminalCols);
    const rows = Number(terminalRows);
    if (
      !Number.isInteger(cols) ||
      !Number.isInteger(rows) ||
      cols < TERMINAL_SIZE_LIMITS.minCols ||
      cols > TERMINAL_SIZE_LIMITS.maxCols ||
      rows < TERMINAL_SIZE_LIMITS.minRows ||
      rows > TERMINAL_SIZE_LIMITS.maxRows
    ) {
      setTerminalSizeError(
        `Use ${TERMINAL_SIZE_LIMITS.minCols}-${TERMINAL_SIZE_LIMITS.maxCols} columns and ${TERMINAL_SIZE_LIMITS.minRows}-${TERMINAL_SIZE_LIMITS.maxRows} rows.`,
      );
      setTerminalSizeNotice('');
      return;
    }
    setTerminalSizeSaving(true);
    setTerminalSizeError('');
    setTerminalSizeNotice('');
    try {
      const saved = await updateTerminalSize({ cols, rows });
      setTerminalCols(String(saved.cols));
      setTerminalRows(String(saved.rows));
      setHost((current) =>
        current ? { ...current, terminal_full_size: saved } : current,
      );
      setTerminalSizeNotice(
        `FULL now asks for ${saved.cols}×${saved.rows}. It applies the next time you switch a Session to FULL.`,
      );
    } catch (caught) {
      setTerminalSizeError(
        caught instanceof Error ? caught.message : 'Could not save the terminal size.',
      );
    } finally {
      setTerminalSizeSaving(false);
    }
  };

  const createBrowserLink = async () => {
    setCreatingLink(true);
    setActionError('');
    setLinkCopied(false);
    setLinkNotice('');
    try {
      const ticket = await createDeviceLink();
      const nextURL = `${window.location.origin}${window.location.pathname}#/link?token=${encodeURIComponent(ticket.token)}`;
      setBrowserLink(nextURL);
      setBrowserLinkExpiry(ticket.expires_at ?? '');
      setBrowserLinkSeconds(120);
      requestAnimationFrame(() => {
        browserLinkInput.current?.focus();
        browserLinkInput.current?.select();
      });
      setLinkNotice('Link created. Copy it, then paste it into Safari on this phone.');
    } catch (caught) {
      setActionError(
        caught instanceof Error ? caught.message : 'Could not create a browser link.',
      );
    } finally {
      setCreatingLink(false);
    }
  };

  const copyBrowserLink = async () => {
    if (!browserLink) return;
    setActionError('');
    setLinkNotice('');
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(browserLink);
      setLinkCopied(true);
      setLinkNotice('Link copied. Open Safari and paste it into the address bar.');
    } catch {
      browserLinkInput.current?.focus();
      browserLinkInput.current?.select();
      setLinkNotice('Automatic copy is unavailable here. The link is selected; use the iOS Copy command.');
    }
  };

  const revoke = async (device: PairedDevice) => {
    const confirmed = window.confirm(
      `Revoke “${device.name}”? Future control requests from that device will be blocked. Existing Work is not stopped.`,
    );
    if (!confirmed) return;

    setRevoking(device.id);
    setActionError('');
    try {
      await revokeDevice(device.id);
      const revoked = {
        ...device,
        revoked_at: new Date().toISOString(),
      };
      setBrowserLink('');
      setBrowserLinkExpiry('');
      setBrowserLinkSeconds(0);
      setLinkNotice('');
      setDevices((current) => current.filter((item) => item.id !== device.id));
      setDeviceHistory((current) => [
        revoked,
        ...current.filter((item) => item.id !== device.id),
      ]);
    } catch (caught) {
      setActionError(
        caught instanceof Error ? caught.message : 'Device revocation failed.',
      );
    } finally {
      setRevoking('');
    }
  };

  const addProject = async (event: Event) => {
    event.preventDefault();
    const name = projectName.trim();
    const path = projectPath.trim();
    if (!name || !path) {
      setProjectError('Enter both a project name and an absolute Mac path.');
      return;
    }

    setProjectSaving(true);
    setProjectError('');
    setProjectNotice('');
    try {
      const created = await createProject({
        name,
        path,
        ...(projectRunner ? { default_runner: projectRunner } : {}),
      });
      setProjects((current) =>
        [...current, created].sort((left, right) =>
          left.name.localeCompare(right.name),
        ),
      );
      setProjectName('');
      setProjectPath('~');
      setProjectNotice(`Registered “${created.name}”. It is now available in Start Work.`);
    } catch (caught) {
      setProjectError(
        caught instanceof Error ? caught.message : 'Could not register the Project.',
      );
    } finally {
      setProjectSaving(false);
    }
  };

  const unregisterProject = async (project: Project) => {
    const confirmed = window.confirm(
      `Unregister “${project.name}”? It will no longer be available for new Work. Existing Sessions and Work will continue.`,
    );
    if (!confirmed) return;

    setProjectRemoving(project.id);
    setProjectError('');
    setProjectNotice('');
    try {
      await deleteProject(project.id);
      setProjects((current) => current.filter((item) => item.id !== project.id));
      setProjectNotice(`Unregistered “${project.name}”. Existing Sessions were not changed.`);
    } catch (caught) {
      setProjectError(
        caught instanceof Error ? caught.message : 'Could not remove the Project.',
      );
    } finally {
      setProjectRemoving('');
    }
  };

  const browserLinkClock = `${Math.floor(browserLinkSeconds / 60)}:${String(browserLinkSeconds % 60).padStart(2, '0')}`;

  return (
    <div class="page-stack">
      <header class="page-heading">
        <div class="section-kicker">This connection</div>
        <h1>Settings</h1>
        <p>Transport facts, Project targets, and paired-device access.</p>
      </header>

      {loading ? (
        <LoadingBlock label="Loading device settings…" />
      ) : error ? (
        <ErrorNotice message={error} onRetry={() => void load()} />
      ) : (
        <>
          <section class="settings-section" aria-labelledby="transport-title">
            <div class="section-heading compact-heading">
              <h2 id="transport-title">Transport</h2>
            </div>
            <TransportNotice transport={host?.transport} />
            <div class="connection-facts card">
              <div>
                <span>Host</span>
                <strong>{host?.name ?? '—'}</strong>
              </div>
              <div>
                <span>Page transport</span>
                <strong>{window.location.protocol.replace(':', '').toUpperCase()}</strong>
              </div>
              <div>
                <span>Runtime profile</span>
                <strong class="mono">
                  {host?.profile ?? RUNTIME_PROFILE}
                </strong>
              </div>
              {host?.public_url && (
                <div>
                  <span>Public URL</span>
                  <strong className="mono">{host.public_url}</strong>
                </div>
              )}
              <div>
                <span>Daemon version</span>
                <strong className="mono">{host?.version ?? '—'}</strong>
              </div>
              <div>
                <span>Host reachability</span>
                <strong class="inline-status">
                  <StatusDot status={host?.status ?? 'unknown'} />
                  {host?.status ?? 'unknown'}
                </strong>
              </div>
            </div>
            <p class="settings-footnote">
              Remote Ready only protects against ordinary idle system sleep
              when the configured policy is active. It does not wake or revive
              an unreachable Mac.
            </p>
          </section>

          <section class="settings-section" aria-labelledby="terminal-size-title">
            <div class="section-heading compact-heading">
              <div>
                <h2 id="terminal-size-title">Full Session size</h2>
                <p>
                  The size FULL asks tmux for, then scales to fit the phone.
                  Programs drop panels below their own width thresholds, so a
                  narrow value shows less than your desktop. A Session whose
                  desktop window is already larger still wins.
                </p>
              </div>
            </div>

            {terminalSizeError && (
              <ErrorNotice
                title="Terminal size was not saved"
                message={terminalSizeError}
              />
            )}
            {terminalSizeNotice && (
              <p class="browser-link-notice" role="status">
                {terminalSizeNotice}
              </p>
            )}

            <form class="card terminal-size-form" onSubmit={saveTerminalSize}>
              <label class="field">
                <span>Columns</span>
                <input
                  type="number"
                  inputMode="numeric"
                  value={terminalCols}
                  onInput={(event) => setTerminalCols(event.currentTarget.value)}
                  min={TERMINAL_SIZE_LIMITS.minCols}
                  max={TERMINAL_SIZE_LIMITS.maxCols}
                  step={1}
                  disabled={terminalSizeSaving}
                  required
                />
              </label>
              <label class="field">
                <span>Rows</span>
                <input
                  type="number"
                  inputMode="numeric"
                  value={terminalRows}
                  onInput={(event) => setTerminalRows(event.currentTarget.value)}
                  min={TERMINAL_SIZE_LIMITS.minRows}
                  max={TERMINAL_SIZE_LIMITS.maxRows}
                  step={1}
                  disabled={terminalSizeSaving}
                  required
                />
              </label>
              <button
                class="button button-primary"
                type="submit"
                disabled={terminalSizeSaving}
              >
                {terminalSizeSaving ? 'Saving…' : 'Save size'}
              </button>
            </form>
            <p class="settings-footnote">
              FULL is requesting {host?.terminal_full_size?.cols ?? '—'}×
              {host?.terminal_full_size?.rows ?? '—'} right now. New Managed Work
              launches at this size too. Allowed range {TERMINAL_SIZE_LIMITS.minCols}
              –{TERMINAL_SIZE_LIMITS.maxCols} columns and{' '}
              {TERMINAL_SIZE_LIMITS.minRows}–{TERMINAL_SIZE_LIMITS.maxRows} rows.
            </p>
          </section>

          <section class="settings-section" aria-labelledby="projects-title">
            <div class="section-heading compact-heading">
              <div>
                <h2 id="projects-title">Projects</h2>
                <p>Only registered Mac paths can be launched from the phone.</p>
              </div>
              <button
                class="icon-button"
                type="button"
                onClick={() => void load()}
                aria-label="Refresh registered Projects"
              >
                ↻
              </button>
            </div>

            {projectError && (
              <ErrorNotice title="Project action failed" message={projectError} />
            )}
            {projectNotice && (
              <p class="browser-link-notice" role="status">
                {projectNotice}
              </p>
            )}

            <form class="project-manager card" onSubmit={addProject}>
              <div class="project-form-grid">
                <label class="field">
                  <span>Project name</span>
                  <input
                    type="text"
                    value={projectName}
                    onInput={(event) => setProjectName(event.currentTarget.value)}
                    placeholder="My Project"
                    maxLength={120}
                    autoCapitalize="sentences"
                    disabled={projectSaving}
                    required
                  />
                </label>
                <label class="field">
                  <span>Mac path</span>
                  <input
                    type="text"
                    value={projectPath}
                    onInput={(event) => setProjectPath(event.currentTarget.value)}
                    placeholder="~ or /Users/you/Projects/my-project"
                    autoCapitalize="off"
                    autoCorrect="off"
                    spellcheck={false}
                    disabled={projectSaving}
                    required
                  />
                </label>
                <label class="field">
                  <span>Default runner</span>
                  <select
                    value={projectRunner}
                    onChange={(event) => setProjectRunner(event.currentTarget.value)}
                    disabled={projectSaving || runners.length === 0}
                  >
                    {runners.length === 0 ? (
                      <option value="">No runner detected</option>
                    ) : (
                      runners.map((runner) => (
                        <option
                          key={runner.id}
                          value={runner.id}
                          disabled={!runner.available}
                        >
                          {runner.name}
                          {runner.available ? '' : ' · unavailable'}
                        </option>
                      ))
                    )}
                  </select>
                </label>
              </div>
              <div class="project-form-actions">
                <small>
                  Use an absolute path or ~. The Mac expands ~ to your home
                  directory; existing Sessions keep their own cwd.
                </small>
                <button
                  class="button button-primary"
                  type="submit"
                  disabled={projectSaving}
                >
                  {projectSaving ? 'Registering…' : 'Register Project'}
                </button>
              </div>
            </form>

            <div class="project-list">
              {projects.length === 0 ? (
                <div class="card empty-copy">
                  No Projects are registered yet. Add the first target above.
                </div>
              ) : (
                projects.map((project) => (
                  <article class="project-card card" key={project.id}>
                    <div class="project-card-content">
                      <div class="project-card-title-row">
                        <h3>{project.name}</h3>
                        {project.default_runner && (
                          <span class="active-badge">{project.default_runner}</span>
                        )}
                      </div>
                      <code>{project.path}</code>
                      {project.path === '/' && (
                        <span class="project-path-warning">
                          This is the Mac root. Consider a narrower directory.
                        </span>
                      )}
                    </div>
                    <button
                      class="button button-danger-quiet button-small"
                      type="button"
                      onClick={() => void unregisterProject(project)}
                      disabled={projectRemoving === project.id}
                    >
                      {projectRemoving === project.id ? 'Unregistering…' : 'Unregister'}
                    </button>
                  </article>
                ))
              )}
            </div>

            <p class="settings-footnote">
              Registering a Project does not start a process. Unregistering it
              only removes the future launch target; existing Sessions and Work
              remain untouched.
            </p>
          </section>

          <section class="settings-section" aria-labelledby="devices-title">
            <div class="section-heading compact-heading">
              <div>
                <h2 id="devices-title">Paired devices</h2>
                <p>Active browser installations are shown once.</p>
              </div>
              <button
                class="icon-button"
                type="button"
                onClick={() => void load()}
                aria-label="Refresh paired devices"
              >
                ↻
              </button>
            </div>

            {actionError && (
              <ErrorNotice title="Device action failed" message={actionError} />
            )}

            <section class="browser-link-panel" aria-labelledby="browser-link-title">
              <div>
                <h3 id="browser-link-title">Link another browser</h3>
                <p>
                  Create a two-minute, one-time link for Safari. It joins the
                  same device record instead of creating another paired device.
                </p>
              </div>
              <button
                class="button button-secondary button-small"
                type="button"
                onClick={() => void createBrowserLink()}
                disabled={creatingLink}
              >
                {creatingLink ? 'Creating…' : browserLink ? 'Create new link' : 'Create link'}
              </button>
              {browserLink && (
                <div class="browser-link-output">
                  <input
                    ref={browserLinkInput}
                    type="text"
                    value={browserLink}
                    readOnly
                    aria-label="One-time Safari browser link"
                    onFocus={(event) => event.currentTarget.select()}
                  />
                  <button
                    class="button button-secondary button-small"
                    type="button"
                    onClick={() => void copyBrowserLink()}
                    disabled={browserLinkSeconds <= 0}
                  >
                    {linkCopied ? 'Copied' : 'Copy link'}
                  </button>
                  <small>
                    Expires in {browserLinkClock} ({formatDate(browserLinkExpiry)}).
                    The link works once and can use this paired device until then.
                  </small>
                </div>
              )}
              {linkNotice && (
                <p class="browser-link-notice" role="status">
                  {linkNotice}
                </p>
              )}
            </section>

            <div class="device-list">
              {devices.length === 0 ? (
                <div class="card empty-copy">No paired devices were returned.</div>
              ) : (
                devices.map((device) => {
                  const revoked = Boolean(device.revoked_at);
                  return (
                    <article class="device-card card" key={device.id}>
                      <div class="device-icon" aria-hidden="true">
                        ▯
                      </div>
                      <div class="device-content">
                        <div class="device-title-row">
                          <h3>{device.name}</h3>
                          <span class={revoked ? 'revoked-badge' : 'active-badge'}>
                            {revoked ? 'Revoked' : 'Active'}
                          </span>
                        </div>
                        <dl class="device-meta">
                          <div>
                            <dt>Created</dt>
                            <dd>{formatDate(device.created_at)}</dd>
                          </div>
                          <div>
                            <dt>Last seen</dt>
                            <dd>{formatDate(device.last_seen)}</dd>
                          </div>
                        </dl>
                        {!revoked && (
                          <button
                            class="button button-danger-quiet button-small"
                            type="button"
                            onClick={() => void revoke(device)}
                            disabled={revoking === device.id}
                          >
                            {revoking === device.id ? 'Revoking…' : 'Revoke device'}
                          </button>
                        )}
                      </div>
                    </article>
                  );
                })
              )}
            </div>

            <details class="device-history card">
              <summary>
                <span>Pairing history</span>
                <strong>{deviceHistory.length}</strong>
              </summary>
              {deviceHistory.length === 0 ? (
                <p class="empty-copy">No revoked browser installations.</p>
              ) : (
                <div class="device-history-list">
                  {deviceHistory.map((device) => (
                    <article class="device-history-row" key={device.id}>
                      <div>
                        <strong>{device.name}</strong>
                        <span>Revoked {formatDate(device.revoked_at)}</span>
                      </div>
                      <code>{device.id}</code>
                    </article>
                  ))}
                </div>
              )}
            </details>

            <p class="settings-footnote">
              A browser stays paired for 30 days. Re-pairing the same browser
              installation reuses this record and rotates its credentials.
              Safari and an installed PWA are separate browser contexts. Use
              the one-time link above to share this device record. Revocation
              does not terminate running Work.
            </p>
          </section>

          <section class="settings-section" aria-labelledby="boundary-title">
            <div class="section-heading compact-heading">
              <h2 id="boundary-title">Connection boundary</h2>
            </div>
            <div class="boundary-list card">
              <div>
                <strong>Origin</strong>
                <code>{window.location.origin}</code>
              </div>
              <div>
                <strong>API base</strong>
                <code>/api/v1</code>
              </div>
              <div>
                <strong>WebSocket attach</strong>
                <code>/api/v1/sessions/:id/terminal</code>
              </div>
            </div>
          </section>
        </>
      )}
    </div>
  );
}
