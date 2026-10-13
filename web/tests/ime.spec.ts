import { test, expect, type Runtime } from './fixtures';
import type { Page } from '@playwright/test';

async function openShell(page: Page, runtime: Runtime) {
  runtime.tmux('new-session', '-d', '-s', 'ime-shell', '/bin/zsh', '-f', '-i');
  await page.goto(runtime.url + '/#/sessions/ime-shell/terminal');
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
}

test('Edit uses native composition and inserts Chinese, numbers and symbols once after confirmation', async ({ pairedPage: page, runtime }) => {
  const frames: string[] = [];
  let promptPosts = 0;
  page.on('websocket', socket => socket.on('framesent', frame => {
    if (typeof frame.payload !== 'string') frames.push(frame.payload.toString('utf8'));
  }));
  page.on('request', request => {
    if (request.method() === 'POST' && request.url().endsWith('/prompt')) promptPosts += 1;
  });
  await openShell(page, runtime);
  await page.getByRole('button', { name: 'Edit terminal text', exact: true }).click();
  const draft = page.getByRole('textbox', { name: 'Terminal text', exact: true });
  await expect(draft).toBeFocused();
  await expect(page.locator('.xterm-helper-textarea')).not.toBeFocused();
  await page.getByRole('button', { name: 'CTRL', exact: true }).click();
  await draft.fill("printf '%s\\n' '");
  await expect(page.getByRole('button', { name: 'Insert', exact: true })).toBeEnabled();
  await draft.evaluate((element: HTMLTextAreaElement) => {
    element.focus();
    element.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true, data: '' }));
    element.value = "printf '%s\\n' 'zhong";
    element.dispatchEvent(new InputEvent('input', { bubbles: true, composed: true, inputType: 'insertCompositionText', isComposing: true, data: 'zhong' }));
    // A tap arriving before the next render must not send an unfinished candidate.
    document.querySelector<HTMLButtonElement>('.prompt-send')!.click();
    document.querySelector<HTMLButtonElement>('.draft-send-return')!.click();
  });
  await expect(page.getByRole('button', { name: 'Insert', exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Send Return', exact: true })).toBeDisabled();
  await expect(draft).toBeEnabled();
  expect(promptPosts).toBe(0);
  expect(frames.join('')).not.toContain('printf');

  const text = '中文123.,!?@# １２３，。';
  const command = `printf '%s\\n' '${text}'`;
  await draft.evaluate((element: HTMLTextAreaElement, value: string) => {
    element.value = value;
    element.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true, data: '中文' }));
    // Keyboard/input event metadata can vary. The editor reads the actual value.
    element.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Unidentified', keyCode: 0 }));
    element.dispatchEvent(new InputEvent('input', { bubbles: true, composed: true, inputType: 'insertText', isComposing: false, data: null }));
    element.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true, key: 'Unidentified', keyCode: 0 }));
  }, command);
  await expect(draft).toHaveValue(command);
  await expect(page.getByRole('button', { name: 'Insert', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Insert', exact: true }).evaluate((button: HTMLButtonElement) => { button.click(); button.click(); });
  await expect(draft).toHaveValue('');
  expect(frames.join('').split(command).length - 1).toBe(1);
  expect(promptPosts).toBe(0);
  await expect(page.getByRole('button', { name: 'CTRL', exact: true })).toHaveAttribute('aria-pressed', 'false');
  const count = () => runtime.tmux('capture-pane', '-pJ', '-t', 'ime-shell', '-S', '-500').split('\n').filter(line => line.trim() === text).length;
  expect(count()).toBe(0);
  await page.getByRole('button', { name: 'Send Return', exact: true }).click();
  await expect.poll(count).toBe(1);
});

test('FULL opens the native editor in the same gesture and preserves its draft when closed', async ({ pairedPage: page, runtime }, testInfo) => {
  await openShell(page, runtime);
  await page.getByRole('button', { name: 'FULL', exact: true }).click();
  await expect(page.locator('.terminal-host')).toHaveClass(/overview/);
  const size = runtime.tmux('display-message', '-p', '-t', 'ime-shell', '#{pane_width}x#{pane_height}').trim();
  await expect(page.getByRole('textbox', { name: 'Terminal text', exact: true })).toHaveCount(0);
  // Observe focus before the click listener returns, rather than after a timer/effect.
  const edit = page.getByRole('button', { name: 'Edit terminal text', exact: true });
  await edit.evaluate((button: HTMLButtonElement) => {
    button.click();
    (window as any).editorFocusedInGesture = document.activeElement?.getAttribute('aria-label') === 'Terminal text';
  });
  expect(await page.evaluate(() => (window as any).editorFocusedInGesture)).toBe(true);
  const draft = page.getByRole('textbox', { name: 'Terminal text', exact: true });
  await expect(draft).toBeFocused();
  await draft.fill('中文数字123@.,');
  await expect(page.locator('.terminal-host')).toHaveClass(/overview/);
  expect(runtime.tmux('display-message', '-p', '-t', 'ime-shell', '#{pane_width}x#{pane_height}').trim()).toBe(size);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('full-native-editor.png') });
  await page.getByRole('button', { name: 'Close text editor', exact: true }).click();
  await expect(draft).toHaveCount(0);
  await expect(page.locator('.terminal-host')).toHaveClass(/overview/);
  await page.getByRole('button', { name: 'Edit terminal text', exact: true }).click();
  await expect(draft).toHaveValue('中文数字123@.,');
  await expect(draft).toBeFocused();
});

test('a Chinese draft survives disconnect and reconnect without being queued or replayed', async ({ pairedPage: page, context, runtime }) => {
  const frames: string[] = [];
  page.on('websocket', socket => socket.on('framesent', frame => {
    if (typeof frame.payload !== 'string') frames.push(frame.payload.toString('utf8'));
  }));
  await openShell(page, runtime);
  await page.getByRole('button', { name: 'Edit terminal text', exact: true }).click();
  const draft = page.getByRole('textbox', { name: 'Terminal text', exact: true });
  const text = "printf '%s\\n' '中文77@.,未发送'";
  await draft.fill(text);
  await context.setOffline(true);
  await expect(page.getByText('Mac is currently unreachable.', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Insert', exact: true })).toBeDisabled();
  await expect(draft).toHaveValue(text);
  await context.setOffline(false);
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  await expect(draft).toHaveValue(text);
  expect(frames.join('')).not.toContain(text);
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  await expect(draft).toHaveValue('');
  expect(frames.join('').split(text).length - 1).toBe(1);
});
