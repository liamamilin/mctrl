const CACHE = 'mctrl-shell-bc716837d99f1885';
const SHELL = ['/', '/index.html', '/manifest.webmanifest'];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE).then(async (cache) => {
      await cache.addAll(SHELL);
      try {
        const response = await fetch('/index.html', { cache: 'no-store' });
        const html = await response.clone().text();
        await cache.put('/index.html', response);
        const assets = [
          ...new Set(
            [...html.matchAll(/(?:src|href)="(\/assets\/[^"?]+)"/g)].map(
              (match) => match[1],
            ),
          ),
        ];
        await cache.addAll(assets);
      } catch {
        // The runtime fetch handler will fill assets on the first online load.
      }
      await self.skipWaiting();
    }),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(
          keys
            .filter(
              (key) =>
                key.startsWith('mctrl-shell-') && key !== CACHE,
            )
            .map((key) => caches.delete(key)),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  if (
    event.request.method !== 'GET' ||
    url.origin !== self.location.origin ||
    url.pathname.startsWith('/api/')
  ) {
    return;
  }

  if (url.pathname.startsWith('/assets/')) {
    event.respondWith(
      caches.match(event.request).then(
        (cached) =>
          cached ||
          fetch(event.request).then((response) => {
            const copy = response.clone();
            caches.open(CACHE).then((cache) => cache.put(event.request, copy));
            return response;
          }),
      ),
    );
    return;
  }

  event.respondWith(
    fetch(event.request)
      .then((response) => {
        if (response.ok) {
          const copy = response.clone();
          caches.open(CACHE).then((cache) => cache.put(event.request, copy));
        }
        return response;
      })
      .catch(() =>
        caches.match(event.request).then((cached) => cached || caches.match('/')),
      ),
  );
});
