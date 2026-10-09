import { test, expect } from './fixtures';

test('Mac pairing keeps the stable address and refreshes its copyable IP fallback', async ({ page, runtime }, testInfo) => {
  const token = runtime.cli('pair').match(/^Pairing token:\s+(\S+)/m)?.[1];
  expect(token).toBeTruthy();
  const port = new URL(runtime.url).port;
  const primary = `http://My-Mac.local:${port}/pair`;
  let ip = '192.168.2.105';
  await page.route('**/api/v1/pair/qr', async route => {
    const response = await route.fetch();
    const payload = await response.json();
    await route.fulfill({ response, json: { ...payload, pair_url: primary, fallback_pair_url: `http://${ip}:${port}/pair` } });
  });
  await page.goto(runtime.url + '/#/pair-console?token=' + encodeURIComponent(token!));
  const preferred = page.getByRole('textbox', { name: 'mctrl manual pairing address', exact: true });
  const fallback = page.getByRole('textbox', { name: 'mctrl fallback pairing address', exact: true });
  await expect(preferred).toHaveValue(primary);
  await expect(page.getByText(/while the Mac’s local name stays the same/)).toBeVisible();
  await expect(fallback).not.toBeVisible();
  await page.getByText('If the phone address does not open', { exact: true }).click();
  await expect(fallback).toHaveValue(`http://${ip}:${port}/pair`);
  await page.evaluate(() => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
  });
  const copy = page.getByRole('button', { name: 'Copy backup address', exact: true });
  await copy.click();
  await expect(page.getByText('Backup address selected. Use the macOS Copy command.')).toBeVisible();
  expect(await fallback.evaluate((element: HTMLInputElement) => element.value.slice(element.selectionStart!, element.selectionEnd!))).toBe(await fallback.inputValue());
  ip = '192.168.2.226';
  await page.getByRole('button', { name: 'Refresh QR display', exact: true }).click();
  await expect(preferred).toHaveValue(primary);
  await page.getByText('If the phone address does not open', { exact: true }).click();
  await expect(fallback).toBeVisible();
  await expect(fallback).toHaveValue(`http://${ip}:${port}/pair`);
  expect((await copy.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect(await fallback.evaluate((element: HTMLInputElement) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('stable-pair-console.png'), fullPage: true });
});

test('loopback pairing retains its address without an unrelated LAN fallback', async ({ page, runtime }) => {
  const token = runtime.cli('pair').match(/^Pairing token:\s+(\S+)/m)?.[1];
  expect(token).toBeTruthy();
  await page.goto(runtime.url + '/#/pair-console?token=' + encodeURIComponent(token!));
  await expect(page.getByRole('textbox', { name: 'mctrl manual pairing address', exact: true })).toHaveValue(runtime.url + '/pair');
  await expect(page.getByText('If the phone address does not open', { exact: true })).toHaveCount(0);
  await expect(page.getByText(/while the Mac’s local name stays the same/)).toHaveCount(0);
  await expect(page.getByRole('img', { name: 'One-time mctrl phone pairing QR code', exact: true })).toBeVisible();
  expect(await page.getByRole('textbox', { name: 'mctrl pairing code', exact: true }).inputValue()).toBe(token);
});
