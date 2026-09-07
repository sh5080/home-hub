import { api } from '../api'

/** 이 기기의 푸시 구독. iOS 는 홈 화면 앱(16.4+)에서, 사용자 탭 핸들러 안에서만 권한을 물을 수 있다. */
export type PushState = 'unsupported' | 'needs-install' | 'denied' | 'off' | 'on'

export function pushSupported() {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
}

function standalone() {
  return window.matchMedia('(display-mode: standalone)').matches ||
    (navigator as unknown as { standalone?: boolean }).standalone === true
}

export async function registerSW() {
  if (!('serviceWorker' in navigator)) return
  try { await navigator.serviceWorker.register('/sw.js') } catch { /* 없어도 앱은 돈다 */ }
}

export async function pushState(): Promise<PushState> {
  const ios = /iPhone|iPad|iPod/.test(navigator.userAgent)
  if (!pushSupported()) return ios && !standalone() ? 'needs-install' : 'unsupported'
  if (Notification.permission === 'denied') return 'denied'
  const reg = await navigator.serviceWorker.getRegistration()
  const sub = await reg?.pushManager.getSubscription()
  return sub ? 'on' : 'off'
}

function b64ToBytes(b64: string) {
  const pad = '='.repeat((4 - (b64.length % 4)) % 4)
  const raw = atob((b64 + pad).replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(raw, (c) => c.charCodeAt(0))
}

function deviceName() {
  const ua = navigator.userAgent
  if (/iPhone/.test(ua)) return 'iPhone'
  if (/iPad/.test(ua)) return 'iPad'
  if (/Android/.test(ua)) return 'Android'
  if (/Mac/.test(ua)) return 'Mac'
  return '브라우저'
}

/**
 * 켠다. fresh 면 브라우저의 기존 구독을 버리고 새로 받는다.
 * 크롬은 사이트 알림 권한을 껐다 켜면 구독을 폐기하는데 getSubscription() 은 옛 것을 계속 준다.
 */
export async function enablePush(fresh = false) {
  const perm = await Notification.requestPermission()
  if (perm !== 'granted') throw new Error('알림 권한을 허용해야 받을 수 있어요')
  await registerSW()
  const reg = await navigator.serviceWorker.ready
  const { public_key } = await api.get<{ public_key: string }>('/api/push/key')
  let sub = await reg.pushManager.getSubscription()
  if (sub && fresh) {
    await sub.unsubscribe()
    sub = null
  }
  if (!sub) {
    sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: b64ToBytes(public_key) })
  }
  const j = sub.toJSON()
  await api.post('/api/push/subscribe', { endpoint: j.endpoint, keys: j.keys, device: deviceName() })
}

/** 이 기기의 구독을 서버에 다시 알린다(권한은 묻지 않는다). */
export async function syncPush() {
  if (!pushSupported() || Notification.permission !== 'granted') return
  const reg = await navigator.serviceWorker.getRegistration()
  const sub = await reg?.pushManager.getSubscription()
  if (!sub) return
  const j = sub.toJSON()
  await api.post('/api/push/subscribe', { endpoint: j.endpoint, keys: j.keys, device: deviceName() })
}

/** 시험 알림. 서버가 '기기가 없다'면 구독이 죽은 것이라 새로 받아 다시 보낸다. */
export async function testPush() {
  try {
    await api.post('/api/push/test')
  } catch (e) {
    const msg = e instanceof Error ? e.message : ''
    if (!msg.includes('기기가 없어요')) throw e
    await enablePush(true)
    await api.post('/api/push/test')
  }
}

export async function disablePush() {
  const reg = await navigator.serviceWorker.getRegistration()
  const sub = await reg?.pushManager.getSubscription()
  if (!sub) return
  await api.post('/api/push/unsubscribe', { endpoint: sub.endpoint })
  await sub.unsubscribe()
}
