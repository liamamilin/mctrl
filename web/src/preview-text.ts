// tmux includes the empty rows beneath the cursor. Keep all content and
// internal blank lines, but don't let unused screen rows hide recent output.
export function previewContent(lines: readonly string[]): string[] {
  let end = lines.length;
  while (end > 0 && lines[end - 1]?.trim() === '') end -= 1;
  return lines.slice(0, end);
}
