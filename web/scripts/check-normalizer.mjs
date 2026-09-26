// Verifies the shipped normalizeHalfWidth against the characters that were
// actually observed arriving from the phone, and against the ones that must not
// be touched. Run: node scripts/check-normalizer.mjs
//
// The function is duplicated here rather than imported because the page is
// TypeScript and this repo has no test runner. If the two ever disagree, this
// file is the stale one.

function normalizeHalfWidth(data) {
  let result = '';
  for (const character of data) {
    const code = character.codePointAt(0) ?? 0;
    if (code >= 0xff01 && code <= 0xff5e) {
      result += String.fromCharCode(code - 0xfee0);
      continue;
    }
    if (code === 0x3000) {
      result += ' ';
      continue;
    }
    if (code === 0x3001) {
      result += ',';
      continue;
    }
    if (code === 0x3002) {
      result += '.';
      continue;
    }
    if (code === 0x2018 || code === 0x2019) {
      result += "'";
      continue;
    }
    if (code === 0x201c || code === 0x201d) {
      result += '"';
      continue;
    }
    result += character;
  }
  return result;
}

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
