import { useEffect, useRef, useState } from 'preact/hooks';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import {
  API_BASE,
  ApiError,
  getSession,
  isUncertainOutcome,
  sendSessionPrompt,
} from '../api';
import { makeRequestId } from '../format';
import { sessionPath } from '../router';

const RECONNECT_DELAYS = [500, 1000, 2000, 5000, 10000] as const;

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
  const toggleOverview = useRef<() => void>(() => undefined);
  const controlArmed = useRef(false);
  const promptStorageKey = `mctrl-prompt:${sessionId}`;
  const initialPromptRequest = readPendingPrompt(promptStorageKey);
  const promptRequest = useRef<string | undefined>(initialPromptRequest);
  const [connection, setConnection] =
    useState<ConnectionState>('connecting');
  const [retryIn, setRetryIn] = useState(0);
  const [protocolMessage, setProtocolMessage] = useState('');
  const [inputBlocked, setInputBlocked] = useState(false);
  const [inputWarning, setInputWarning] = useState('');
  const [controlArmedState, setControlArmedState] = useState(false);
  const [manualReconnect, setManualReconnect] = useState(0);
  const [overview, setOverview] = useState(false);
  const [overviewBusy, setOverviewBusy] = useState(false);
  const [overviewError, setOverviewError] = useState('');
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

    const blockInput = (message: string) => {
      inputIsBlocked = true;
      terminal.options.disableStdin = true;
      setInputBlocked(true);
      setInputWarning(message);
    };

    const sendTerminalResize = () => {
      if (socket?.readyState !== WebSocket.OPEN) return;
      socket.send(
        JSON.stringify({
          type: 'resize',
          cols: terminal.cols,
          rows: terminal.rows,
        }),
      );
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
      return Math.max(
        0.01,
        Math.min(host.clientWidth / content.width, host.clientHeight / content.height),
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

    const setOverviewMode = async (enabled: boolean) => {
      if (enabled === overviewMode || disposed) return;
      setOverviewBusy(true);
      setOverviewError('');
      try {
        if (enabled) {
          const session = await getSession(sessionId);
          const cols = Math.max(20, Math.min(500, Math.round(session.active_pane?.width ?? 80)));
          const rows = Math.max(10, Math.min(200, Math.round(session.active_pane?.height ?? 24)));
          terminal.resize(cols, rows);
          overviewMode = true;
          overviewFitScale = 1;
          overviewZoom = 1;
          host.classList.add('overview');
          overviewFitScale = measureOverviewFitScale();
          host.scrollLeft = 0;
          host.scrollTop = 0;
          terminal.options.disableStdin = inputIsBlocked || stopped;
          terminal.refresh(0, Math.max(0, rows - 1));
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
        return;
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

    sendRaw.current = send;
    const rawDisposable = terminal.onData((data) => {
      const transformed = withControlModifier(data, controlArmed.current);
      if (send(transformed)) controlArmed.current = false;
    });
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
    resizeObserver?.observe(host);
    window.addEventListener('resize', scheduleFit);
    window.visualViewport?.addEventListener('resize', scheduleFit);
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
      window.visualViewport?.removeEventListener('resize', scheduleFit);
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
    if (sendRaw.current(data)) controlArmed.current = false;
  };

  const toggleControl = () => {
    const next = !controlArmed.current;
    controlArmed.current = next;
    setControlArmedState(next);
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

  return (
    <div class="terminal-page">
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
          <strong>{sessionId}</strong>
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
        </div>

        {overview && (
          <div className="overview-hint" role="status">
            <strong>Full Session overview</strong>
            <span>Editable. Pinch to zoom, drag to pan, then use the keyboard button to type.</span>
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

        <div class="terminal-keybar" aria-label="Terminal keys">
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
          <span class="keybar-divider" />
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
            class="keyboard-button"
            type="button"
            onClick={() =>
              terminalHost.current
                ?.querySelector<HTMLTextAreaElement>('.xterm-helper-textarea')
                ?.focus()
            }
            disabled={terminalInputDisabled}
            aria-label="Show terminal keyboard"
          >
            ⌨
          </button>
        </div>

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
      </main>
    </div>
  );
}
