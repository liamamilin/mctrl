import { chromium, webkit } from '@playwright/test';
import { test, expect, pairPage } from './fixtures';

test('lost Work requires confirmation, refuses a live child, and then unblocks its Project', async ({ pairedPage: page, runtime }) => {
  const project = await runtime.registerProject();
  const item = {
    id: 'work_lost_browser', attempt_id: 'attempt_browser', request_id: 'request_browser',
    project_id: project.id, runner_id: 'shell', state: 'RUNNING',
    session_name: 'missing-session', runner_pid: 9999999, child_pid: process.pid,
    created_at: new Date(Date.now() - 60_000).toISOString(),
    recovery_status: 'session_missing_after_recovery',
  };
  await runtime.writeWork(item);
  await page.goto(runtime.url + '/#/settings');
  await expect(page.getByText('1 active Work', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Review and archive' }).click();
  const dialog = page.getByRole('alertdialog', { name: 'Confirm archive lost Work' });
  await expect(dialog).toBeVisible();
  expect((await runtime.work(item.id)).archived_at).toBeUndefined();
  await dialog.getByRole('button', { name: 'Archive lost Work' }).click();
  await expect(dialog.getByRole('alert')).toContainText('still exist');
  await expect(dialog).toBeVisible();
  await runtime.writeWork({ ...item, child_pid: 9999998 });
  await dialog.getByRole('button', { name: 'Archive lost Work' }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText('1 active Work', { exact: true })).toHaveCount(0);
  const archived = await runtime.work(item.id);
  expect(archived.archived_at).toBeTruthy();
  expect(archived.state).toBe('RUNNING');
  expect(archived.exit_code).toBeUndefined();
  await page.getByRole('button', { name: 'Unregister', exact: true }).click();
  await page.getByRole('alertdialog', { name: 'Confirm unregister' }).getByRole('button', { name: 'Unregister Project' }).click();
  await expect(page.getByText('Unregistered “Browser Project”. Existing Sessions were not changed.', { exact: true })).toBeVisible();
});

test('empty Shell Work survives browser close and daemon restart with the same processes', async ({ browserName, runtime }) => {
  const browserType = browserName === 'webkit' ? webkit : chromium;
  let browser = await browserType.launch();
  try {
  const context = await browser.newContext({ viewport: { width: 393, height: 852 }, isMobile: true, hasTouch: true });
  const page = await context.newPage();
  await pairPage(runtime, page);
  await runtime.registerProject();
  await page.goto(runtime.url + '/#/start');
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  const response = await page.request.get(runtime.url + '/api/v1/work');
  const items = await response.json();
  const id = items[0].id;
  await expect.poll(async () => (await runtime.work(id)).state).toBe('RUNNING');
  const before = await runtime.work(id);
  const storageState = await context.storageState();
  await browser.close();
  process.kill(before.runner_pid, 0);
  process.kill(before.child_pid, 0);
  expect(runtime.tmux('has-session', '-t', before.session_id)).toBe('');
  await runtime.stop();
  process.kill(before.runner_pid, 0);
  process.kill(before.child_pid, 0);
  await runtime.start();
  const after = await runtime.work(id);
  expect(after.state).toBe('RUNNING');
  expect(after.runner_pid).toBe(before.runner_pid);
  expect(after.child_pid).toBe(before.child_pid);
  expect(after.session_id).toBe(before.session_id);
  browser = await browserType.launch();
  const resumed = await (await browser.newContext({ storageState })).newPage();
  await resumed.goto(runtime.url + '/#/');
  await expect(resumed.locator('.session-card').filter({ hasText: before.session_name })).toBeVisible();
  } finally {
    await browser.close();
  }
});

test('terminal reconnect never replays raw input and FULL keeps usable controls', async ({ pairedPage: page, context, runtime }) => {
  await runtime.registerProject();
  await page.goto(runtime.url + '/#/start');
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  const items = await (await page.request.get(runtime.url + '/api/v1/work')).json();
  const session = items[0].session_id;
  await page.goto(runtime.url + '/#/sessions/' + encodeURIComponent(session) + '/terminal');
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Show terminal keyboard for editing' }).click();
  await page.locator('.xterm-helper-textarea').pressSequentially("printf 'MCTRL_RAW_ONCE\\n'");
  await page.getByRole('button', { name: 'Send Return' }).click();
  const outputCount = () => runtime.tmux('capture-pane', '-p', '-t', session, '-S', '-500').split('\n').filter(line => line.trim() === 'MCTRL_RAW_ONCE').length;
  await expect.poll(outputCount).toBe(1);
  await context.setOffline(true);
  await expect(page.getByText('Mac is currently unreachable.', { exact: true })).toBeVisible();
  await context.setOffline(false);
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  expect(outputCount()).toBe(1);
  await page.getByRole('button', { name: 'FULL', exact: true }).click();
  await expect(page.locator('.overview-hint')).toContainText('Full');
  await expect(page.getByRole('textbox', { name: 'Terminal text' })).toHaveCount(0);
  for (const name of ['Arrow up', 'Arrow down', 'Arrow left', 'Arrow right', 'Send Return', 'Page Up', 'Page Down']) {
    await expect(page.getByRole('button', { name, exact: true })).toBeEnabled();
  }
  await page.getByRole('button', { name: 'LOCAL', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Terminal text' })).toBeVisible();
});

test('an uncertain structured Prompt is retained across reconnect without resending', async ({ pairedPage: page, runtime }) => {
  await runtime.registerProject();
  await page.goto(runtime.url + '/#/start');
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  const items = await (await page.request.get(runtime.url + '/api/v1/work')).json();
  await page.goto(runtime.url + '/#/sessions/' + encodeURIComponent(items[0].session_id) + '/terminal');
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  let sends = 0;
  await page.route('**/api/v1/sessions/*/prompt', route => { sends += 1; return route.abort('failed'); });
  await page.getByRole('textbox', { name: 'Terminal text' }).fill('uncertain probe');
  await page.getByRole('button', { name: 'Send to shell + Return', exact: true }).click();
  await expect(page.getByText(/Prompt delivery is uncertain/)).toBeVisible();
  await page.reload();
  await expect(page.getByRole('button', { name: 'Clear prompt', exact: true })).toBeVisible();
  expect(sends).toBe(1);
  await expect(page.getByRole('button', { name: 'Send to shell + Return', exact: true })).toBeDisabled();
});
