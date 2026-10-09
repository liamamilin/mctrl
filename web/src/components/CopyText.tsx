import { useEffect, useRef, useState } from 'preact/hooks';

// The native field is also the fallback on Trusted-LAN HTTP, where Clipboard
// API access may be unavailable. Nothing copied here is sent to the terminal.
export function CopyText({
  label, value, multiline = false, onUse, onUseDirectory,
}: {
  label: string;
  value: string;
  multiline?: boolean;
  onUse?: (text: string) => void;
  onUseDirectory?: (text: string) => void;
}) {
  const field = useRef<HTMLTextAreaElement>(null);
  const range = useRef({ start: 0, end: 0 });
  const readingHistory = useRef(false);
  const [snapshot, setSnapshot] = useState(value);
  const [notice, setNotice] = useState('');
  const [hasSelection, setHasSelection] = useState(false);
  useEffect(() => {
    if (multiline && (range.current.end > range.current.start || readingHistory.current)) return;
    setSnapshot(value);
    setNotice('');
    setHasSelection(false);
    range.current = { start: 0, end: 0 };
  }, [value, multiline]);
  useEffect(() => {
    if (multiline && field.current) field.current.scrollTop = field.current.scrollHeight;
  }, [snapshot, multiline]);

  const syncSelection = () => {
    const element = field.current;
    if (!element) return;
    range.current = { start: element.selectionStart, end: element.selectionEnd };
    setHasSelection(range.current.end > range.current.start);
  };
  const refreshText = () => {
    range.current = { start: 0, end: 0 };
    readingHistory.current = false;
    field.current?.setSelectionRange(0, 0);
    setHasSelection(false);
    setNotice('');
    setSnapshot(value);
  };

  const selectedText = () => {
    return snapshot.slice(range.current.start, range.current.end);
  };
  const copy = async (selection = false) => {
    const element = field.current;
    if (!element || !snapshot) return;
    const text = selection ? selectedText() : snapshot;
    if (!text) return;
    setNotice('');
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        setNotice('Copied.');
        return;
      }
    } catch { /* Native selection remains available when permission is refused. */ }
    element.focus({ preventScroll: true });
    if (selection) element.setSelectionRange(range.current.start, range.current.end);
    else element.select();
    syncSelection();
    try {
      if (document.execCommand('copy')) { setNotice('Copied.'); return; }
    } catch { /* Use the phone's native text-selection menu. */ }
    setNotice('Text selected. Touch and hold the selection, then choose Copy.');
    setHasSelection(true);
  };

  return (
    <div class={`copy-text${multiline ? ' copy-text-output' : ''}${onUse ? ' copy-text-with-draft' : ''}`}>
      <label>
        <span class="copy-text-label">{label}</span>
        <textarea
          ref={field}
          aria-label={label}
          value={snapshot}
          rows={multiline ? 8 : 1}
          wrap={multiline ? 'soft' : 'off'}
          readOnly
          spellcheck={false}
          onSelect={syncSelection}
          onScroll={(event) => {
            const element = event.currentTarget;
            readingHistory.current = multiline && element.scrollHeight - element.clientHeight - element.scrollTop > 5;
          }}
        />
      </label>
      <div class="copy-text-actions">
        <button class="button button-secondary button-small" type="button" aria-label={multiline ? 'Copy all output' : `Copy ${label.toLowerCase()}`} disabled={!snapshot} onClick={() => void copy()}>
          {multiline ? 'Copy all output' : 'Copy'}
        </button>
        {multiline && <button class="button button-quiet button-small" type="button" disabled={!hasSelection} onClick={() => void copy(true)}>Copy selection</button>}
        {onUse && <button class="button button-quiet button-small" type="button" disabled={multiline ? !hasSelection : !snapshot} onClick={() => onUse(multiline ? selectedText() : snapshot)}>
          {multiline ? 'Use selection in draft' : 'Use in draft'}
        </button>}
        {onUseDirectory && <button class="button button-quiet button-small" type="button" disabled={!hasSelection} onClick={() => onUseDirectory(selectedText())}>Prepare cd command</button>}
      </div>
      {multiline && value !== snapshot && <div class="copy-text-update" role="status">
        <span>New output available. Your view is kept while reading or selecting.</span>
        <button class="button button-quiet button-small" type="button" onClick={refreshText}>Show latest output</button>
      </div>}
      {notice && <p class="copy-text-notice" role="status">{notice}</p>}
    </div>
  );
}
