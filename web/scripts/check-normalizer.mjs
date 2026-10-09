// Compile and test the same function imported by TerminalPage.
import { readFile } from 'node:fs/promises';
import ts from 'typescript';
const source = await readFile(new URL('../src/terminal-input.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } });
const { normalizeHalfWidth } = await import('data:text/javascript;base64,' + Buffer.from(outputText).toString('base64'));

const hex = (ch) =>
  'U+' + ch.codePointAt(0).toString(16).toUpperCase().padStart(4, '0');

// Observed on the phone, and the ASCII a program would have bound instead.
const converted = [
  ['，', ','],
  ['：', ':'],
  ['／', '/'],
  ['；', ';'],
  ['！', '!'],
  ['？', '?'],
  ['（', '('],
  ['）', ')'],
  ['－', '-'],
  ['～', '~'],
  ['　', ' '],
  ['。', '.'],
  ['、', ','],
  ['“', '"'],
  ['”', '"'],
  ['‘', "'"],
  ['’', "'"],
  ['１', '1'],
];

// Must survive untouched: real CJK text, CJK brackets, and the marks that are
// characters rather than width errors.
const untouched = [
  '我',
  '按',
  '了',
  '打不出来',
  '「',
  '」',
  '【',
  '】',
  '《',
  '》',
  '—',
  '…',
  '·',
  '〃',
  '々',
  '➍',
  '➐',
  'a',
  '1',
  ',',
  '.',
  '\r',
  '\x1b[A',
];

let failures = 0;

for (const [input, want] of converted) {
  const got = normalizeHalfWidth(input);
  if (got !== want) {
    failures += 1;
    console.error(
      `FAIL ${hex(input)} ${JSON.stringify(input)} → ${JSON.stringify(got)}, want ${JSON.stringify(want)}`,
    );
  }
}

for (const input of untouched) {
  const got = normalizeHalfWidth(input);
  if (got !== input) {
    failures += 1;
    console.error(
      `FAIL ${hex(input)} ${JSON.stringify(input)} was changed to ${JSON.stringify(got)}`,
    );
  }
}

// A realistic mixed string, the way a paste or a fast typist arrives.
const mixed = '路径／tmp／，符号：。结束';
const wantMixed = '路径/tmp/,符号:.结束';
if (normalizeHalfWidth(mixed) !== wantMixed) {
  failures += 1;
  console.error(
    `FAIL mixed: ${JSON.stringify(normalizeHalfWidth(mixed))}, want ${JSON.stringify(wantMixed)}`,
  );
}

if (failures > 0) {
  console.error(`\n${failures} failure(s)`);
  process.exit(1);
}
console.log(
  `normalizeHalfWidth: ${converted.length} conversions and ${untouched.length} untouched cases pass`,
);
