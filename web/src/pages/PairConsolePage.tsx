import { useEffect, useRef, useState } from 'preact/hooks';
import { getPairingQR } from '../api';
import { formatDate } from '../format';
import { TransportNotice } from '../components/ui';

export function PairConsolePage({ initialToken }: { initialToken?: string }) {
  const [qrDataURL, setQRDataURL] = useState('');
  const [pairURL, setPairURL] = useState('');
  const [expiresAt, setExpiresAt] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [copyNotice, setCopyNotice] = useState('');
  const codeInput = useRef<HTMLInputElement>(null);
  const urlInput = useRef<HTMLInputElement>(null);

  const load = async () => {
    const token = initialToken?.trim() ?? '';
    if (!token) {
      setError('This Pair Console was opened without a one-time token. Double-click Mctrl Pair.app again.');
      setQRDataURL('');
      return;
    }
    setLoading(true);
    setError('');
    setCopyNotice('');
    try {
      const result = await getPairingQR(token);
      setQRDataURL(result.data_url);
      setPairURL(result.pair_url);
      setExpiresAt(result.expires_at ?? '');
    } catch (caught) {
      setQRDataURL('');
      setError(
        caught instanceof Error
          ? caught.message
          : 'Could not render the phone pairing QR code.',
      );
    } finally {
      setLoading(false);
    }
  };

  const copyValue = async (
    value: string,
    input: HTMLInputElement | null,
    label: string,
  ) => {
    if (!value) return;
    setCopyNotice('');
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(value);
      setCopyNotice(`${label} copied.`);
    } catch {
      input?.focus();
      input?.select();
      setCopyNotice(`${label} selected. Use the macOS Copy command.`);
    }
  };

  useEffect(() => {
    void load();
  }, [initialToken]);

  return (
    <div class="pair-page pair-console-page">
      <header class="pair-header">
        <a class="brand" href="#/" aria-label="mctrl home">
          <span class="brand-mark" aria-hidden="true">
            m
          </span>
          <span>mctrl</span>
        </a>
        <span class="pair-header-label">Mac pairing console</span>
      </header>

      <main class="pair-main">
        <section class="pair-intro">
          <div class="pair-kicker">Pair your iPhone</div>
          <h1>Scan on the phone</h1>
          <p>
            This Mac page is only a pairing console. It does not pair the Mac
            browser as a mobile device.
          </p>
        </section>

        <TransportNotice compact />

        <section class="pair-console-card card">
          {loading ? (
            <div class="pair-console-state">Rendering secure QR code…</div>
          ) : qrDataURL ? (
            <>
              <img
                class="pair-console-qr"
                src={qrDataURL}
                alt="One-time mctrl phone pairing QR code"
              />
              <div class="pair-console-instructions">
                <strong>On your iPhone</strong>
                <ol>
                  <li>Open the Camera and scan this code.</li>
                  <li>Open the mctrl Pair page.</li>
                  <li>Confirm the device name and pair.</li>
                </ol>
              </div>
              {initialToken && (
                <div class="pair-console-code-box">
                  <span>Pairing code</span>
                  <div>
                    <input
                      ref={codeInput}
                      type="text"
                      value={initialToken}
                      readOnly
                      aria-label="mctrl pairing code"
                      onFocus={(event) => event.currentTarget.select()}
                    />
                    <button
                      class="button button-secondary button-small"
                      type="button"
                      onClick={() =>
                        void copyValue(initialToken, codeInput.current, 'Pairing code')
                      }
                    >
                      Copy code
                    </button>
                  </div>
                </div>
              )}
              {pairURL && (
                <div class="pair-console-code-box">
                  <span>Can’t scan? Open this address on the iPhone</span>
                  <div>
                    <input
                      ref={urlInput}
                      type="text"
                      value={pairURL}
                      readOnly
                      aria-label="mctrl manual pairing address"
                      onFocus={(event) => event.currentTarget.select()}
                    />
                    <button
                      class="button button-secondary button-small"
                      type="button"
                      onClick={() =>
                        void copyValue(pairURL, urlInput.current, 'Pairing address')
                      }
                    >
                      Copy address
                    </button>
                  </div>
                  <small>Then enter the pairing code above on the phone.</small>
                </div>
              )}
              {copyNotice && (
                <p class="pair-console-copy-notice" role="status">
                  {copyNotice}
                </p>
              )}
              {expiresAt && (
                <p class="pair-console-expiry">
                  One-time code expires {formatDate(expiresAt)}.
                </p>
              )}
            </>
          ) : (
            <div class="form-error" role="alert">
              <strong>Pair Console unavailable</strong>
              <span>{error || 'The QR code could not be rendered.'}</span>
            </div>
          )}
        </section>

        <button
          class="button button-secondary button-full"
          type="button"
          onClick={() => void load()}
          disabled={loading}
        >
          {loading ? 'Refreshing…' : 'Refresh QR display'}
        </button>
      </main>
    </div>
  );
}
