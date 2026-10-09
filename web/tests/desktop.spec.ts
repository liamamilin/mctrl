import { test, expect } from './fixtures';

test('desktop guidance provides one directory command and Session return copies its exact connection', async ({ pairedPage: page, runtime }) => {
  await page.getByText('Continue work from your Mac', { exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Mac start command' })).toHaveValue('mctrl here');
  const name = "desktop's 项目";
  runtime.tmux('new-session', '-d', '-s', name, '-c', runtime.root, '/bin/zsh', '-f', '-i');
  const session = runtime.tmux('display-message', '-p', '-t', name, '#{session_id}').trim();
  const pid = runtime.tmux('display-message', '-p', '-t', name, '#{pane_pid}').trim();
  await page.getByRole('button', { name: 'Refresh sessions' }).click();
  await page.getByRole('heading', { name, exact: true }).click();
  await page.getByText('Continue on your Mac', { exact: true }).click();
  const command = page.getByRole('textbox', { name: 'Mac resume command' });
  const value = `mctrl resume --socket '${runtime.root}/s' --name 'desktop'\\''s 项目' '${session}'`;
  await expect(command).toHaveValue(value);
  let mutations = 0;
  page.on('request', request => { if (request.method() !== 'GET') mutations += 1; });
  await page.getByRole('button', { name: 'Copy mac resume command' }).click();
  await expect(command).toHaveValue(value);
  expect(mutations).toBe(0);
  expect(runtime.tmux('display-message', '-p', '-t', name, '#{pane_pid}').trim()).toBe(pid);
  expect(await (await page.request.get(runtime.url + '/api/v1/work')).json()).toHaveLength(0);
});

test('continuing an external desktop Session allows phone input without creating another Work', async ({ pairedPage: page, runtime }) => {
  runtime.tmux('new-session', '-d', '-s', 'desktop-input', '-c', runtime.root, '/bin/zsh', '-f', '-i');
  const session = runtime.tmux('display-message', '-p', '-t', 'desktop-input', '#{session_id}').trim();
  const pid = runtime.tmux('display-message', '-p', '-t', 'desktop-input', '#{pane_pid}').trim();
  await page.getByRole('button', { name: 'Refresh sessions' }).click();
  await page.getByRole('heading', { name: 'desktop-input', exact: true }).click();
  await page.getByRole('link', { name: 'Open Terminal' }).click();
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  await page.getByRole('textbox', { name: 'Terminal text' }).fill("printf 'PHONE_CONTINUED_DESKTOP\\n'");
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  await page.getByRole('button', { name: 'Send Return', exact: true }).click();
  await expect.poll(() => runtime.tmux('capture-pane', '-p', '-t', session).split('\n').filter(line => line.trim() === 'PHONE_CONTINUED_DESKTOP').length).toBe(1);
  await page.getByRole('link', { name: 'Back to Session' }).click();
  await page.getByText('Continue on your Mac', { exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Mac resume command' })).toHaveValue(new RegExp(session.replace('$', '\\$')));
  expect(runtime.tmux('display-message', '-p', '-t', session, '#{pane_pid}').trim()).toBe(pid);
  expect(await (await page.request.get(runtime.url + '/api/v1/work')).json()).toHaveLength(0);
});
