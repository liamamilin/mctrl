import type { ComponentChildren } from 'preact';
import type { Route } from '../router';

export function LoadingBlock({ label = 'Loading' }: { label?: string }) {
  return (
    <div class="loading-block" role="status" aria-live="polite">
      <span class="spinner" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}

export function ErrorNotice({
  title = 'Could not load this page',
  message,
  onRetry,
}: {
  title?: string;
  message: string;
  onRetry?: () => void;
}) {
  return (
    <section class="notice notice-error" role="alert">
      <div class="notice-symbol" aria-hidden="true">
        !
      </div>
      <div class="notice-content">
        <strong>{title}</strong>
        <p>{message}</p>
        {onRetry && (
          <button class="button button-quiet button-small" onClick={onRetry}>
            Try again
          </button>
        )}
      </div>
    </section>
  );
}

export function EmptyState({
  title,
  message,
  action,
}: {
  title: string;
  message: string;
  action?: ComponentChildren;
}) {
  return (
    <section class="empty-state">
      <div class="empty-mark" aria-hidden="true">
        <span />
        <span />
        <span />
      </div>
      <h2>{title}</h2>
      <p>{message}</p>
      {action}
    </section>
  );
}

export function StatusDot({
  status,
  pulse = false,
}: {
  status: 'online' | 'reconnecting' | 'offline' | 'unknown';
  pulse?: boolean;
}) {
  return (
    <span
      class={`status-dot status-${status}${pulse ? ' status-pulse' : ''}`}
      aria-hidden="true"
    />
  );
}

export function TransportNotice({
  transport,
  compact = false,
}: {
  transport?: string;
  compact?: boolean;
}) {
  const normalized = transport?.toLowerCase() ?? '';
  const trustedLanHttp =
    normalized === 'trusted_lan_http' ||
    (normalized.includes('trusted') && normalized.includes('http')) ||
    (!normalized && window.location.protocol === 'http:');
  const httpsProfile = normalized.includes('https') || normalized.includes('wss');

  if (trustedLanHttp) {
    return (
      <aside class={`transport-notice${compact ? ' compact' : ''}`}>
        <div class="transport-icon" aria-hidden="true">
          !
        </div>
        <div>
          <strong>Trusted-LAN HTTP</strong>
          <p>
            No end-to-end transport encryption. A malicious or actively
            monitored LAN may observe or tamper with traffic. Use only on a
            network you trust.
          </p>
        </div>
      </aside>
    );
  }

  if (httpsProfile || window.location.protocol === 'https:') {
    return (
      <aside class={`transport-notice transport-https${compact ? ' compact' : ''}`}>
        <div class="transport-icon" aria-hidden="true">
          T
        </div>
        <div>
          <strong>HTTPS/WSS profile</strong>
          <p>
            The current page uses transport protection. Your TLS endpoint and
            certificate configuration remain part of the trust boundary.
          </p>
        </div>
      </aside>
    );
  }

  return (
    <aside class={`transport-notice transport-neutral${compact ? ' compact' : ''}`}>
      <div class="transport-icon" aria-hidden="true">
        T
      </div>
      <div>
        <strong>Transport profile not confirmed</strong>
        <p>
          mctrl cannot verify the configured transport until it receives Host
          status.
        </p>
      </div>
    </aside>
  );
}

function navIcon(name: 'home' | 'start' | 'settings') {
  if (name === 'home') {
    return (
      <svg viewBox="0 0 24 24" aria-hidden="true">
        <path d="M3.5 10.7 12 3.8l8.5 6.9v8.8a1 1 0 0 1-1 1h-5v-6h-5v6h-5a1 1 0 0 1-1-1v-8.8Z" />
      </svg>
    );
  }
  if (name === 'start') {
    return (
      <svg viewBox="0 0 24 24" aria-hidden="true">
        <path d="M12 4v16M4 12h16" />
      </svg>
    );
  }
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M12 15.2a3.2 3.2 0 1 0 0-6.4 3.2 3.2 0 0 0 0 6.4Z" />
      <path d="M19.1 13.2a7.7 7.7 0 0 0 0-2.4l2-1.5-2-3.4-2.4 1a8.1 8.1 0 0 0-2-1.2L14.3 3H10l-.4 2.7a8.1 8.1 0 0 0-2 1.2l-2.4-1-2 3.4 2 1.5a7.7 7.7 0 0 0 0 2.4l-2 1.5 2 3.4 2.4-1a8.1 8.1 0 0 0 2 1.2l.4 2.7h4.3l.4-2.7a8.1 8.1 0 0 0 2-1.2l2.4 1 2-3.4-2-1.5Z" />
    </svg>
  );
}

function BottomNav({ route }: { route: Route }) {
  const isActive = (name: 'home' | 'start' | 'settings') => {
    if (name === 'home') {
      return route.name === 'home' || route.name === 'session';
    }
    return route.name === name;
  };

  const items = [
    { name: 'home' as const, label: 'Home', href: '#/' },
    { name: 'start' as const, label: 'Start', href: '#/start' },
    { name: 'settings' as const, label: 'Settings', href: '#/settings' },
  ];

  return (
    <nav class="bottom-nav" aria-label="Primary navigation">
      <div class="bottom-nav-inner">
        {items.map((item) => (
          <a
            key={item.name}
            href={item.href}
            class={isActive(item.name) ? 'active' : ''}
            aria-current={isActive(item.name) ? 'page' : undefined}
          >
            {navIcon(item.name)}
            <span>{item.label}</span>
          </a>
        ))}
      </div>
    </nav>
  );
}

export function AppFrame({
  children,
  route,
  headerAction,
}: {
  children: ComponentChildren;
  route: Route;
  headerAction?: ComponentChildren;
}) {
  return (
    <div class="app-shell">
      <header class="app-header">
        <a class="brand" href="#/" aria-label="mctrl home">
          <span class="brand-mark" aria-hidden="true">
            m
          </span>
          <span>mctrl</span>
        </a>
        <div class="header-action">{headerAction}</div>
      </header>
      <main class="page-content">{children}</main>
      <BottomNav route={route} />
    </div>
  );
}
