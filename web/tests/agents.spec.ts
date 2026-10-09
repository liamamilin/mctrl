import { execFileSync } from 'node:child_process';
import { test, expect } from './fixtures';

test.describe('installed interactive agents', () => {
  test.skip(process.env.MCTRL_LIVE_RUNNERS !== '1', 'Opt-in: opens the locally configured Agent CLIs');
  for (const id of ['codex', 'opencode']) {
    test(`an empty ${id} Prompt opens its interactive interface`, async ({ pairedPage: page, runtime }) => {
      await runtime.registerProject();
      await page.goto(runtime.url + '/#/start');
      const radio = page.locator(`input[name="runner"][value="${id}"]`);
      await page.locator('.runner-option').filter({ has: radio }).click();
      await expect(radio).toBeChecked();
      await page.getByRole('button', { name: 'Start Work', exact: true }).click();
      await expect(page.getByText('Live', { exact: true })).toBeVisible();
      const items = await (await page.request.get(runtime.url + '/api/v1/work')).json();
      const workId = items[0].id;
      await expect.poll(async () => (await runtime.work(workId)).state).toBe('RUNNING');
      const work = await runtime.work(workId);
      expect(work.prompt_delivery).toBe('none');
      const command = execFileSync('ps', ['-p', String(work.child_pid), '-o', 'command='], { encoding: 'utf8' });
      if (id === 'codex') {
        expect(command).toContain('-C');
        expect(command).not.toContain(' exec ');
      } else {
        expect(command).toContain('--standalone');
        expect(command).not.toContain(' run ');
      }
      await page.goto(runtime.url + '/#/sessions/' + encodeURIComponent(work.session_id) + '/terminal');
      await expect(page.getByText('Live', { exact: true })).toBeVisible();
      // A disposable Codex directory may first show its native trust screen.
      // That is an interactive UI; the probe does not approve the directory.
      const initialUI = id === 'codex' ? /codex|do you trust the contents/ : /opencode/;
      await expect.poll(() => runtime.tmux('capture-pane', '-p', '-t', work.session_id).toLowerCase()).toMatch(initialUI);
      process.kill(work.child_pid, 0);
    });
  }
});
