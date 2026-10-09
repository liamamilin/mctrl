import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { getSession, getSessionPreview } from '../api';
import { CopyText } from './CopyText';
import { sessionDirectory } from '../session-directory';
import { previewContent } from '../preview-text';
import { formatDate } from '../format';

export function TerminalOutput({ sessionId, onClose, onUse, onDirectory }: {
  sessionId: string;
  onClose: () => void;
  onUse: (text: string) => void;
  onDirectory?: (text: string) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const restoreFocus = useRef(true);
  const [cwd, setCwd] = useState('');
  const [pathLabel, setPathLabel] = useState('Current path');
  const [output, setOutput] = useState('');
  const [capturedAt, setCapturedAt] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const loadId = useRef(0);
  const load = useCallback(async () => {
    const id = ++loadId.current;
    setLoading(true); setError('');
    const results = await Promise.allSettled([getSession(sessionId), getSessionPreview(sessionId, 500)]);
    if (id !== loadId.current) return;
    if (results[0].status === 'fulfilled') {
      const directory = sessionDirectory(results[0].value);
      setCwd(directory.path); setPathLabel(directory.label);
    } else setCwd('');
    if (results[1].status === 'fulfilled') {
      setOutput(previewContent(results[1].value.lines).join('\n'));
      setCapturedAt(results[1].value.captured_at);
    }
    else setError('Output could not be loaded. Try refreshing.');
    setLoading(false);
  }, [sessionId]);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialog.current?.showModal();
    void load();
    return () => { loadId.current += 1; if (restoreFocus.current) previous?.focus({ preventScroll: true }); };
  }, [load]);
  const useText = (text: string, directory = false) => {
    restoreFocus.current = false;
    // Release the native modal's inert state before focusing the draft in the
    // same user gesture; deferring that focus would lose the iOS keyboard.
    dialog.current?.close();
    if (directory) onDirectory?.(text);
    else onUse(text);
  };
  return (
    <dialog ref={dialog} class="terminal-output-dialog" aria-labelledby="terminal-output-title" onCancel={(event) => { event.preventDefault(); onClose(); }}>
      <div class="card-title-row">
        <h2 id="terminal-output-title">Copy output &amp; path</h2>
        <button class="button button-quiet button-small" type="button" onClick={onClose}>Done</button>
      </div>
      <p class="muted-label">Select text here to copy it or add it to your draft.</p>
      {cwd && <CopyText label={pathLabel} value={cwd} onUse={text => useText(text)} />}
      {error && <p class="form-error" role="alert">{error}</p>}
      {capturedAt && <p class="muted-label output-captured-at">Captured {formatDate(capturedAt)}</p>}
      {loading ? <p role="status">Loading output…</p> : <CopyText label="Recent output" value={output} multiline onUse={text => useText(text)} onUseDirectory={onDirectory ? text => useText(text, true) : undefined} />}
      <button class="button button-secondary button-small" type="button" disabled={loading} onClick={() => void load()}>Refresh output</button>
    </dialog>
  );
}
