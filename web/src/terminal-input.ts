// Terminals want half-width ASCII, and a Chinese IME does not send it.
//
// Measured on the phone, with the daemon recording every frame the browser
// sends: a Chinese keyboard delivers letters and Return and delivers nothing at
// all for the digit row or for punctuation. Those keys never produce a DOM
// event, so no conversion here can help and none is attempted for them. The
// English keyboard sends `1` as 0x31 and `,` as 0x2C, which is why switching
// keyboards works.
//
// What this does fix is the text that does arrive in the wrong width. A Chinese
// keyboard with an English layout active, a paste, or a hardware keyboard
// produces real characters, and they are full-width or CJK punctuation. A
// program that binds `.` or `,` never sees `。` or `、`.
//
// Three blocks are covered, and only characters with one unambiguous ASCII
// equivalent are converted:
//
//   U+FF01..U+FF5E  full-width forms        → ASCII 0x21..0x7E
//   U+3000..U+3003  ideographic punctuation → space , . (and 〃 untouched)
//   U+2018..U+201D  curly quotes            → ' and "
//
// Everything else is left exactly as typed: CJK text, CJK brackets such as
// 「」【】, the em dash and the ellipsis, and the circled digits a long press
// produces. Those are meaningful characters, not width errors.
//
// Set mctrl-terminal-half-width to "off" in localStorage to send raw bytes.
export function normalizeHalfWidth(data: string): string {
  let result = '';
  for (const character of data) {
    const code = character.codePointAt(0) ?? 0;
    if (code >= 0xff01 && code <= 0xff5e) {
      // U+FF01..U+FF5E map onto ASCII 0x21..0x7E.
      result += String.fromCharCode(code - 0xfee0);
      continue;
    }
    if (code === 0x3000) {
      result += ' ';
      continue;
    }
    if (code === 0x3001) {
      // 、 ideographic comma
      result += ',';
      continue;
    }
    if (code === 0x3002) {
      // 。 ideographic full stop. This is the character a Chinese keyboard
      // sends for the period key, and it sits below the full-width block, so
      // covering only U+FF01..U+FF5E let it through unchanged.
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
