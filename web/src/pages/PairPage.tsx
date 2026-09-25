import { useState } from 'preact/hooks';
import { ApiError, isUncertainOutcome, pairDevice } from '../api';
import { navigate } from '../router';
import { TransportNotice } from '../components/ui';

export function PairPage({
  reason,
  initialToken,
}: {
  reason?: string;
  initialToken?: string;
}) {
  const [token, setToken] = useState(initialToken ?? '');
  const [deviceName, setDeviceName] = useState('My iPhone');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [uncertain, setUncertain] = useState(false);

  const submit = async (event: Event) => {
    event.preventDefault();
    const cleanToken = token.trim();
    const cleanName = deviceName.trim();
    if (!cleanToken || !cleanName || submitting) return;

    setSubmitting(true);
    setError('');
    setUncertain(false);

    try {
      await pairDevice(cleanToken, cleanName);
      setToken('');
      navigate('/');
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(
          caught.code === 'PAIR_TOKEN_EXPIRED'
            ? 'This pairing code has expired. Run mctrl pair on the Mac to open a new short-lived window.'
            : caught.message,
        );
      } else {
        setError('Pairing failed. Check the code and try again.');
      }
      if (isUncertainOutcome(caught)) {
        setUncertain(true);
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div class="pair-page">
      <header class="pair-header">
        <a class="brand" href="#/" aria-label="mctrl home">
          <span class="brand-mark" aria-hidden="true">
            m
          </span>
          <span>mctrl</span>
        </a>
        <span class="pair-header-label">Device pairing</span>
      </header>

      <main class="pair-main">
        <section class="pair-intro">
          <div class="pair-kicker">Mobile control for your Mac</div>
          <h1>Connect this device</h1>
          <p>
            Pair only with the Mac you control. mctrl gives a paired device
            privileged terminal access.
          </p>
        </section>

        <TransportNotice compact />

        <form class="pair-form card" onSubmit={submit}>
          {reason && !error && (
            <div class="inline-message" role="status">
              {reason}
            </div>
          )}

          <label class="field">
            <span>Pairing code</span>
            <input
              type="password"
              value={token}
              onInput={(event) => setToken(event.currentTarget.value)}
              placeholder="Enter the short-lived code"
              autoComplete="one-time-code"
              autoCapitalize="off"
              autoCorrect="off"
              spellcheck={false}
              required
              disabled={submitting}
            />
            <small>Run <code>mctrl pair</code> on the Mac to open pairing.</small>
          </label>

          <label class="field">
            <span>Device name</span>
            <input
              type="text"
              value={deviceName}
              onInput={(event) => setDeviceName(event.currentTarget.value)}
              autoComplete="off"
              maxLength={80}
              required
              disabled={submitting}
            />
            <small>Use a name you will recognize in Settings.</small>
          </label>

          {error && (
            <div class="form-error" role="alert">
              <strong>Pairing did not complete</strong>
              <span>{error}</span>
              {uncertain && (
                <span>
                  The connection failed before a result was confirmed. No code
                  was submitted again automatically; follow the Mac instructions
                  before trying manually.
                </span>
              )}
            </div>
          )}

          <button
            class="button button-primary button-full"
            type="submit"
            disabled={submitting || !token.trim() || !deviceName.trim()}
          >
            {submitting ? <span class="button-spinner" /> : null}
            {submitting ? 'Pairing…' : 'Pair this device'}
          </button>
        </form>

        <p class="pair-footnote">
          Pairing is closed by default. A successful code is one-time use and
          expires quickly. This browser installation remains paired for 30 days;
          pairing it again reuses its device record.
        </p>
      </main>
    </div>
  );
}
