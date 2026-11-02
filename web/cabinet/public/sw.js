const CACHE_NAME = 'cabinet-shell-v1'
const APP_SHELL = ['/cabinet/', '/cabinet/index.html', '/cabinet/favicon.svg']

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(APP_SHELL)).then(() => self.skipWaiting()),
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k))),
    ).then(() => self.clients.claim()),
  )
})

self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting()
  }
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (!url.pathname.startsWith('/cabinet/')) return
  if (url.pathname.startsWith('/cabinet/api/')) return

  // Always try network first for navigations, so users get fresh index/app shell.
  if (req.mode === 'navigate') {
    event.respondWith(
      fetch(req)
        .then((res) => {
          const copy = res.clone()
          caches.open(CACHE_NAME).then((cache) => cache.put('/cabinet/index.html', copy))
          return res
        })
        .catch(() => caches.match('/cabinet/index.html')),
    )
    return
  }

  event.respondWith(
    caches.match(req).then((cached) => {
      if (cached) return cached
      return fetch(req)
        .then((res) => {
          const copy = res.clone()
          caches.open(CACHE_NAME).then((cache) => cache.put(req, copy))
          return res
        })
        .catch(() => caches.match('/cabinet/index.html'))
    }),
  )
})

// Push от кабинета: колокольчик, когда вкладка закрыта. Текст и адрес
// присылает сервер (internal/cabinetnotify), здесь только показ.
self.addEventListener('push', (event) => {
  let data = {}
  try {
    data = event.data ? event.data.json() : {}
  } catch {
    data = { body: event.data ? event.data.text() : '' }
  }
  const title = data.title || 'VPN'
  // Открытым вкладкам — сигнал обновить колокольчик, не дожидаясь опроса.
  const refreshTabs = self.clients
    .matchAll({ type: 'window', includeUncontrolled: true })
    .then((list) => list.forEach((c) => c.postMessage({ type: 'cabinet-notification' })))
  event.waitUntil(Promise.all([refreshTabs,
    self.registration.showNotification(title, {
      body: data.body || '',
      icon: data.icon || '/cabinet/favicon.svg',
      badge: data.icon || '/cabinet/favicon.svg',
      tag: data.tag || undefined,
      // Важное не исчезает само: «перебои, переключитесь» должно дождаться человека.
      requireInteraction: Boolean(data.important),
      data: { url: data.url || '/cabinet/dashboard#notifications' },
    }),
  ]))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = new URL(event.notification.data?.url || '/cabinet/dashboard#notifications', self.location.origin)
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((list) => {
      // Открытая вкладка кабинета — переводим её, а не плодим новую.
      for (const client of list) {
        const url = new URL(client.url)
        if (url.origin === target.origin && url.pathname.startsWith('/cabinet/')) {
          return client.focus().then((c) => (c && 'navigate' in c ? c.navigate(target.href) : undefined))
        }
      }
      return self.clients.openWindow(target.href)
    }),
  )
})
