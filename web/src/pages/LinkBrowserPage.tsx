import { useEffect, useState } from 'preact/hooks';
import { ApiError, isUncertainOutcome, linkDeviceBrowser } from '../api';
import { replaceRoute } from '../router';
import { TransportNotice } from '../components/ui';

export function LinkBrowserPage({ initialToken }: { initialToken?: string }) {
  const [token, setToken] = useState(initialToken ?? '');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!initialToken) return;
    setToken(initialToken);
    setError('');
    setSubmitting(false);
  }, [initialToken]);

  const submit = async (event: Event) => {
    event.preventDefault();
    const cleanToken = token.trim();
    if (!cleanToken || submitting) return;

    setSubmitting(true);
    setError('');
    try {
      await linkDeviceBrowser(cleanToken);
      setToken('');
      replaceRoute('/');
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(
          caught.code === 'DEVICE_LINK_EXPIRED'
            ? 'This browser link has expired. Create a new link from the already paired PWA.'
            : isUncertainOutcome(caught)
              ? 'The link may already have been consumed, but the result could not be confirmed. It will not be retried automatically; create a new link if needed.'
              : caught.message,
        );
      } else {
        setError('The browser could not be linked. The link will not be retried automatically; create a new one if needed.');
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
        <span class="pair-header-label">Browser linking</span>
      </header>

      <main class="pair-main">
        <section class="pair-intro">
          <div class="pair-kicker">Use mctrl in another browser</div>
          <h1>Link this browser</h1>
          <p>
            A browser link connects Safari to the device record already paired on
            your PWA. It grants the same privileged terminal access.
          </p>
        </section>

        <TransportNotice compact />

        <form class="pair-form card" onSubmit={submit}>
          <div class="inline-message warning" role="note">
            Continue only when you created this link from your own paired PWA.
          </div>

          <label class="field">
            <span>One-time browser link</span>
            <input
              type="password"
              value={token}
              onInput={(event) => setToken(event.currentTarget.value)}
              placeholder="Paste the link code"
              autoComplete="one-time-code"
              autoCapitalize="off"
              autoCorrect="off"
              spellcheck={false}
              required
              disabled={submitting}
            />
            <small>The link expires after two minutes and works only once.</small>
          </label>

          {error && (
            <div class="form-error" role="alert">
              <strong>Browser linking did not complete</strong>
              <span>{error}</span>
            </div>
          )}

          <button
            class="button button-primary button-full"
            type="submit"
            disabled={submitting || !token.trim()}
          >
            {submitting ? <span class="button-spinner" /> : null}
            {submitting ? 'Linking…' : 'Link this browser'}
          </button>
        </form>
      </main>
    </div>
  );
}
