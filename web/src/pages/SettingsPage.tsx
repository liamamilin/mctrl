import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  createDeviceLink,
  getDeviceHistory,
  getDevices,
  getHost,
  RUNTIME_PROFILE,
  revokeDevice,
} from '../api';
import { formatDate } from '../format';
import type { Host, PairedDevice } from '../types';
import { ErrorNotice, LoadingBlock, StatusDot, TransportNotice } from '../components/ui';

export function SettingsPage() {
  const [host, setHost] = useState<Host>();
  const [devices, setDevices] = useState<PairedDevice[]>([]);
  const [deviceHistory, setDeviceHistory] = useState<PairedDevice[]>([]);
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

    try {
      const [nextHost, nextDevices, nextHistory] = await Promise.all([
        getHost(),
        getDevices(),
        getDeviceHistory(),
      ]);
      if (currentLoad !== loadId.current) return;
      setHost(nextHost);
      setDevices(nextDevices);
      setDeviceHistory(nextHistory);
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

  const browserLinkClock = `${Math.floor(browserLinkSeconds / 60)}:${String(browserLinkSeconds % 60).padStart(2, '0')}`;

  return (
    <div class="page-stack">
      <header class="page-heading">
        <div class="section-kicker">This connection</div>
        <h1>Settings</h1>
        <p>Transport facts and paired-device access.</p>
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
