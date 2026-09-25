import { createHash } from 'node:crypto';
import { access, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

const args = process.argv.slice(2);
const verifyOnly = args.includes('--verify-only');
const rootArg = args.find((value) => !value.startsWith('--'));
const root = path.resolve(rootArg || 'dist');
const indexPath = path.join(root, 'index.html');
const serviceWorkerPath = path.join(root, 'sw.js');
const assetsRoot = path.join(root, 'assets');

await access(indexPath);
await access(serviceWorkerPath);

const index = await readFile(indexPath, 'utf8');
const references = [
  ...new Set(
    [...index.matchAll(/(?:src|href)="(\/assets\/[^"?]+)"/g)].map((match) => match[1]),
  ),
];

for (const reference of references) {
  const relative = reference.replace(/^\/+/, '');
  if (relative.startsWith('..') || path.isAbsolute(relative)) {
    throw new Error(`unsafe asset reference: ${reference}`);
  }
  await access(path.join(root, relative));
}

const assetNames = (await readdir(assetsRoot, { withFileTypes: true }))
  .filter((entry) => entry.isFile())
  .map((entry) => entry.name)
  .sort();

if (assetNames.length === 0) {
  throw new Error('dist contains no hashed assets');
}

const digest = createHash('sha256');
digest.update(index);
for (const name of assetNames) {
  digest.update(name);
  digest.update(await readFile(path.join(assetsRoot, name)));
}
const buildID = digest.digest('hex').slice(0, 16);
const expectedCache = `mctrl-shell-${buildID}`;
const serviceWorker = await readFile(serviceWorkerPath, 'utf8');
const cacheMatch = serviceWorker.match(/const CACHE = 'mctrl-shell-([^']+)'/);

if (verifyOnly) {
  if (!cacheMatch || cacheMatch[1] !== buildID) {
    throw new Error(
      `service worker cache id does not match dist (expected ${expectedCache})`,
    );
  }
} else {
  if (!/const CACHE = 'mctrl-shell-(?:__BUILD_ID__|[^']+)'/.test(serviceWorker)) {
    throw new Error('service worker does not contain a supported cache declaration');
  }
  await writeFile(
    serviceWorkerPath,
    serviceWorker.replace(
      /const CACHE = 'mctrl-shell-(?:__BUILD_ID__|[^']+)'/,
      `const CACHE = '${expectedCache}'`,
    ),
  );
}

console.log(
  `${verifyOnly ? 'verified' : 'finalized'} ${root}: ${references.length} referenced assets, cache ${expectedCache}`,
);
