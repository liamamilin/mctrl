import type {
  Host,
  HostStatus,
  LaunchRequest,
  LaunchResult,
  ManagedWork,
  PairedDevice,
  Preview,
  Project,
  Runner,
  Session,
} from './types';

export const API_BASE = '/api/v1';
const runtimeMeta =
  typeof document === 'undefined'
    ? undefined
    : document.querySelector<HTMLMetaElement>('meta[name="mctrl-runtime"]');
const runtimeProfile = runtimeMeta?.content.trim().toLowerCase() || 'v1';
export const RUNTIME_PROFILE = runtimeProfile;
export const runtimeStorageKey = (key: string): string =>
  runtimeProfile === 'v1' ? key : `${key}:${runtimeProfile}`;
export const UNAUTHORIZED_EVENT =
  runtimeProfile === 'v1' ? 'mctrl:unauthorized' : `mctrl:${runtimeProfile}:unauthorized`;
const CSRF_COOKIE = runtimeStorageKey('mctrl_csrf');
const INSTALLATION_STORAGE_KEY = runtimeStorageKey('mctrl_device_installation_id');
let csrfToken = '';
let volatileInstallationID = '';

function readCookie(name: string): string {
  if (typeof document === 'undefined') return '';
  const prefix = `${encodeURIComponent(name)}=`;
  for (const part of document.cookie.split(';')) {
    const value = part.trim();
    if (value.startsWith(prefix)) return decodeURIComponent(value.slice(prefix.length));
  }
  return '';
}

function validInstallationID(value: string): boolean {
  return /^[A-Za-z0-9_-]{20,128}$/.test(value);
}

function newInstallationID(): string {
  const bytes = new Uint8Array(16);
  if (!globalThis.crypto?.getRandomValues) {
    throw new Error('Secure browser identity is unavailable.');
  }
  globalThis.crypto.getRandomValues(bytes);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary)
    .replaceAll('+', '-')
    .replaceAll('/', '_')
    .replace(/=+$/u, '');
}

function deviceInstallationID(): string {
  if (volatileInstallationID) return volatileInstallationID;
  try {
    const stored = window.localStorage.getItem(INSTALLATION_STORAGE_KEY) ?? '';
    if (validInstallationID(stored)) {
      volatileInstallationID = stored;
      return stored;
    }
  } catch {
    // Fall back to a page-lifetime identity when storage is unavailable.
  }
  const created = newInstallationID();
  try {
    window.localStorage.setItem(INSTALLATION_STORAGE_KEY, created);
  } catch {
    // The server still receives a valid identity for this page lifetime.
  }
  volatileInstallationID = created;
  return created;
}

type JsonRecord = Record<string, unknown>;

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: unknown;

  constructor(
    status: number,
    code: string,
    message: string,
    details?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function asString(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback;
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 ? value : undefined;
}

function asNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function asBoolean(value: unknown): boolean {
  return value === true;
}

function parsePayload(text: string): unknown {
  if (!text) return undefined;
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return undefined;
  }
}

function errorFromResponse(status: number, payload: unknown): ApiError {
  const root = isRecord(payload) ? payload : {};
  const nested = isRecord(root.error) ? root.error : root;
  const code =
    asString(nested.code, status === 401 ? 'UNAUTHORIZED' : 'REQUEST_FAILED') ||
    'REQUEST_FAILED';
  const message =
    asString(nested.message) || `Request failed with status ${status}.`;
  return new ApiError(status, code, message, nested.details);
}

async function request<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (
    init.method &&
    init.method !== 'GET' &&
    init.method !== 'HEAD' &&
    !headers.has('X-CSRF-Token')
  ) {
    const token = readCookie(CSRF_COOKIE) || csrfToken;
    if (token) headers.set('X-CSRF-Token', token);
  }

  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers,
      credentials: 'include',
    });
  } catch {
    throw new ApiError(
      0,
      'NETWORK_ERROR',
      'The Mac is currently unreachable.',
    );
  }

  const payload = parsePayload(await response.text());
  if (!response.ok) {
    const error = errorFromResponse(response.status, payload);
    if (response.status === 401) {
      window.dispatchEvent(
        new CustomEvent(UNAUTHORIZED_EVENT, {
          detail: { code: error.code, message: error.message },
        }),
      );
    }
    throw error;
  }

  return payload as T;
}

function listPayload(
  payload: unknown,
  keys: string[],
): unknown[] | undefined {
  if (Array.isArray(payload)) return payload;
  if (!isRecord(payload)) return undefined;

  for (const key of keys) {
    if (Array.isArray(payload[key])) return payload[key] as unknown[];
  }
  if (Array.isArray(payload.items)) return payload.items as unknown[];
  if (Array.isArray(payload.data)) return payload.data as unknown[];

  if (isRecord(payload.data)) {
    return listPayload(payload.data, keys);
  }
  return undefined;
}

function unwrapRecord(payload: unknown, key: string): unknown {
  if (!isRecord(payload)) return payload;
  const nested = payload[key];
  return nested === undefined ? payload : nested;
}

function normalizeHost(payload: unknown): Host {
  const value = unwrapRecord(payload, 'host');
  const record = isRecord(value) ? value : {};
  const rawStatus = asString(record.status, 'unknown');
  const status: HostStatus =
    rawStatus === 'online' ||
    rawStatus === 'reconnecting' ||
    rawStatus === 'offline'
      ? rawStatus
      : 'unknown';

  return {
    name: asString(record.name, 'mctrl Mac'),
    hostname: asString(record.hostname),
    status,
    remote_ready: asBoolean(record.remote_ready),
    transport: asOptionalString(record.transport),
    public_url: asOptionalString(record.public_url),
    version: asOptionalString(record.version),
  };
}

function normalizeProject(payload: unknown): Project {
  const record = isRecord(payload) ? payload : {};
  return {
    id: asString(record.id),
    name: asString(record.name, asString(record.id, 'Project')),
    path: asString(record.path),
    default_runner: asOptionalString(record.default_runner),
  };
}

function normalizeRunner(payload: unknown): Runner {
  const record = isRecord(payload) ? payload : {};
  return {
    id: asString(record.id),
    name: asString(record.name, asString(record.id, 'Runner')),
    available: asBoolean(record.available),
  };
}

function normalizeWork(payload: unknown): ManagedWork | undefined {
  if (!isRecord(payload)) return undefined;
  return {
    id: asString(payload.id),
    request_id: asOptionalString(payload.request_id),
    project_id: asOptionalString(payload.project_id),
    runner_id: asOptionalString(payload.runner_id),
    session_id: asOptionalString(payload.session_id),
    session_name: asOptionalString(payload.session_name),
    state: asString(payload.state, 'UNKNOWN'),
    created_at: asOptionalString(payload.created_at),
    started_at: asOptionalString(payload.started_at),
    finished_at: asOptionalString(payload.finished_at),
    runner_pid: asNumber(payload.runner_pid),
    child_pid: asNumber(payload.child_pid),
    exit_code: asNumber(payload.exit_code),
    exit_signal: asNumber(payload.exit_signal),
    keep_awake:
      typeof payload.keep_awake === 'boolean' ? payload.keep_awake : undefined,
    launch_stage: asOptionalString(payload.launch_stage),
    termination_reason: asOptionalString(payload.termination_reason),
    recovery_status: asOptionalString(payload.recovery_status),
    prompt_delivery: asOptionalString(payload.prompt_delivery),
    error_code: asOptionalString(payload.error_code),
    error_message: asOptionalString(payload.error_message),
  };
}

function normalizeSession(payload: unknown): Session {
  const record = isRecord(payload) ? payload : {};
  const activePane = isRecord(record.active_pane) ? record.active_pane : {};
  const managedWork =
    normalizeWork(record.managed_work) ?? normalizeWork(record.work);

  return {
    id: asString(record.id),
    name: asString(record.name, asString(record.id, 'Session')),
    windows: asNumber(record.windows) ?? 0,
    panes: asNumber(record.panes) ?? 0,
    attached: asBoolean(record.attached),
    active_pane: {
      command: asOptionalString(activePane.command),
      cwd: asOptionalString(activePane.cwd),
      width: asNumber(activePane.width),
      height: asNumber(activePane.height),
    },
    active_command:
      asOptionalString(record.active_command) ??
      asOptionalString(activePane.command),
    cwd:
      asOptionalString(record.cwd) ?? asOptionalString(activePane.cwd),
    managed_work: managedWork,
  };
}

function normalizeDevice(payload: unknown): PairedDevice {
  const record = isRecord(payload) ? payload : {};
  return {
    id: asString(record.id),
    name: asString(
      record.display_name,
      asString(record.name, 'Unnamed device'),
    ),
    display_name: asOptionalString(record.display_name),
    created_at: asOptionalString(record.created_at),
    last_seen: asOptionalString(record.last_seen),
    revoked_at: asOptionalString(record.revoked_at),
  };
}

function normalizePreview(payload: unknown): Preview {
  const record = isRecord(payload) ? payload : {};
  const nested = isRecord(record.preview) ? record.preview : record;
  const rawLines = Array.isArray(payload)
    ? payload
    : Array.isArray(nested.lines)
      ? nested.lines
      : [];
  return {
    lines: rawLines.filter((line): line is string => typeof line === 'string'),
    captured_at: asOptionalString(nested.captured_at ?? record.captured_at),
  };
}

function normalizeLaunchResult(payload: unknown): LaunchResult {
  const record = isRecord(payload) ? payload : {};
  const rawWork = record.work ?? record;
  const work = normalizeWork(rawWork);
  const rawSession = record.session;
  const session = rawSession ? normalizeSession(rawSession) : undefined;

  return {
    work,
    session,
    work_id: asOptionalString(record.work_id) ?? work?.id,
    session_id:
      asOptionalString(record.session_id) ??
      session?.id ??
      work?.session_id,
    request_id: asOptionalString(record.request_id) ?? work?.request_id,
  };
}

export async function getMe(): Promise<{ device: { id: string; name: string }; csrf_token: string }> {
  const payload = await request<unknown>('/me');
  const record = isRecord(payload) ? payload : {};
  const token = asString(record.csrf_token);
  if (token) csrfToken = token;
  const device = isRecord(record.device) ? record.device : {};
  return { device: { id: asString(device.id), name: asString(device.name) }, csrf_token: token };
}

export async function bindDeviceInstallation(): Promise<void> {
  await request<unknown>('/devices/installation', {
    method: 'POST',
    body: JSON.stringify({ installation_id: deviceInstallationID() }),
  });
}

export async function getHost(): Promise<Host> {
  return normalizeHost(await request<unknown>('/host'));
}

export async function getProjects(): Promise<Project[]> {
  const payload = await request<unknown>('/projects');
  return (listPayload(payload, ['projects']) ?? [])
    .map(normalizeProject)
    .filter((project) => project.id.length > 0);
}

export async function createProject(body: {
  name: string;
  path: string;
  default_runner?: string;
}): Promise<Project> {
  const payload = await request<unknown>('/projects', {
    method: 'POST',
    body: JSON.stringify(body),
  });
  return normalizeProject(unwrapRecord(payload, 'project'));
}

export async function deleteProject(projectId: string): Promise<void> {
  await request<unknown>(`/projects/${encodeURIComponent(projectId)}`, {
    method: 'DELETE',
  });
}

export async function getRunners(): Promise<Runner[]> {
  const payload = await request<unknown>('/runners');
  return (listPayload(payload, ['runners']) ?? [])
    .map(normalizeRunner)
    .filter((runner) => runner.id.length > 0);
}

export async function getSessions(): Promise<Session[]> {
  const payload = await request<unknown>('/sessions');
  return (listPayload(payload, ['sessions']) ?? [])
    .map(normalizeSession)
    .filter((session) => session.id.length > 0);
}

export async function getSession(sessionId: string): Promise<Session> {
  const payload = await request<unknown>(
    `/sessions/${encodeURIComponent(sessionId)}`,
  );
  return normalizeSession(unwrapRecord(payload, 'session'));
}

export async function getSessionPreview(
  sessionId: string,
  lines: number,
): Promise<Preview> {
  const payload = await request<unknown>(
    `/sessions/${encodeURIComponent(sessionId)}/preview?lines=${lines}`,
  );
  return normalizePreview(payload);
}

export async function getWork(): Promise<ManagedWork[]> {
  const payload = await request<unknown>('/work');
  return (listPayload(payload, ['work', 'works']) ?? []).flatMap((item) => {
    const work = normalizeWork(item);
    return work && work.id.length > 0 ? [work] : [];
  });
}

export async function getWorkItem(workId: string): Promise<ManagedWork> {
  const payload = await request<unknown>(`/work/${encodeURIComponent(workId)}`);
  const work = normalizeWork(unwrapRecord(payload, 'work'));
  if (!work) throw new ApiError(500, 'INVALID_RESPONSE', 'Work was not returned.');
  return work;
}

export async function launchWork(body: LaunchRequest): Promise<LaunchResult> {
  const payload = await request<unknown>('/work', {
    method: 'POST',
    body: JSON.stringify(body),
  });
  return normalizeLaunchResult(payload);
}

export async function sendSessionPrompt(
  sessionId: string,
  body: { request_id: string; text: string },
): Promise<{ request_id: string; delivery: string }> {
  const payload = await request<unknown>(`/sessions/${encodeURIComponent(sessionId)}/prompt`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
  const record = isRecord(payload) ? payload : {};
  const result = {
    request_id: asString(record.request_id, body.request_id),
    delivery: asString(record.delivery, 'UNKNOWN'),
  };
  if (result.delivery !== 'CONFIRMED') {
    throw new ApiError(
      502,
      'PROMPT_DELIVERY_FAILED',
      'Prompt delivery was not confirmed. Inspect the terminal output before trying again.',
      result,
    );
  }
  return result;
}

export async function getDevices(): Promise<PairedDevice[]> {
  const payload = await request<unknown>('/devices');
  return (listPayload(payload, ['devices']) ?? [])
    .map(normalizeDevice)
    .filter((device) => device.id.length > 0);
}

export async function createDeviceLink(): Promise<{
  token: string;
  expires_at?: string;
}> {
  const payload = await request<unknown>('/devices/links', {
    method: 'POST',
  });
  const record = isRecord(payload) ? payload : {};
  const token = asString(record.token);
  if (!token) {
    throw new ApiError(502, 'INVALID_RESPONSE', 'Browser link token was not returned.');
  }
  return { token, expires_at: asOptionalString(record.expires_at) };
}

export async function linkDeviceBrowser(token: string): Promise<void> {
  const payload = await request<unknown>('/devices/link', {
    method: 'POST',
    body: JSON.stringify({
      token,
      installation_id: deviceInstallationID(),
    }),
  });
  if (isRecord(payload)) {
    const next = asString(payload.csrf_token);
    if (next) csrfToken = next;
  }
}

export async function getDeviceHistory(): Promise<PairedDevice[]> {
  const payload = await request<unknown>('/devices/history');
  return (listPayload(payload, ['devices', 'history']) ?? [])
    .map(normalizeDevice)
    .filter((device) => device.id.length > 0);
}

export async function revokeDevice(deviceId: string): Promise<void> {
  await request<unknown>(`/devices/${encodeURIComponent(deviceId)}`, {
    method: 'DELETE',
  });
}

export async function getPairingQR(token: string): Promise<{
  data_url: string;
  pair_url: string;
  expires_at?: string;
}> {
  const payload = await request<unknown>('/pair/qr', {
    method: 'POST',
    body: JSON.stringify({ token }),
  });
  const record = isRecord(payload) ? payload : {};
  const dataURL = asString(record.data_url);
  if (!dataURL.startsWith('data:image/png;base64,')) {
    throw new ApiError(502, 'INVALID_RESPONSE', 'Pairing QR code was not returned.');
  }
  return {
    data_url: dataURL,
    pair_url: asString(record.pair_url),
    expires_at: asOptionalString(record.expires_at),
  };
}

export async function pairDevice(
  token: string,
  deviceName: string,
): Promise<void> {
  const payload = await request<unknown>('/pair', {
    method: 'POST',
    body: JSON.stringify({
      token,
      device_name: deviceName,
      installation_id: deviceInstallationID(),
    }),
  });
  if (isRecord(payload)) {
    const next = asString(payload.csrf_token);
    if (next) csrfToken = next;
  }
}

export function isUncertainOutcome(error: unknown): boolean {
  if (!(error instanceof ApiError)) return true;
  return (
    error.status === 0 ||
    error.status === 408 ||
    error.status === 425 ||
    error.status === 429 ||
    error.status >= 500
  );
}
