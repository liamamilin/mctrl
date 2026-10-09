import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { test, expect, type Runtime } from './fixtures';
import type { Page } from '@playwright/test';

async function openShell(page: Page, runtime: Runtime) {
  await runtime.registerProject();
  await page.goto(runtime.url + '/#/start');
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  const items = await (await page.request.get(runtime.url + '/api/v1/work')).json();
  await expect.poll(async () => (await runtime.work(items[0].id)).state).toBe('RUNNING');
  return items[0].session_id as string;
}

test('first directory registration returns to the selected directory and opens Shell there', async ({ pairedPage: page, runtime }) => {
  await page.goto(runtime.url + '/#/start');
  await page.getByRole('link', { name: 'Register a Project', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Projects', exact: true })).toBeInViewport();
  const directory = path.join(runtime.root, 'project with spaces');
  await mkdir(directory);
  await page.getByLabel('Project name', { exact: true }).fill('Phone project');
  await page.getByLabel('Mac path', { exact: true }).fill(directory);
  await page.getByRole('button', { name: 'Register Project', exact: true }).click();
  await page.getByRole('link', { name: 'Start in this directory' }).click();
  await expect(page.getByRole('combobox', { name: 'Project', exact: true })).toHaveText('Phone project');
  await expect(page.getByRole('textbox', { name: 'Starting path' })).toHaveValue(directory);
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.locator('.terminal-title strong')).toHaveText('Phone project');
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Back to Session' }).click();
  await expect(page.getByRole('heading', { name: 'Phone project', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Refresh Session' }).click();
  await expect(page.getByRole('textbox', { name: 'Current path' })).toHaveValue(directory);
  await expect(page.getByRole('heading', { name: 'Recent output', exact: true })).toBeInViewport();
  await expect(page.getByRole('button', { name: 'Close Session', exact: true })).not.toBeVisible();
  const items = await (await page.request.get(runtime.url + '/api/v1/work')).json();
  expect(items).toHaveLength(1);
  expect(runtime.tmux('display-message', '-p', '-t', items[0].session_id, '#{pane_current_path}').trim()).toBe(directory);
});

test('draft insertion waits for Return, preserves text, and selected output returns to the draft', async ({ pairedPage: page, runtime }) => {
  const session = await openShell(page, runtime);
  const draft = page.getByRole('textbox', { name: 'Terminal text' });
  const command = "printf '%s\\n' 'MCTRL_DRAFT_ONCE 中文 “原样”'";
  const outputCount = () => runtime.tmux('capture-pane', '-p', '-t', session, '-S', '-500').split('\n').filter(line => line.trim() === 'MCTRL_DRAFT_ONCE 中文 “原样”').length;
  await draft.fill(command);
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  await expect(draft).toHaveValue('');
  expect(outputCount()).toBe(0);
  await page.getByRole('button', { name: 'Send Return', exact: true }).click();
  await expect.poll(outputCount).toBe(1);
  await page.getByRole('button', { name: 'Copy output and current path' }).click();
  const dialog = page.getByRole('dialog', { name: 'Copy output & path' });
  const output = dialog.getByRole('textbox', { name: 'Recent output' });
  await expect(output).toHaveValue(/MCTRL_DRAFT_ONCE/);
  await output.evaluate((element: HTMLTextAreaElement) => {
    const start = element.value.indexOf('MCTRL_DRAFT_ONCE');
    element.focus(); element.setSelectionRange(start, start + 'MCTRL_DRAFT_ONCE'.length);
    element.dispatchEvent(new Event('select', { bubbles: true }));
  });
  await dialog.getByRole('button', { name: 'Use selection in draft' }).click();
  await expect(dialog).toHaveCount(0);
  await expect(draft).toHaveValue('MCTRL_DRAFT_ONCE');
  expect(outputCount()).toBe(1);
});

test('terminal input stays closed until launch facts are ready and early keys are never queued', async ({ pairedPage: page, runtime }) => {
  const session = await openShell(page, runtime);
  let starting = true;
  await page.route('**/api/v1/sessions/*', async route => {
    if (!starting) return route.continue();
    const response = await route.fetch();
    const facts = await response.json();
    facts.managed_work.state = 'STARTING';
    facts.managed_work.prompt_delivery = 'pending';
    return route.fulfill({ response, json: facts });
  });
  await page.reload();
  await expect(page.locator('.connection-pill')).toHaveText('Starting');
  await expect(page.getByRole('button', { name: 'Insert', exact: true })).toBeDisabled();
  await page.locator('.xterm-helper-textarea').pressSequentially("printf 'MCTRL_EARLY_INPUT\\n'\r");
  starting = false;
  await expect(page.locator('.connection-pill')).toHaveText('Live');
  expect(runtime.tmux('capture-pane', '-p', '-t', session)).not.toContain('MCTRL_EARLY_INPUT');
  await page.getByRole('textbox', { name: 'Terminal text' }).fill("printf 'MCTRL_READY_INPUT\\n'");
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  await page.getByRole('button', { name: 'Send Return', exact: true }).click();
  await expect.poll(() => runtime.tmux('capture-pane', '-p', '-t', session).split('\n').filter(line => line.trim() === 'MCTRL_READY_INPUT').length).toBe(1);
});

test('copy has a native selection fallback without Clipboard API or execCommand', async ({ pairedPage: page, runtime }) => {
  await openShell(page, runtime);
  await page.evaluate(() => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
    document.execCommand = () => false;
  });
  await page.getByRole('button', { name: 'Copy output and current path' }).click();
  const dialog = page.getByRole('dialog', { name: 'Copy output & path' });
  const currentPath = dialog.getByRole('textbox', { name: 'Current path' });
  await expect(currentPath).not.toHaveValue('');
  await dialog.getByRole('button', { name: 'Copy current path', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('Touch and hold');
  expect(await currentPath.evaluate((element: HTMLTextAreaElement) => element.value.slice(element.selectionStart, element.selectionEnd))).toBe(await currentPath.inputValue());
  const directory = await currentPath.inputValue();
  await dialog.getByRole('button', { name: 'Use in draft' }).click();
  await expect(page.getByRole('textbox', { name: 'Terminal text' })).toHaveValue(directory);
});

test('a directory selected from output prepares a quoted cd draft and changes directory only after Return', async ({ pairedPage: page, runtime }) => {
  const session = await openShell(page, runtime);
  const name = "目录 'with' spaces";
  const target = path.join(runtime.root, 'project', name);
  await mkdir(target);
  runtime.tmux('send-keys', '-t', session, '-l', '--', `printf '%s\\n' "${name}"`);
  runtime.tmux('send-keys', '-t', session, 'Enter');
  await expect.poll(() => runtime.tmux('capture-pane', '-p', '-t', session)).toContain(name);
  await page.getByRole('button', { name: 'Copy output and current path' }).click();
  const dialog = page.getByRole('dialog', { name: 'Copy output & path' });
  const output = dialog.getByRole('textbox', { name: 'Recent output' });
  await expect(output).toHaveValue(new RegExp(name));
  await output.evaluate((element: HTMLTextAreaElement, text: string) => {
    const start = element.value.indexOf(text); element.focus(); element.setSelectionRange(start, start + text.length);
    element.dispatchEvent(new Event('select', { bubbles: true }));
  }, name);
  await dialog.getByRole('button', { name: 'Prepare cd command' }).click();
  const draft = page.getByRole('textbox', { name: 'Terminal text' });
  await expect(draft).toHaveValue("cd -- '目录 '\\''with'\\'' spaces'");
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  expect((await (await page.request.get(runtime.url + '/api/v1/sessions/' + encodeURIComponent(session))).json()).managed_work.child_cwd).toBe(path.dirname(target));
  await page.getByRole('button', { name: 'Send Return', exact: true }).click();
  await expect.poll(async () => (await (await page.request.get(runtime.url + '/api/v1/sessions/' + encodeURIComponent(session))).json()).managed_work.child_cwd).toBe(target);
  runtime.tmux('send-keys', '-t', session, '-l', '--', "printf '%s\\n' \"$PWD\"");
  runtime.tmux('send-keys', '-t', session, 'Enter');
  await expect.poll(() => runtime.tmux('capture-pane', '-pJ', '-t', session, '-S', '-500')).toContain(target);
});

test('multiline paste is guarded and bracketed paste sends one literal payload without Return or replay', async ({ pairedPage: page, runtime, context }) => {
  const script = path.join(runtime.root, 'harness.cjs');
  const received = path.join(runtime.root, 'received');
  await writeFile(script, `const fs = require('fs'); process.stdin.setRawMode(true); process.stdout.write('harness ready'); setInterval(() => process.stdout.write('still running'), 100); process.stdin.on('data', data => fs.appendFileSync(${JSON.stringify(received)}, data));`);
  runtime.tmux('new-session', '-d', '-s', 'harness', process.execPath, script);
  await page.goto(runtime.url + '/#/sessions/' + encodeURIComponent('harness') + '/terminal');
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  const draft = page.getByRole('textbox', { name: 'Terminal text' });
  const text = '第一行 “保持原样”\nsecond line';
  await draft.fill(text);
  await expect(page.getByRole('button', { name: 'Insert', exact: true })).toBeDisabled();
  await expect(page.getByRole('status')).toContainText('multiline paste safely');
  await expect(draft).toHaveValue(text);
  await expect.poll(() => runtime.tmux('capture-pane', '-pJ', '-t', 'harness')).toContain('still running'.repeat(10));
  await expect(page.getByRole('button', { name: 'Insert', exact: true })).toBeDisabled();
  expect(await readFile(received, 'utf8').catch(() => '')).toBe('');
  // A test harness advertises the same bracketed-paste capability as a TUI.
  runtime.tmux('send-keys', '-t', 'harness', 'C-c');
  runtime.tmux('kill-session', '-t', 'harness');
  await writeFile(script, `const fs = require('fs'); process.stdin.setRawMode(true); process.stdout.write('harness ready'); setInterval(() => process.stdout.write('\\x1b[?2004h'), 100); process.stdin.on('data', data => fs.appendFileSync(${JSON.stringify(received)}, data));`);
  await writeFile(received, '');
  runtime.tmux('new-session', '-d', '-s', 'bracketed-harness', process.execPath, script);
  await page.goto(runtime.url + '/#/sessions/' + encodeURIComponent('bracketed-harness') + '/terminal');
  await expect(page.locator('.terminal-title strong')).toHaveText('bracketed-harness');
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  await draft.fill(text);
  await expect(page.getByRole('button', { name: 'Insert', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  await expect(draft).toHaveValue('');
  await expect.poll(async () => readFile(received, 'utf8')).toBe('\x1b[200~第一行 “保持原样”\rsecond line\x1b[201~');
  const before = await readFile(received, 'utf8');
  await context.setOffline(true);
  await expect(page.getByText('Mac is currently unreachable.', { exact: true })).toBeVisible();
  await context.setOffline(false);
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  expect(await readFile(received, 'utf8')).toBe(before);
  await page.getByRole('button', { name: 'Send Return', exact: true }).click();
  await expect.poll(async () => readFile(received, 'utf8')).toBe(before + '\r');
});

test('desktop tmux work is resumed on the phone without registering or launching another Work', async ({ pairedPage: page, runtime }) => {
  runtime.tmux('new-session', '-d', '-s', 'desktop-project', '-c', runtime.root, '/bin/zsh', '-f', '-i');
  await page.goto(runtime.url + '/#/');
  await page.getByRole('heading', { name: 'desktop-project', exact: true }).click();
  await page.getByRole('link', { name: 'Open Terminal' }).click();
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  const session = runtime.tmux('display-message', '-p', '-t', 'desktop-project', '#{session_id}').trim();
  await page.getByRole('link', { name: 'Back to Session' }).click();
  await page.getByRole('link', { name: 'Sessions', exact: true }).click();
  const resume = page.getByRole('link', { name: /Continue last session/ });
  await expect(resume).toHaveAttribute('href', '#/sessions/' + encodeURIComponent(session) + '/terminal');
  await resume.click();
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  expect(await (await page.request.get(runtime.url + '/api/v1/work')).json()).toHaveLength(0);
  runtime.tmux('set-option', '-t', session, 'remain-on-exit', 'on');
  runtime.tmux('send-keys', '-t', session, '-l', '--', 'exit');
  runtime.tmux('send-keys', '-t', session, 'Enter');
  await expect.poll(() => runtime.tmux('display-message', '-p', '-t', session, '#{pane_dead}').trim()).toBe('1');
  await page.goto(runtime.url + '/#/');
  await expect(page.getByRole('link', { name: /Review last session/ })).toHaveAttribute('href', '#/sessions/' + encodeURIComponent(session));
  await page.evaluate(() => {
    const key = Object.keys(localStorage).find(key => key.includes('mctrl-recent-session'))!;
    const value = JSON.parse(localStorage.getItem(key)!); value.name = 'old session'; localStorage.setItem(key, JSON.stringify(value));
  });
  await page.reload();
  await expect(page.getByRole('heading', { name: /^Sessions(?: \d+)?$/ })).toBeVisible();
  await expect(page.getByRole('link', { name: /(?:Continue|Review) last session/ })).toHaveCount(0);
});

test('harness launch modes preview the behavior and do not offer structured Shell send', async ({ pairedPage: page, runtime }) => {
  await page.route('**/api/v1/runners', route => route.fulfill({ json: ['shell', 'codex', 'opencode'].map(id => ({ id, name: id === 'codex' ? 'Codex' : id === 'opencode' ? 'OpenCode' : 'Shell', available: true })) }));
  await runtime.registerProject();
  await page.goto(runtime.url + '/#/start');
  const radio = page.locator('input[name="runner"][value="codex"]');
  await page.locator('.runner-option').filter({ has: radio }).click();
  await expect(page.getByRole('button', { name: 'Open & interact', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('textbox', { name: /^Prompt/ })).toHaveCount(0);
  await page.getByRole('button', { name: 'Run a task', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Start Work', exact: true })).toBeDisabled();
  await page.getByRole('textbox', { name: /^Prompt/ }).fill('A task to submit later');
  await expect(page.getByRole('button', { name: 'Start Work', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Open & interact', exact: true }).click();
  let body: Record<string, unknown> = {};
  await page.route('**/api/v1/work', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    body = route.request().postDataJSON();
    return route.fulfill({ json: { id: 'work_preview', state: 'ACCEPTED' } });
  });
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'The Mac accepted this Work' })).toBeVisible();
  expect(body.prompt).toBe('');
  runtime.tmux('new-session', '-d', '-s', 'agent-facts', '/bin/zsh', '-f', '-i');
  await runtime.writeWork({ id: 'work_agent_facts', attempt_id: 'attempt_agent', state: 'RUNNING', runner_id: 'codex', session_name: 'agent-facts', created_at: new Date().toISOString() });
  await page.goto(runtime.url + '/#/sessions/agent-facts/terminal');
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'Terminal text' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Send to shell + Return' })).toHaveCount(0);
});
