import { useEffect, useState } from 'preact/hooks';
import { bindDeviceInstallation, getMe, UNAUTHORIZED_EVENT } from './api';
import { AppFrame } from './components/ui';
import { HomePage } from './pages/HomePage';
import { LinkBrowserPage } from './pages/LinkBrowserPage';
import { PairConsolePage } from './pages/PairConsolePage';
import { PairPage } from './pages/PairPage';
import { SessionPage } from './pages/SessionPage';
import { SettingsPage } from './pages/SettingsPage';
import { StartWorkPage } from './pages/StartWorkPage';
import { TerminalPage } from './pages/TerminalPage';
import { replaceRoute, useHashRoute } from './router';

export function App() {
  const route = useHashRoute();
  const [pairReason, setPairReason] = useState('');

  useEffect(() => {
    if (
      window.location.hash.startsWith('#/link') ||
      window.location.hash.startsWith('#/pair-console')
    ) {
      return;
    }
    void getMe()
      .then(() => bindDeviceInstallation())
      .catch(() => {
        // The API client emits the unauthorized event used by the pair route.
        // Binding is a best-effort migration for pre-installation-ID records.
      });
  }, []);

  useEffect(() => {
    const onUnauthorized = (event: Event) => {
      const detail = (event as CustomEvent<{ code?: string }>).detail;
      setPairReason(
        detail?.code === 'PAIR_TOKEN_EXPIRED'
          ? 'The pairing code has expired.'
          : 'This browser is not paired, or its access was revoked. Pair again to continue.',
      );
      if (
        window.location.hash.startsWith('#token=') ||
        window.location.hash.startsWith('#/link') ||
        window.location.hash.startsWith('#/pair-console')
      ) {
        return;
      }
      replaceRoute('/pair');
    };

    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
  }, []);

  useEffect(() => {
    const titles: Record<string, string> = {
      home: 'mctrl',
      pair: 'Pair device · mctrl',
      'pair-console': 'Pair phone · mctrl',
      link: 'Link browser · mctrl',
      start: 'Start Work · mctrl',
      settings: 'Settings · mctrl',
      session: 'Session · mctrl',
      terminal: 'Terminal · mctrl',
    };
    document.title = titles[route.name] ?? 'mctrl';
  }, [route.name]);

  if (route.name === 'pair') {
    return <PairPage reason={pairReason} initialToken={route.pairingToken} />;
  }
  if (route.name === 'pair-console') {
    return <PairConsolePage initialToken={route.pairingToken} />;
  }
  if (route.name === 'link') {
    return <LinkBrowserPage initialToken={route.linkToken} />;
  }
  if (route.name === 'terminal') {
    return <TerminalPage sessionId={route.sessionId} />;
  }

  return (
    <AppFrame route={route}>
      {route.name === 'home' && <HomePage />}
      {route.name === 'start' && <StartWorkPage />}
      {route.name === 'settings' && <SettingsPage />}
      {route.name === 'session' && <SessionPage sessionId={route.sessionId} />}
    </AppFrame>
  );
}
