import { useEffect, useRef, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import {
  API_BASE,
  ApiError,
  getHost,
  getSession,
  isUncertainOutcome,
  runtimeStorageKey,
  sendSessionPrompt,
} from '../api';
import { makeRequestId } from '../format';
import { sessionPath } from '../router';
import type { ActivePane, TerminalSize } from '../types';

const RECONNECT_DELAYS = [500, 1000, 2000, 5000, 10000] as const;

// Mirrors the Mac's canonical full Session size. The /host response is
// authoritative — Settings can change it while the daemon runs — so this only
// keeps FULL usable before that response arrives or when it cannot be read.
const FALLBACK_FULL_SIZE: TerminalSize = { cols: 240, rows: 60 };

const TERMINAL_LIMITS = {
  minCols: 20,
  minRows: 10,
  maxCols: 500,
  maxRows: 200,
} as const;

function clampDimension(
  value: number,
  low: number,
  high: number,
  fallback: number,
): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.min(high, Math.max(low, Math.round(value)));
}

// FULL asks the Mac for its canonical full size, raised to whatever a desktop
// client already made larger. Reading the pane's current size instead would
// make FULL identical to LOCAL whenever the phone is the only attached client,
// because tmux's largest-client policy has already shrunk the window to the
// phone's own viewport. See TMUX_CONTRACT.md.
function overviewTargetSize(
  canonical: TerminalSize,
  pane: ActivePane | undefined,
): TerminalSize {
  return {
    cols: clampDimension(
      Math.max(canonical.cols, pane?.width ?? 0),
      TERMINAL_LIMITS.minCols,
      TERMINAL_LIMITS.maxCols,
      FALLBACK_FULL_SIZE.cols,
    ),
    rows: clampDimension(
      Math.max(canonical.rows, pane?.height ?? 0),
      TERMINAL_LIMITS.minRows,
      TERMINAL_LIMITS.maxRows,
      FALLBACK_FULL_SIZE.rows,
    ),
  };
}

// Terminals want half-width ASCII, and a Chinese IME does not send it.
//
// The digit row of a Chinese keyboard is a candidate selector, so those keys
// never reach the page at all — that is the OS's design and nothing here can
// recover it. Punctuation is different: it is committed text, it does arrive,
// and it arrives full-width (：，／). A program that binds ":" or "/" then sees
// nothing it recognises, which is the half of the original report that this
// conversion actually fixes.
//
// Only these two blocks are touched, so real CJK input is left exactly as typed.
// Set mctrl-terminal-half-width to "off" in localStorage to send the raw bytes.
function normalizeHalfWidth(data: string): string {
  let result = '';
  for (const character of data) {
    const code = character.codePointAt(0) ?? 0;
    if (code >= 0xff01 && code <= 0xff5e) {
      // U+FF01..U+FF5E map onto ASCII 0x21..0x7E.
      result += String.fromCharCode(code - 0xfee0);
      continue;
    }
    if (code === 0x3000) {
      result += ' ';
      continue;
    }
    result += character;
  }
  return result;
}

// The escape hatch for a program that genuinely wants full-width input. There is
// no control for it: the conversion is not a preference the phone user is
// expected to reason about, it is a correction for a keyboard the OS controls.
function halfWidthEnabled(): boolean {
  try {
    const stored = window.localStorage.getItem(
      runtimeStorageKey('mctrl-terminal-half-width'),
    );
    return stored !== 'off';
  } catch {
    return true;
  }
}

// TEMPORARY INPUT DIAGNOSTIC — remove once the iOS event stream is known.
//
// Two attempts to fix the Chinese keyboard's punctuation by inference both
// failed on the real phone, so this records the events iOS actually delivers to
// xterm's helper textarea. The answer is only in there: if no event carries the
// character, the page is never told about it and no interception can help.
const INPUT_TRACE_EVENTS = [
  'keydown',
  'beforeinput',
  'input',
  'compositionstart',
  'compositionupdate',
  'compositionend',
  'keyup',
] as const;

const INPUT_TRACE_LIMIT = 40;

function describeInputEvent(event: Event): string {
  const anyEvent = event as KeyboardEvent & InputEvent & CompositionEvent;
  if (event.type === 'keydown' || event.type === 'keyup') {
    return `key=${JSON.stringify(anyEvent.key)} code=${JSON.stringify(
      anyEvent.code,
    )} keyCode=${anyEvent.keyCode}`;
  }
  if (event.type === 'beforeinput' || event.type === 'input') {
    return `inputType=${JSON.stringify(anyEvent.inputType)} data=${JSON.stringify(
      anyEvent.data,
    )} composed=${anyEvent.composed} isComposing=${anyEvent.isComposing} cancelable=${anyEvent.cancelable} prevented=${anyEvent.defaultPrevented} value=${JSON.stringify(
      (event.target as HTMLTextAreaElement | null)?.value ?? '',
    )}`;
  }
  return `composition data=${JSON.stringify(anyEvent.data)}`;
}

type ConnectionState =
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'offline'
  | 'stopped';

type PromptNotice = {
  kind: 'error' | 'uncertain' | 'success';
  message: string;
};

function terminalUrl(sessionId: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}${API_BASE}/sessions/${encodeURIComponent(
    sessionId,
  )}/terminal`;
}

function withControlModifier(data: string, armed: boolean): string {
  if (!armed) return data;
  return Array.from(data)
    .map((character) => {
      const code = character.charCodeAt(0);
      if (code >= 97 && code <= 122) {
        return String.fromCharCode(code - 96);
      }
      if (code >= 65 && code <= 90) {
        return String.fromCharCode(code - 64);
      }
      return character;
    })
    .join('');
}

function readPendingPrompt(key: string): string | undefined {
  try {
    return window.sessionStorage.getItem(key) ?? undefined;
  } catch {
    return undefined;
  }
}

function writePendingPrompt(key: string, value: string): void {
  try {
    window.sessionStorage.setItem(key, value);
  } catch {
    // Session storage is a best-effort guard; the server-side request ID is
    // still the authoritative idempotency key.
  }
}

function clearPendingPrompt(key: string): void {
  try {
    window.sessionStorage.removeItem(key);
  } catch {
    // Ignore storage failures.
  }
}

export function TerminalPage({ sessionId }: { sessionId: string }) {
  const terminalHost = useRef<HTMLDivElement>(null);
  const sendRaw = useRef<(data: string) => boolean>(() => false);
  const focusTerminal = useRef<() => void>(() => undefined);
  const toggleOverview = useRef<() => void>(() => undefined);
  // The terminal instance itself stays inside the effect; only the one
  // operation the page needs from outside is published here.
  const scrollToTail = useRef<() => void>(() => undefined);
  const controlArmed = useRef(false);
  const promptStorageKey = runtimeStorageKey(`mctrl-prompt:${sessionId}`);
  const initialPromptRequest = readPendingPrompt(promptStorageKey);
  const promptRequest = useRef<string | undefined>(initialPromptRequest);
  const [connection, setConnection] =
    useState<ConnectionState>('connecting');
  const [retryIn, setRetryIn] = useState(0);
  const [protocolMessage, setProtocolMessage] = useState('');
  const [inputBlocked, setInputBlocked] = useState(false);
  const [inputWarning, setInputWarning] = useState('');
  const [transportNotice, setTransportNotice] = useState('');
  const [controlArmedState, setControlArmedState] = useState(false);
  const [manualReconnect, setManualReconnect] = useState(0);
  const [overview, setOverview] = useState(false);
  const [overviewBusy, setOverviewBusy] = useState(false);
  const [overviewError, setOverviewError] = useState('');
  const [overviewSize, setOverviewSize] = useState<TerminalSize | undefined>(
    undefined,
  );
  const [sessionLabel, setSessionLabel] = useState('');
  const [scrolledBack, setScrolledBack] = useState(0);
  const [keyboardInset, setKeyboardInset] = useState(0);
  // TEMPORARY INPUT DIAGNOSTIC — remove with the recorder and the keybar button.
  const [inputTrace, setInputTrace] = useState<string[]>([]);
  const [traceOpen, setTraceOpen] = useState(false);
  // Read once per mount: it is an escape hatch, not a preference the user is
  // expected to flip while typing.
  const [halfWidth] = useState(halfWidthEnabled);
  const [prompt, setPrompt] = useState('');
  const [sendingPrompt, setSendingPrompt] = useState(false);
  const [promptNotice, setPromptNotice] = useState<PromptNotice | undefined>(() =>
    initialPromptRequest
      ? {
          kind: 'uncertain',
          message:
            'A previous Prompt request has an unconfirmed delivery outcome. Nothing will be resent automatically; inspect the terminal output before continuing.',
        }
      : undefined,
  );

  useEffect(() => {
    const pending = readPendingPrompt(promptStorageKey);
    promptRequest.current = pending;
    setPrompt('');
    setOverview(false);
    setOverviewBusy(false);
    setOverviewError('');
    setOverviewSize(undefined);
    setPromptNotice(
      pending
        ? {
            kind: 'uncertain',
            message:
              'A previous Prompt request has an unconfirmed delivery outcome. Nothing will be resent automatically; inspect the terminal output before continuing.',
          }
        : undefined,
    );
  }, [sessionId]);

  // The route carries tmux's stable id ($0, $1, …). Show the human name so the
  // header is never a bare identifier.
  useEffect(() => {
    let cancelled = false;
    setSessionLabel('');
    void getSession(sessionId)
      .then((session) => {
        if (!cancelled) setSessionLabel(session.name || sessionId);
      })
      .catch(() => {
        if (!cancelled) setSessionLabel('');
      });
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  useEffect(() => {
    const host = terminalHost.current;
    if (!host) return;

    let disposed = false;
    let socket: WebSocket | undefined;
    let reconnectTimer: number | undefined;
    let connectTimer: number | undefined;
    let resizeFrame: number | undefined;
    let failedAttempts = 0;
    let stopped = false;
    let inputIsBlocked = false;
    let hadConnection = false;
    let openedAt = 0;
    let overviewMode = false;
    let overviewFitScale = 1;
    let overviewZoom = 1;

    const terminal = new Terminal({
      allowTransparency: false,
      convertEol: false,
      cursorBlink: true,
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      fontSize: 13,
      lineHeight: 1.2,
      scrollback: 5000,
      theme: {
        background: '#070a09',
        foreground: '#dce8e1',
        cursor: '#79e6a5',
        cursorAccent: '#07100b',
        selectionBackground: '#315d4766',
        black: '#111713',
        green: '#79e6a5',
        red: '#ff7d78',
        yellow: '#f3c969',
        blue: '#79b8ff',
        magenta: '#d7a4ff',
        cyan: '#72d6d3',
        white: '#dce8e1',
        brightBlack: '#718078',
        brightGreen: '#a0f3be',
        brightRed: '#ffaaa6',
        brightYellow: '#ffe39a',
        brightBlue: '#a6d2ff',
        brightMagenta: '#ebc7ff',
        brightCyan: '#a0eeeb',
        brightWhite: '#ffffff',
      },
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    const panLayer = document.createElement('div');
    panLayer.className = 'terminal-pan-layer';
    host.appendChild(panLayer);
    terminal.open(panLayer);
    const terminalElement = terminal.element;
    if (!terminalElement) {
      throw new Error('Terminal element was not created.');
    }
    terminal.options.disableStdin = true;

    // The 编辑 button must reach the terminal's own hidden input, otherwise iOS
    // keeps the software keyboard aimed at another field.
    focusTerminal.current = () => {
      terminal.focus();
      terminal.textarea?.focus({ preventScroll: true });
    };

    // TEMPORARY INPUT DIAGNOSTIC. Remove once the iOS event stream is known.
    //
    // The previous commit claimed committed IME text on `beforeinput` and it
    // changed nothing on the phone. Two guesses have now been wrong, so this
    // records what iOS actually delivers instead of inferring it: if the stream
    // shows no event carrying the character at all, no amount of interception
    // can help and the answer is that the page is never told.
    const helperTextarea = terminal.textarea;
    const recordInputEvent = (event: Event) => {
      setInputTrace((current) =>
        [...current, `${event.type} ${describeInputEvent(event)}`].slice(
          -INPUT_TRACE_LIMIT,
        ),
      );
    };
    if (helperTextarea) {
      for (const name of INPUT_TRACE_EVENTS) {
        helperTextarea.addEventListener(name, recordInputEvent, true);
      }
    }

    const blockInput = (message: string) => {
      inputIsBlocked = true;
      terminal.options.disableStdin = true;
      setInputBlocked(true);
      setInputWarning(message);
    };

    let lastSentCols = 0;
    let lastSentRows = 0;
    const sendTerminalResize = () => {
      if (socket?.readyState !== WebSocket.OPEN) return;
      const cols = Math.max(2, Math.min(500, terminal.cols));
      const rows = Math.max(1, Math.min(200, terminal.rows));
      if (cols === lastSentCols && rows === lastSentRows) return;
      socket.send(
        JSON.stringify({
          type: 'resize',
          cols,
          rows,
        }),
      );
      lastSentCols = cols;
      lastSentRows = rows;
    };

    const overviewContentSize = () => {
      const screen = terminalElement.querySelector<HTMLElement>('.xterm-screen');
      return {
        width: Math.max(1, screen?.offsetWidth ?? terminal.cols * 8),
        height: Math.max(1, screen?.offsetHeight ?? terminal.rows * 17),
      };
    };

    const measureOverviewFitScale = () => {
      const content = overviewContentSize();
      const hostWidth = host.clientWidth;
      const hostHeight = host.clientHeight;
      if (hostWidth <= 0 || hostHeight <= 0) return 1;
      // Fit means "never larger than the viewport". A scale above 1 would
      // magnify a Session that is already smaller than the phone.
      return Math.max(
        0.01,
        Math.min(1, hostWidth / content.width, hostHeight / content.height),
      );
    };

    const overviewBaseScale = () => overviewFitScale;

    const applyOverviewTransform = () => {
      if (!overviewMode) return;
      const content = overviewContentSize();
      const scale = overviewBaseScale() * overviewZoom;
      const scaledWidth = content.width * scale;
      const scaledHeight = content.height * scale;
      const stageWidth = Math.max(host.clientWidth, scaledWidth);
      const stageHeight = Math.max(host.clientHeight, scaledHeight);
      const offsetX = Math.max(0, (host.clientWidth - scaledWidth) / 2);
      const offsetY = Math.max(0, (host.clientHeight - scaledHeight) / 2);
      panLayer.style.width = `${stageWidth}px`;
      panLayer.style.height = `${stageHeight}px`;
      terminalElement.style.width = `${content.width}px`;
      terminalElement.style.height = `${content.height}px`;
      terminalElement.style.transform = `translate3d(${offsetX}px, ${offsetY}px, 0) scale(${scale})`;
    };

    const clearOverviewLayout = () => {
      host.classList.remove('overview');
      host.scrollLeft = 0;
      host.scrollTop = 0;
      panLayer.style.width = '';
      panLayer.style.height = '';
      terminalElement.style.width = '';
      terminalElement.style.height = '';
      terminalElement.style.transform = '';
    };

    const canonicalFullSize = async (): Promise<TerminalSize> => {
      try {
        const hostInfo = await getHost();
        return hostInfo.terminal_full_size ?? FALLBACK_FULL_SIZE;
      } catch {
        // A FULL overview is still useful from the mirrored size.
        return FALLBACK_FULL_SIZE;
      }
    };

    const setOverviewMode = async (enabled: boolean) => {
      if (enabled === overviewMode || disposed) return;
      setOverviewBusy(true);
      setOverviewError('');
      try {
        if (enabled) {
          const [session, canonical] = await Promise.all([
            getSession(sessionId),
            canonicalFullSize(),
          ]);
          const full = overviewTargetSize(canonical, session.active_pane);
          setOverviewSize(full);
          terminal.resize(full.cols, full.rows);
          overviewMode = true;
          overviewFitScale = 1;
          overviewZoom = 1;
          host.classList.add('overview');
          overviewFitScale = measureOverviewFitScale();
          host.scrollLeft = 0;
          host.scrollTop = 0;
          terminal.options.disableStdin = inputIsBlocked || stopped;
          terminal.refresh(0, Math.max(0, full.rows - 1));
          setOverview(true);
          sendTerminalResize();
          requestAnimationFrame(applyOverviewTransform);
        } else {
          overviewMode = false;
          clearOverviewLayout();
          terminal.options.disableStdin = inputIsBlocked || stopped;
          setOverview(false);
          try {
            fitAddon.fit();
          } catch {
            // The next resize pass will fit after layout settles.
          }
          sendTerminalResize();
        }
      } catch (caught) {
        overviewMode = false;
        clearOverviewLayout();
        setOverview(false);
        terminal.options.disableStdin = inputIsBlocked;
        try {
          fitAddon.fit();
        } catch {
          // Ignore layout timing; the regular resize pass will recover.
        }
        setOverviewError(
          caught instanceof Error ? caught.message : 'Could not load the full Session size.',
        );
      } finally {
        setOverviewBusy(false);
      }
    };

    toggleOverview.current = () => {
      void setOverviewMode(!overviewMode);
    };

    let overviewGesture:
      | {
          startZoom: number;
          startDistance: number;
          contentX: number;
          contentY: number;
        }
      | undefined;

    const touchDistance = (touches: TouchList) =>
      Math.hypot(
        touches[0]!.clientX - touches[1]!.clientX,
        touches[0]!.clientY - touches[1]!.clientY,
      );
    const touchCenter = (touches: TouchList) => ({
      x: (touches[0]!.clientX + touches[1]!.clientX) / 2,
      y: (touches[0]!.clientY + touches[1]!.clientY) / 2,
    });
    const beginOverviewGesture = (event: TouchEvent) => {
      if (!overviewMode || event.touches.length < 2) {
        overviewGesture = undefined;
        return;
      }
      const distance = touchDistance(event.touches);
      const center = touchCenter(event.touches);
      const terminalRect = terminalElement.getBoundingClientRect();
      const currentScale = overviewBaseScale() * overviewZoom;
      overviewGesture = {
        startZoom: overviewZoom,
        startDistance: Math.max(1, distance),
        contentX: (center.x - terminalRect.left) / currentScale,
        contentY: (center.y - terminalRect.top) / currentScale,
      };
    };
    const moveOverviewGesture = (event: TouchEvent) => {
      if (!overviewMode || !overviewGesture || event.touches.length < 2) return;
      event.preventDefault();
      const distance = touchDistance(event.touches);
      const center = touchCenter(event.touches);
      overviewZoom = Math.max(
        1,
        Math.min(8, overviewGesture.startZoom * (distance / overviewGesture.startDistance)),
      );
      applyOverviewTransform();
      const scale = overviewBaseScale() * overviewZoom;
      const hostRect = host.getBoundingClientRect();
      host.scrollLeft = overviewGesture.contentX * scale - (center.x - hostRect.left);
      host.scrollTop = overviewGesture.contentY * scale - (center.y - hostRect.top);
    };
    const endOverviewGesture = () => {
      overviewGesture = undefined;
    };

    const sendResize = () => {
      if (overviewMode) {
        applyOverviewTransform();
        return;
      }
      try {
        fitAddon.fit();
      } catch {
        // Keep the last known or default size; a resize is still useful before
        // the mobile layout has finished settling.
      }
      sendTerminalResize();
    };

    const scheduleFit = () => {
      if (resizeFrame !== undefined) cancelAnimationFrame(resizeFrame);
      resizeFrame = requestAnimationFrame(sendResize);
    };

    const send = (data: string): boolean => {
      if (
        disposed ||
        inputIsBlocked ||
        socket?.readyState !== WebSocket.OPEN
      ) {
        return false;
      }
      try {
        socket.send(new TextEncoder().encode(data));
        return true;
      } catch {
        blockInput(
          'The last terminal input could not be confirmed and will not be resent. Reconnect and inspect the output before continuing.',
        );
        return false;
      }
    };

    // One path for typed text, so the keybar and the software keyboard cannot
    // drift apart.
    const sendTyped = (text: string) => {
      const transformed = withControlModifier(
        halfWidth ? normalizeHalfWidth(text) : text,
        controlArmed.current,
      );
      if (send(transformed)) controlArmed.current = false;
    };

    sendRaw.current = send;
    const rawDisposable = terminal.onData(sendTyped);
    const binaryDisposable = terminal.onBinary((data) => {
      const bytes = Uint8Array.from(data, (character) => character.charCodeAt(0));
      if (
        disposed ||
        inputIsBlocked ||
        socket?.readyState !== WebSocket.OPEN
      ) {
        return;
      }
      try {
        socket.send(bytes);
      } catch {
        blockInput(
          'The last terminal input could not be confirmed and will not be resent. Reconnect and inspect the output before continuing.',
        );
      }
    });

    const clearReconnectTimer = () => {
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      reconnectTimer = undefined;
    };

    const clearConnectTimer = () => {
      if (connectTimer !== undefined) window.clearTimeout(connectTimer);
      connectTimer = undefined;
    };

    const scheduleReconnect = () => {
      if (disposed || stopped || reconnectTimer !== undefined) return;
      if (!navigator.onLine) {
        setConnection('offline');
        setRetryIn(0);
        return;
      }

      const delay =
        RECONNECT_DELAYS[
          Math.min(failedAttempts, RECONNECT_DELAYS.length - 1)
        ] ?? 10000;
      failedAttempts += 1;
      setConnection('reconnecting');
      setRetryIn(Math.ceil(delay / 1000));
      reconnectTimer = window.setTimeout(() => {
        reconnectTimer = undefined;
        connect();
      }, delay);
    };

    async function handleFrame(
      source: WebSocket,
      event: MessageEvent<ArrayBuffer | Blob | string>,
    ) {
      if (disposed || source !== socket) return;

      if (typeof event.data === 'string') {
        try {
          const frame = JSON.parse(event.data) as {
            type?: string;
            code?: string;
            message?: string;
          };
          if (frame.type === 'error') {
            const code = frame.code ?? 'TERMINAL_ERROR';
            if (code === 'SESSION_GONE') {
              stopped = true;
              terminal.options.disableStdin = true;
              setProtocolMessage('The Session no longer exists on the Mac.');
              setConnection('stopped');
              setRetryIn(0);
              source.close(1000, code);
            } else {
              setProtocolMessage(frame.message || code);
              source.close(1011, code);
            }
          } else if (frame.type === 'notice') {
            // The Mac repaired the Session's keyboard transport. Input stays
            // enabled; the notice only explains why typing starts working.
            if (frame.code === 'INPUT_MODE_REPAIRED' && frame.message) {
              setTransportNotice(frame.message);
            }
          } else {
            setProtocolMessage(
              `Unsupported server control frame: ${frame.type ?? 'unknown'}`,
            );
            source.close(1003, 'unsupported control frame');
          }
        } catch {
          setProtocolMessage('The Mac sent an invalid terminal control frame.');
          source.close(1003, 'invalid control frame');
        }
        return;
      }

      let bytes: Uint8Array;
      if (event.data instanceof Blob) {
        bytes = new Uint8Array(await event.data.arrayBuffer());
      } else {
        bytes = new Uint8Array(event.data);
      }
      if (!disposed && source === socket) terminal.write(bytes);
    }

    function connect() {
      if (disposed || stopped) return;
      if (
        socket &&
        (socket.readyState === WebSocket.CONNECTING ||
          socket.readyState === WebSocket.OPEN)
      ) {
        return;
      }
      clearReconnectTimer();
      clearConnectTimer();
      if (!navigator.onLine) {
        setConnection('offline');
        return;
      }

      setConnection(failedAttempts === 0 ? 'connecting' : 'reconnecting');
      try {
        const nextSocket = new WebSocket(terminalUrl(sessionId));
        socket = nextSocket;
        nextSocket.binaryType = 'arraybuffer';
        connectTimer = window.setTimeout(() => {
          if (disposed || socket !== nextSocket) return;
          if (nextSocket.readyState === WebSocket.CONNECTING) {
            nextSocket.close();
          }
        }, 5000);

        nextSocket.onopen = () => {
          clearConnectTimer();
          if (disposed || socket !== nextSocket) return;
          hadConnection = true;
          openedAt = Date.now();
          inputIsBlocked = false;
          terminal.options.disableStdin = inputIsBlocked || stopped;
          setInputBlocked(false);
          setInputWarning('');
          setProtocolMessage('');
          setTransportNotice('');
          setConnection('connected');
          setRetryIn(0);
          scheduleFit();
        };

        nextSocket.onmessage = (event) => {
          void handleFrame(nextSocket, event);
        };

        nextSocket.onerror = () => {
          // Browsers do not expose the HTTP status. The REST probe below
          // distinguishes a revoked session from an attach/network failure.
        };

        nextSocket.onclose = (event) => {
          clearConnectTimer();
          if (disposed || socket !== nextSocket) return;
          socket = undefined;
          terminal.options.disableStdin = true;
          setInputBlocked(true);
          if (stopped) {
            setConnection('stopped');
            return;
          }
          if (Date.now() - openedAt >= 2000) failedAttempts = 0;

          void getSession(sessionId)
            .then(() => {
              // The Session still exists; a fresh attachment may reconnect.
            })
            .catch((caught) => {
              if (
                caught instanceof ApiError &&
                (caught.status === 404 || caught.code === 'SESSION_NOT_FOUND')
              ) {
                stopped = true;
                setProtocolMessage('The Session no longer exists on the Mac.');
                setConnection('stopped');
                setRetryIn(0);
                return;
              }
              // A 401 has already routed the app to Pair. Other REST failures
              // do not prove that terminal reconnection should stop.
            });

          if (stopped) return;
          if (!hadConnection && [1006, 1008].includes(event.code)) {
            setProtocolMessage('The Mac could not authorize the terminal attachment.');
          }
          scheduleReconnect();
        };
      } catch {
        scheduleReconnect();
      }
    }

    const resumeConnection = () => {
      if (disposed || stopped) return;
      failedAttempts = 0;
      clearReconnectTimer();
      clearConnectTimer();
      if (socket?.readyState === WebSocket.OPEN) {
        setConnection('connected');
        scheduleFit();
        return;
      }
      if (socket) {
        const staleSocket = socket;
        socket = undefined;
        staleSocket.onclose = null;
        staleSocket.close();
      }
      if (!navigator.onLine) {
        setConnection('offline');
        return;
      }
      setConnection('connecting');
      connect();
    };
    const onOnline = () => resumeConnection();
    const onOffline = () => {
      clearConnectTimer();
      setConnection('offline');
      setRetryIn(0);
      if (socket) socket.close(1000, 'browser offline');
    };
    const onVisibility = () => {
      if (document.visibilityState === 'visible') resumeConnection();
    };

    const resizeObserver =
      typeof ResizeObserver === 'undefined'
        ? undefined
        : new ResizeObserver(scheduleFit);
    const viewport = window.visualViewport;
    resizeObserver?.observe(host);
    window.addEventListener('resize', scheduleFit);
    viewport?.addEventListener('resize', scheduleFit);

    // iOS Safari keeps the layout viewport at full height when the software
    // keyboard opens, so a terminal sized to the layout viewport ends up behind
    // the keyboard. Measure the inset between the two viewports and hand it to
    // the layout instead of guessing.
    const applyKeyboardInset = () => {
      if (!viewport) {
        setKeyboardInset(0);
        return;
      }
      const layoutHeight = window.innerHeight;
      const visibleBottom = viewport.offsetTop + viewport.height;
      const inset = Math.round(layoutHeight - visibleBottom);
      // Ignore rounding noise and anything implausible; a wrong value would
      // shrink the terminal for no reason.
      setKeyboardInset(inset > 80 && inset < layoutHeight * 0.75 ? inset : 0);
      scheduleFit();
    };
    viewport?.addEventListener('resize', applyKeyboardInset);
    viewport?.addEventListener('scroll', applyKeyboardInset);
    applyKeyboardInset();

    // "Jump to the live tail" needs to know whether the viewport has left it.
    const syncScrollPosition = () => {
      const buffer = terminal.buffer.active;
      const distance = Math.max(0, buffer.baseY - buffer.viewportY);
      setScrolledBack((current) => (current === distance ? current : distance));
    };
    const scrollDisposable = terminal.onScroll(syncScrollPosition);
    const renderDisposable = terminal.onRender(syncScrollPosition);

    scrollToTail.current = () => {
      terminal.scrollToBottom();
      syncScrollPosition();
    };

    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    document.addEventListener('visibilitychange', onVisibility);
    window.addEventListener('pageshow', resumeConnection);
    host.addEventListener('touchstart', beginOverviewGesture, { passive: false });
    host.addEventListener('touchmove', moveOverviewGesture, { passive: false });
    host.addEventListener('touchend', endOverviewGesture);
    host.addEventListener('touchcancel', endOverviewGesture);
    const preventOverviewPageZoom = (event: Event) => {
      if (overviewMode) event.preventDefault();
    };
    host.addEventListener('gesturestart', preventOverviewPageZoom, { passive: false });
    host.addEventListener('gesturechange', preventOverviewPageZoom, { passive: false });

    setConnection(navigator.onLine ? 'connecting' : 'offline');
    requestAnimationFrame(sendResize);
    connect();

    return () => {
      disposed = true;
      stopped = true;
      clearReconnectTimer();
      clearConnectTimer();
      if (resizeFrame !== undefined) cancelAnimationFrame(resizeFrame);
      resizeObserver?.disconnect();
      window.removeEventListener('resize', scheduleFit);
      viewport?.removeEventListener('resize', scheduleFit);
      viewport?.removeEventListener('resize', applyKeyboardInset);
      viewport?.removeEventListener('scroll', applyKeyboardInset);
      // TEMPORARY INPUT DIAGNOSTIC — remove with the recorder above.
      for (const name of INPUT_TRACE_EVENTS) {
        helperTextarea?.removeEventListener(name, recordInputEvent, true);
      }
      scrollDisposable.dispose();
      renderDisposable.dispose();
      window.removeEventListener('online', onOnline);
      window.removeEventListener('offline', onOffline);
      document.removeEventListener('visibilitychange', onVisibility);
      window.removeEventListener('pageshow', resumeConnection);
      host.removeEventListener('touchstart', beginOverviewGesture);
      host.removeEventListener('touchmove', moveOverviewGesture);
      host.removeEventListener('touchend', endOverviewGesture);
      host.removeEventListener('touchcancel', endOverviewGesture);
      host.removeEventListener('gesturestart', preventOverviewPageZoom);
      host.removeEventListener('gesturechange', preventOverviewPageZoom);
      toggleOverview.current = () => undefined;
      focusTerminal.current = () => undefined;
      rawDisposable.dispose();
      binaryDisposable.dispose();
      sendRaw.current = () => false;
      if (socket) {
        socket.onclose = null;
        socket.close(1000, 'terminal page closed');
      }
      terminal.dispose();
    };
  }, [sessionId, manualReconnect]);

  useEffect(() => {
    if (connection === 'connected') {
      controlArmed.current = false;
      setControlArmedState(false);
    }
  }, [connection]);

  const pressKey = (data: string) => {
    if (!sendRaw.current(halfWidth ? normalizeHalfWidth(data) : data)) return;
    controlArmed.current = false;
    setControlArmedState(false);
  };

  const toggleControl = () => {
    const next = !controlArmed.current;
    controlArmed.current = next;
    setControlArmedState(next);
  };

  const jumpToBottom = () => {
    scrollToTail.current();
    setScrolledBack(0);
  };
  const reconnect = () => {
    setRetryIn(0);
    setInputWarning('');
    setOverview(false);
    setManualReconnect((value) => value + 1);
  };

  const submitPrompt = async (event: Event) => {
    event.preventDefault();
    const text = prompt.trim();
    if (!text || sendingPrompt || connection !== 'connected') return;

    const requestId = promptRequest.current ?? makeRequestId();
    promptRequest.current = requestId;
    writePendingPrompt(promptStorageKey, requestId);
    setSendingPrompt(true);
    setPromptNotice(undefined);

    try {
      await sendSessionPrompt(sessionId, { request_id: requestId, text });
      setPrompt('');
      promptRequest.current = undefined;
      clearPendingPrompt(promptStorageKey);
      setPromptNotice({ kind: 'success', message: 'Prompt delivery confirmed.' });
    } catch (caught) {
      if (isUncertainOutcome(caught)) {
        setPromptNotice({
          kind: 'uncertain',
          message:
            'Prompt delivery is uncertain. Nothing was resent. Inspect the terminal output before entering a new prompt.',
        });
      } else {
        promptRequest.current = undefined;
        clearPendingPrompt(promptStorageKey);
        setPromptNotice({
          kind: 'error',
          message:
            caught instanceof ApiError
              ? caught.message
              : 'Prompt delivery failed.',
        });
      }
    } finally {
      setSendingPrompt(false);
    }
  };

  const terminalInputDisabled = connection !== 'connected' || inputBlocked;
  const promptUncertain = promptNotice?.kind === 'uncertain';
  const promptDisabled =
    terminalInputDisabled || overview || sendingPrompt || promptUncertain;
  // FULL is a whole-Session view; the Structured Prompt dock would only take
  // space there. An unconfirmed prompt still has to stay reachable, because it
  // is the one thing in that dock the user may have to act on.
  const showPromptDock = !overview || promptUncertain;

  return (
    <div
      class="terminal-page"
      // A custom property rather than a padding value, so the layout rule in
      // styles.css stays the single place that decides what the inset means.
      style={
        keyboardInset
          ? ({ '--keyboard-inset': `${keyboardInset}px` } as JSX.CSSProperties)
          : undefined
      }
    >
      <header class="terminal-header">
        <a
          class="terminal-back"
          href={`#${sessionPath(sessionId)}`}
          aria-label="Back to Session"
        >
          ‹
        </a>
        <div class="terminal-title">
          <span>Session</span>
          <strong>{sessionLabel || sessionId}</strong>
        </div>
        <div class={`connection-pill connection-${connection}`}>
          <span aria-hidden="true" />
          {connection === 'connected' ? 'Live' : connection}
        </div>
      </header>

      <main class="terminal-workspace">
        <div class="terminal-canvas">
          <div class="terminal-host" ref={terminalHost} />
          {connection !== 'connected' && (
            <div class="connection-overlay" role="status" aria-live="polite">
              <div class="connection-overlay-card">
                {connection === 'offline' ? (
                  <>
                    <strong>Mac is currently unreachable.</strong>
                    <p>Terminal input is disabled. No input will be queued.</p>
                  </>
                ) : connection === 'stopped' ? (
                  <>
                    <strong>Terminal attachment stopped.</strong>
                    <p>{protocolMessage || 'The Session is no longer available.'}</p>
                    <a class="button button-secondary button-small" href="#/">
                      Back to Home
                    </a>
                  </>
                ) : (
                  <>
                    <strong>
                      {connection === 'connecting'
                        ? 'Connecting to the Session…'
                        : 'Connection lost.'}
                    </strong>
                    <p>
                      {connection === 'reconnecting'
                        ? `Your session is still running on the Mac. Reconnecting in ${retryIn}s…`
                        : 'A fresh, disposable terminal attachment is being created.'}
                    </p>
                    {connection === 'reconnecting' && (
                      <button
                        class="button button-secondary button-small"
                        type="button"
                        onClick={reconnect}
                      >
                        Reconnect now
                      </button>
                    )}
                    {protocolMessage && <small>{protocolMessage}</small>}
                  </>
                )}
              </div>
            </div>
          )}

          {/* The one floating action. It stays out of the grid so the terminal
              keeps every row, and it only appears once the viewport has left
              the live tail. */}
          {/* TEMPORARY INPUT DIAGNOSTIC — remove once the iOS event stream is
              known. It records what the phone delivers so the Chinese-keyboard
              failure can be read instead of guessed. */}
          <button
            class="canvas-action canvas-action-left"
            type="button"
            onClick={() => setTraceOpen((open) => !open)}
            aria-expanded={traceOpen}
            aria-label="Show recorded input events"
            title="TEMPORARY input diagnostic"
          >
            ⌦ trace
          </button>

          {scrolledBack > 0 && (
            <button
              className="canvas-action"
              type="button"
              onClick={jumpToBottom}
            >
              <span aria-hidden="true">↓</span>
              {scrolledBack} lines back
            </button>
          )}
        </div>

        {overview && (
          <div className="overview-hint" role="status">
            <strong>
              Full
              {overviewSize ? ` ${overviewSize.cols}×${overviewSize.rows}` : ''}
            </strong>
            <span>Editable · pinch · pan</span>
          </div>
        )}
        {overviewError && (
          <div className="overview-error" role="alert">
            {overviewError}
          </div>
        )}

        {inputWarning && connection === 'connected' && (
          <div class="terminal-warning" role="alert">
            <span>{inputWarning}</span>
            <button class="text-button" type="button" onClick={reconnect}>
              Reconnect
            </button>
          </div>
        )}

        {transportNotice && connection === 'connected' && (
          <div class="terminal-notice" role="status">
            <span>{transportNotice}</span>
            <button
              class="text-button"
              type="button"
              onClick={() => setTransportNotice('')}
            >
              Dismiss
            </button>
          </div>
        )}

        <div
          class="terminal-keybar"
          role="toolbar"
          aria-label="Terminal keys"
        >
          {/* TEMPORARY INPUT DIAGNOSTIC — remove once the iOS event stream is
              known. It records what the phone delivers so the Chinese-keyboard
              failure can be read instead of guessed. */}
          {traceOpen && (
            <div class="input-trace">
              <textarea
                readOnly
                rows={6}
                value={
                  inputTrace.length
                    ? inputTrace.join('\n')
                    : 'No events recorded yet. Tap 编辑, then the keys, then reopen.'
                }
                aria-label="Recorded input events"
              />
              <div class="input-trace-actions">
                <button
                  type="button"
                  onClick={() => setInputTrace([])}
                >
                  Clear
                </button>
                <button type="button" onClick={() => setTraceOpen(false)}>
                  Close
                </button>
              </div>
            </div>
          )}
          <div
            class="keybar-primary"
            role="group"
            aria-label="Terminal controls"
          >
            <button
              type="button"
              onClick={() => pressKey('\x1b')}
              disabled={terminalInputDisabled}
            >
              ESC
            </button>
            <button
              type="button"
              onClick={() => pressKey('\t')}
              disabled={terminalInputDisabled}
            >
              TAB
            </button>
            <button
              class={controlArmedState ? 'armed' : ''}
              type="button"
              aria-pressed={controlArmedState}
              onClick={toggleControl}
              disabled={terminalInputDisabled}
            >
              CTRL
            </button>
            <button
              class={overview ? 'armed overview-toggle' : 'overview-toggle'}
              type="button"
              aria-pressed={overview}
              onClick={() => toggleOverview.current()}
              disabled={overviewBusy || (!overview && connection !== 'connected')}
            >
              {overviewBusy ? '…' : overview ? 'LOCAL' : 'FULL'}
            </button>
            <button
              class="keyboard-button"
              type="button"
              onClick={() => focusTerminal.current()}
              disabled={terminalInputDisabled}
              aria-label="Show terminal keyboard for editing"
            >
              <span aria-hidden="true">⌨</span>
              <span>编辑</span>
            </button>
          </div>
          <div
            class="direction-keys"
            role="group"
            aria-label="Arrow keys"
          >
            <button
              type="button"
              onClick={() => pressKey('\x1b[A')}
              disabled={terminalInputDisabled}
              aria-label="Arrow up"
            >
              ↑
            </button>
            <button
              type="button"
              onClick={() => pressKey('\x1b[B')}
              disabled={terminalInputDisabled}
              aria-label="Arrow down"
            >
              ↓
            </button>
            <button
              type="button"
              onClick={() => pressKey('\x1b[D')}
              disabled={terminalInputDisabled}
              aria-label="Arrow left"
            >
              ←
            </button>
            <button
              type="button"
              onClick={() => pressKey('\x1b[C')}
              disabled={terminalInputDisabled}
              aria-label="Arrow right"
            >
              →
            </button>
            <button
              class="return-button"
              type="button"
              onClick={() => pressKey('\r')}
              disabled={terminalInputDisabled}
              aria-label="Send Return"
              title="Send Return"
            >
              ⏎
            </button>
          </div>
        </div>

        {showPromptDock && (
          <form class="prompt-dock" onSubmit={submitPrompt}>
            {promptNotice && (
              <div
                class={`prompt-notice prompt-notice-${promptNotice.kind}`}
                role={promptNotice.kind === 'success' ? 'status' : 'alert'}
              >
                <span>{promptNotice.message}</span>
                {promptNotice.kind === 'uncertain' && (
                  <button
                    class="text-button"
                    type="button"
                    onClick={() => {
                      setPrompt('');
                      promptRequest.current = undefined;
                      clearPendingPrompt(promptStorageKey);
                      setPromptNotice(undefined);
                    }}
                  >
                    Clear prompt
                  </button>
                )}
                {promptNotice.kind === 'success' && (
                  <button
                    class="notice-dismiss"
                    type="button"
                    onClick={() => setPromptNotice(undefined)}
                    aria-label="Dismiss prompt status"
                  >
                    ×
                  </button>
                )}
              </div>
            )}
            <div class="prompt-input-row">
              <textarea
                value={prompt}
                onInput={(event) => setPrompt(event.currentTarget.value)}
                rows={2}
                placeholder={
                  overview
                    ? 'Structured Prompt is available in LOCAL mode'
                    : connection === 'connected'
                      ? 'Prompt this session…'
                      : 'Reconnect to send a prompt'
                }
                disabled={promptDisabled}
                aria-label="Structured session prompt"
              />
              <button
                class="prompt-send"
                type="submit"
                disabled={promptDisabled || !prompt.trim()}
              >
                {sendingPrompt ? '…' : 'Send'}
              </button>
            </div>
            <p class="at-most-once-note">
              Prompt actions use a request ID. Terminal keystrokes are sent once
              and never replayed after uncertainty.
            </p>
          </form>
        )}
      </main>
    </div>
  );
}
