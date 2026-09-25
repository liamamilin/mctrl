export function formatDate(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

export function displayValue(value: string | number | boolean | undefined): string {
  if (value === undefined) return '—';
  if (value === '') return '—';
  return String(value);
}

export function hostStatusLabel(status: string): string {
  if (status === 'remote_ready') return 'Remote ready';
  return status.charAt(0).toUpperCase() + status.slice(1);
}

export function stateLabel(value: string): string {
  return value
    .toLowerCase()
    .split('_')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

export function makeRequestId(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  return `mctrl-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
