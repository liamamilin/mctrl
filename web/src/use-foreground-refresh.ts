import { useEffect, useRef } from 'preact/hooks';

// Phone suspension stops timers. Refresh facts when returning instead of
// waiting for the next periodic tick. This hook never sends terminal input.
export function useForegroundRefresh(refresh: () => void): void {
  const current = useRef(refresh);
  current.current = refresh;
  useEffect(() => {
    let lastWake = 0;
    const wake = () => {
      if (document.visibilityState !== 'visible') return;
      const now = Date.now();
      if (now - lastWake < 250) return;
      lastWake = now;
      current.current();
    };
    const restore = (event: PageTransitionEvent) => {
      if (event.persisted) wake();
    };
    document.addEventListener('visibilitychange', wake);
    window.addEventListener('focus', wake);
    window.addEventListener('pageshow', restore);
    return () => {
      document.removeEventListener('visibilitychange', wake);
      window.removeEventListener('focus', wake);
      window.removeEventListener('pageshow', restore);
    };
  }, []);
}
