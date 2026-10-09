import { test, expect } from './fixtures';

test('output replaces the draft selection, returns focus, and sends nothing until Insert', async ({ pairedPage: page, runtime }) => {
  const inputFrames: string[] = [];
  page.on('websocket', socket => socket.on('framesent', frame => {
    if (typeof frame.payload !== 'string') inputFrames.push(frame.payload.toString('utf8'));
  }));
  await runtime.registerProject();
  await page.goto(runtime.url + '/#/start');
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.getByText('Live', { exact: true })).toBeVisible();
  await expect(page.locator('.terminal-title strong')).toHaveText('Browser Project');
  const items = await (await page.request.get(runtime.url + '/api/v1/work')).json();
  const session = items[0].session_id;
  runtime.tmux('send-keys', '-t', session, '-l', '--', "printf '%s\\n' '中文 copied path'");
  runtime.tmux('send-keys', '-t', session, 'Enter');
  await expect.poll(() => runtime.tmux('capture-pane', '-pJ', '-t', session).split('\n').filter(line => line.trim() === '中文 copied path').length).toBe(1);
  const draft = page.getByRole('textbox', { name: 'Terminal text' });
  await draft.fill("printf '%s\\n' 'replace me' # keep this suffix");
  await draft.evaluate((element: HTMLTextAreaElement) => {
    const start = element.value.indexOf('replace me');
    element.setSelectionRange(start, start + 'replace me'.length);
    element.dispatchEvent(new Event('select', { bubbles: true }));
  });
  const before = runtime.tmux('capture-pane', '-pJ', '-t', session).trimEnd();
  await page.getByRole('button', { name: 'Copy output and current path' }).click();
  const dialog = page.getByRole('dialog', { name: 'Copy output & path' });
  const output = dialog.getByRole('textbox', { name: 'Recent output' });
  await expect(output).toHaveValue(/中文 copied path/);
  await output.evaluate((element: HTMLTextAreaElement) => {
    const start = element.value.indexOf('中文 copied path');
    element.focus(); element.setSelectionRange(start, start + '中文 copied path'.length);
    element.dispatchEvent(new Event('select', { bubbles: true }));
  });
  await dialog.getByRole('button', { name: 'Use selection in draft' }).click();
  await expect(draft).toHaveValue("printf '%s\\n' '中文 copied path' # keep this suffix");
  await expect(draft).toBeFocused();
  expect(await draft.evaluate((element: HTMLTextAreaElement) => element.selectionStart)).toBe("printf '%s\\n' '中文 copied path".length);
  // tmux can query terminal capabilities and focus; neither carries draft text.
  expect(inputFrames.join('')).not.toContain('replace me');
  expect(inputFrames.join('')).not.toContain('copied path');
  expect(runtime.tmux('capture-pane', '-pJ', '-t', session).trimEnd()).toBe(before);
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  await page.getByRole('button', { name: 'Send Return', exact: true }).click();
  await expect.poll(() => runtime.tmux('capture-pane', '-pJ', '-t', session).split('\n').filter(line => line.trim() === '中文 copied path').length).toBe(2);
});

test('live preview follows its tail, preserves selected text during updates, then shows the latest output', async ({ pairedPage: page, runtime }) => {
  runtime.tmux('new-session', '-d', '-s', 'preview', '/bin/zsh', '-f', '-i');
  const original = Array.from({ length: 50 }, (_, i) => `row ${i}  中文`);
  let lines = [...original, '', '   ', ''];
  await page.route('**/api/v1/sessions/*/preview?*', route => route.fulfill({ json: { lines, captured_at: new Date().toISOString() } }));
  await page.goto(runtime.url + '/#/sessions/preview');
  const output = page.getByRole('textbox', { name: 'Output text' });
  await expect(output).toHaveValue(original.join('\n'));
  await expect.poll(() => output.evaluate((element: HTMLTextAreaElement) => element.scrollHeight - element.clientHeight - element.scrollTop)).toBeLessThan(2);
  await output.evaluate((element: HTMLTextAreaElement) => {
    const start = element.value.indexOf('row 10');
    element.focus(); element.setSelectionRange(start, start + 'row 10  中文'.length);
    element.dispatchEvent(new Event('select', { bubbles: true }));
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async (text: string) => { (window as any).copiedText = text; } } });
  });
  lines = [...original, '', 'NEW_LAST_LINE', '', ''];
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await expect(page.getByRole('button', { name: 'Show latest output' })).toBeVisible();
  await expect(output).toHaveValue(original.join('\n'));
  await page.getByRole('button', { name: 'Copy selection', exact: true }).click();
  expect(await page.evaluate(() => (window as any).copiedText)).toBe('row 10  中文');
  await page.getByRole('button', { name: 'Show latest output' }).click();
  await expect(output).toHaveValue([...original, '', 'NEW_LAST_LINE'].join('\n'));
  await expect.poll(() => output.evaluate((element: HTMLTextAreaElement) => element.scrollHeight - element.clientHeight - element.scrollTop)).toBeLessThan(2);
});

test('returning to the foreground refreshes meaningful Home output without launching or sending input', async ({ pairedPage: page, runtime }) => {
  runtime.tmux('new-session', '-d', '-s', 'wake-preview', '/bin/zsh', '-f', '-i');
  runtime.tmux('send-keys', '-t', 'wake-preview', '-l', '--', "printf 'WAKE_BEFORE\\n'");
  runtime.tmux('send-keys', '-t', 'wake-preview', 'Enter');
  await expect.poll(() => runtime.tmux('capture-pane', '-p', '-t', 'wake-preview')).toContain('WAKE_BEFORE');
  await page.goto(runtime.url + '/#/');
  // Pairing already mounted Home before this external session was created.
  await page.getByRole('button', { name: 'Refresh sessions' }).click();
  const preview = page.locator('.session-card .session-preview');
  await expect(preview).toContainText('WAKE_BEFORE');
  let mutations = 0;
  page.on('request', request => { if (request.method() === 'POST' && /\/(?:work|prompt)(?:$|\?)/.test(request.url())) mutations += 1; });
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  runtime.tmux('send-keys', '-t', 'wake-preview', '-l', '--', "printf 'WAKE_AFTER\\n'");
  runtime.tmux('send-keys', '-t', 'wake-preview', 'Enter');
  await expect.poll(() => runtime.tmux('capture-pane', '-p', '-t', 'wake-preview')).toContain('WAKE_AFTER');
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(preview).toContainText('WAKE_AFTER', { timeout: 2000 });
  expect(mutations).toBe(0);
  expect(await (await page.request.get(runtime.url + '/api/v1/work')).json()).toHaveLength(0);
});

test('an accepted launch follows its original Work into the terminal and rapid taps create one POST', async ({ pairedPage: page, runtime }) => {
  await runtime.registerProject();
  let posts = 0;
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/v1/work', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    posts += 1;
    const response = await route.fetch();
    const item = await response.json();
    return route.fulfill({ response, json: { id: item.id, state: 'ACCEPTED' } });
  });
  await page.route('**/api/v1/work/*', async route => { reads += 1; await gate; return route.continue(); });
  try {
    await page.goto(runtime.url + '/#/start');
    await page.getByRole('button', { name: 'Start Work', exact: true }).evaluate((button: HTMLButtonElement) => { button.click(); button.click(); });
    await expect(page.getByRole('status')).toContainText('Your terminal will open automatically');
    expect(posts).toBe(1);
    release();
    await expect(page.getByText('Live', { exact: true })).toBeVisible();
    await expect(page).toHaveURL(/\/terminal$/);
    expect(reads).toBeGreaterThan(0);
    expect(posts).toBe(1);
    expect(await (await page.request.get(runtime.url + '/api/v1/work')).json()).toHaveLength(1);
  } finally { release(); }
});

test('leaving an accepted launch cancels its follow-up navigation when late Session evidence arrives', async ({ pairedPage: page, runtime }) => {
  await runtime.registerProject();
  let posts = 0;
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/v1/work', route => {
    if (route.request().method() !== 'POST') return route.continue();
    posts += 1;
    return route.fulfill({ json: { id: 'work_late_session', state: 'ACCEPTED' } });
  });
  await page.route('**/api/v1/work/work_late_session', async route => {
    await gate;
    return route.fulfill({ json: { id: 'work_late_session', state: 'RUNNING', session_id: '$98765' } });
  });
  try {
    await page.goto(runtime.url + '/#/start');
    await page.getByRole('button', { name: 'Start Work', exact: true }).click();
    await expect(page.getByRole('status')).toContainText('Waiting for the Session');
    await page.getByRole('link', { name: 'Back to Home' }).click();
    await expect(page.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
    const response = page.waitForResponse('**/api/v1/work/work_late_session');
    release(); await response;
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await expect(page).toHaveURL(runtime.url + '/#/');
    expect(posts).toBe(1);
  } finally { release(); }
});

test('leaving while the launch response is pending preserves the new page', async ({ pairedPage: page, runtime }) => {
  await runtime.registerProject();
  let posts = 0;
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/v1/work', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    posts += 1;
    await gate;
    return route.fulfill({ json: { id: 'work_late_response', state: 'RUNNING', session_id: '$98765' } });
  });
  try {
    await page.goto(runtime.url + '/#/start');
    await page.getByRole('button', { name: 'Start Work', exact: true }).click();
    await expect.poll(() => posts).toBe(1);
    await page.getByRole('link', { name: 'Home', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
    const response = page.waitForResponse(response => response.url() === runtime.url + '/api/v1/work' && response.request().method() === 'POST');
    release(); await response;
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await expect(page).toHaveURL(runtime.url + '/#/');
    expect(posts).toBe(1);
  } finally { release(); }
});

test('observed launch failure is shown without automatically retrying the Work', async ({ pairedPage: page, runtime }) => {
  await runtime.registerProject();
  let posts = 0;
  await page.route('**/api/v1/work', route => {
    if (route.request().method() !== 'POST') return route.continue();
    posts += 1;
    return route.fulfill({ json: { id: 'work_failed_launch', state: 'ACCEPTED' } });
  });
  await page.route('**/api/v1/work/work_failed_launch', route => route.fulfill({ json: { id: 'work_failed_launch', state: 'LAUNCH_FAILED', error_code: 'RUNNER_MISSING', error_message: 'The selected runner is unavailable.' } }));
  await page.goto(runtime.url + '/#/start');
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'This Work could not start' })).toBeVisible();
  await expect(page.getByRole('alert')).toContainText('selected runner is unavailable');
  await page.getByRole('button', { name: 'Review launch settings' }).click();
  await expect(page.getByRole('combobox', { name: 'Project', exact: true })).toHaveText('Browser Project');
  expect(posts).toBe(1);
});

test('Run a task opens output while interactive startup opens a terminal', async ({ pairedPage: page, runtime }) => {
  const project = await runtime.registerProject();
  runtime.tmux('new-session', '-d', '-s', 'task-output', '/bin/zsh', '-f', '-i');
  const session = runtime.tmux('display-message', '-p', '-t', 'task-output', '#{session_id}').trim();
  await runtime.writeWork({ id: 'work_task_route', project_id: project.id, runner_id: 'codex', state: 'EXITED', session_id: session, session_name: 'task-output', exit_code: 0, created_at: new Date().toISOString() });
  await page.route('**/api/v1/runners', route => route.fulfill({ json: [{ id: 'shell', name: 'Shell', available: true }, { id: 'codex', name: 'Codex', available: true }] }));
  let body: Record<string, unknown> = {};
  await page.route('**/api/v1/work', route => {
    if (route.request().method() !== 'POST') return route.continue();
    body = route.request().postDataJSON();
    return route.fulfill({ json: { id: 'work_task_route', state: 'EXITED', session_id: session } });
  });
  await page.goto(runtime.url + '/#/start');
  await page.locator('.runner-option').filter({ has: page.locator('input[name="runner"][value="codex"]') }).click();
  await page.getByRole('button', { name: 'Run a task', exact: true }).click();
  await page.getByRole('textbox', { name: /^Prompt/ }).fill('Read the task output later');
  await page.getByRole('button', { name: 'Start Work', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Recent output', exact: true })).toBeVisible();
  await expect(page).toHaveURL(runtime.url + '/#/sessions/' + encodeURIComponent(session));
  await expect(page.getByRole('textbox', { name: 'Terminal text' })).toHaveCount(0);
  expect(body.prompt).toBe('Read the task output later');
});
