const DESKTOP_TOKEN_FRAGMENT_KEY = 'desktop_token'

let authenticationRequired = false
const subscribers = new Set()

export function desktopAuthenticationRequired() {
  return authenticationRequired
}

export function subscribeToDesktopAuthentication(listener) {
  subscribers.add(listener)
  return () => subscribers.delete(listener)
}

export function requireDesktopAuthentication() {
  if (authenticationRequired) return
  authenticationRequired = true
  subscribers.forEach((listener) => listener())
}

export function clearDesktopAuthenticationRequirement() {
  if (!authenticationRequired) return
  authenticationRequired = false
  subscribers.forEach((listener) => listener())
}

export async function bootstrapDesktopSession(options = {}) {
  const location = options.location || globalThis.location
  const history = options.history || globalThis.history
  const fetchImpl = options.fetchImpl || globalThis.fetch
  const fragment = location?.hash?.startsWith('#') ? location.hash.slice(1) : ''
  const params = new URLSearchParams(fragment)

  if (!params.has(DESKTOP_TOKEN_FRAGMENT_KEY)) {
    return { ok: true, attempted: false }
  }

  const token = params.get(DESKTOP_TOKEN_FRAGMENT_KEY) || ''
  history.replaceState(history.state ?? null, '', `${location.pathname || '/'}${location.search || ''}`)
  if (!token) {
    return { ok: false, attempted: true }
  }

  try {
    const response = await fetchImpl('/api/v1/desktop-sessions', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ token }),
    })
    const payload = await response.json()
    if (!response.ok || payload?.success !== true) {
      return { ok: false, attempted: true }
    }
    clearDesktopAuthenticationRequirement()
    return { ok: true, attempted: true }
  } catch {
    return { ok: false, attempted: true }
  }
}
