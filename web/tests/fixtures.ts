import { test as base, expect, type Page } from '@playwright/test';
import { spawn, execFileSync, type ChildProcess } from 'node:child_process';
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import { createServer } from 'node:net';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const repo = fileURLToPath(new URL('../../', import.meta.url));
const binary = path.join(repo, 'bin/mctrl');

export class Runtime {
  root = '';
  url = '';
  env: NodeJS.ProcessEnv = {};
  daemon?: ChildProcess;
  daemonExit?: Promise<void>;

  cli(...args: string[]): string {
    return execFileSync(binary, args, { env: this.env, encoding: 'utf8', timeout: 15_000 });
  }

  tmux(...args: string[]): string {
    return execFileSync('tmux', ['-S', path.join(this.root, 's'), ...args], { env: this.env, encoding: 'utf8', timeout: 5000, stdio: ['ignore', 'pipe', 'pipe'] });
  }

  async init() {
    this.root = await mkdtemp('/private/tmp/mctrl-browser-');
    // Login dotfiles belong to the operator, not the browser fixture. Use a
    // real interactive shell with deterministic startup and no personal rc.
    const shell = path.join(this.root, 'shell');
    await writeFile(shell, '#!/bin/sh\nexec /bin/zsh -f -i\n', { mode: 0o700 });
    this.env = { ...process.env, MCTRL_PROFILE: 'v1', MCTRL_HOME: this.root,
      MCTRL_TMUX_SOCKET: path.join(this.root, 's'),
      MCTRL_RUNNER_BINARY: path.join(repo, 'bin/mctrl-runner'), SHELL: shell, TMUX: '' };
    const socket = createServer();
    await new Promise<void>((resolve, reject) => { socket.once('error', reject); socket.listen(0, '127.0.0.1', resolve); });
    const port = (socket.address() as { port: number }).port;
    await new Promise<void>((resolve) => socket.close(() => resolve()));
    this.url = `http://127.0.0.1:${port}`;
    this.cli('setup', '--no-start', '--no-start-after-login', '--port', String(port), '--remote-availability', 'work_only');
    await this.start();
  }

  async start() {
    this.daemon = spawn(binary, ['serve'], { env: this.env, stdio: ['ignore', 'ignore', 'pipe'] });
    this.daemon.stderr?.resume();
    this.daemonExit = new Promise<void>((resolve, reject) => {
      this.daemon!.once('error', reject);
      this.daemon!.once('exit', () => resolve());
    });
    await expect.poll(async () => {
      try { return (await fetch(this.url + '/healthz')).status; } catch { return 0; }
    }).toBe(200);
  }

  async stop() {
    if (!this.daemon || this.daemon.exitCode !== null) return;
    this.daemon.kill('SIGTERM');
    await this.daemonExit;
  }

  async cleanup() {
    await this.stop();
    try { this.tmux('kill-server'); } catch { /* An empty server already stopped. */ }
    if (this.root) await rm(this.root, { recursive: true, force: true });
  }

  async registerProject() {
    const projectPath = path.join(this.root, 'project');
    await mkdir(projectPath, { recursive: true });
    this.cli('project', 'add', projectPath, '--name', 'Browser Project');
    const registry = JSON.parse(await readFile(path.join(this.root, 'projects.json'), 'utf8'));
    return registry.projects[0];
  }

  async writeWork(item: Record<string, unknown>) {
    await writeFile(path.join(this.root, 'work', `${item.id}.json`), JSON.stringify(item), { mode: 0o600 });
  }

  async work(id: string) {
    return JSON.parse(await readFile(path.join(this.root, 'work', `${id}.json`), 'utf8'));
  }
}

export const test = base.extend<{ runtime: Runtime; pairedPage: Page }>({
  runtime: async ({}, use) => {
    const runtime = new Runtime();
    try { await runtime.init(); await use(runtime); } finally { await runtime.cleanup(); }
  },
  pairedPage: async ({ page, runtime }, use) => {
    await pairPage(runtime, page);
    await use(page);
  },
});

export async function pairPage(runtime: Runtime, page: Page) {
  const token = runtime.cli('pair').match(/^Pairing token:\s+(\S+)/m)?.[1];
  if (!token) throw new Error('Fixture could not open a Pair window');
  await page.goto(runtime.url + '/#token=' + encodeURIComponent(token));
  await page.getByRole('button', { name: 'Pair this device', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
}

export { expect };
