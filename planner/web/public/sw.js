// 서비스 워커 — 푸시만 한다. 캐시를 붙이면 배포해도 옛 화면이 남는다.

self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()))

self.addEventListener('push', (e) => {
  let d = {}
  try { d = e.data ? e.data.json() : {} } catch { d = { title: '알림', body: e.data ? e.data.text() : '' } }
  e.waitUntil(self.registration.showNotification(d.title || '알림', {
    body: d.body || '',
    tag: d.tag || undefined,       // 같은 종류는 쌓지 않고 바꿔 끼운다
    renotify: !!d.tag,
    icon: '/icon-192.png',
    badge: '/icon-192.png',
    data: { url: d.url || '/' },
  }))
})

self.addEventListener('notificationclick', (e) => {
  e.notification.close()
  const url = (e.notification.data && e.notification.data.url) || '/'
  e.waitUntil((async () => {
    const all = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    for (const c of all) {
      if ('focus' in c) {
        await c.focus()
        if ('navigate' in c) await c.navigate(url)
        return
      }
    }
    await self.clients.openWindow(url)
  })())
})
