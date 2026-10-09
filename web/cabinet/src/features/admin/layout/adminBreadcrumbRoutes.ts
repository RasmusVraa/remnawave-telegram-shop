/** Статические сегменты admin routes → i18n key (без `/admin` prefix). */
export const ADMIN_BREADCRUMB_ROUTE_KEYS: Record<string, string> = {
  '': 'admin.nav.dashboard',
  stats: 'admin.nav.stats',
  users: 'admin.nav.users',
  payments: 'admin.nav.payments',
  promos: 'admin.nav.promos',
  tariffs: 'admin.nav.tariffs',
  loyalty: 'admin.nav.loyalty',
  broadcast: 'admin.nav.broadcast',
  settings: 'admin.nav.settings',
  infra: 'admin.nav.infra',
  status: 'admin.nav.status',
  sync: 'admin.nav.sync',
}

export function parseAdminPath(pathname: string): {
  sectionKey: string | null
  userId: number | null
} {
  if (!pathname.startsWith('/admin')) {
    return { sectionKey: null, userId: null }
  }
  const rest = pathname.slice('/admin'.length).replace(/^\//, '')
  if (!rest) {
    return { sectionKey: '', userId: null }
  }
  const parts = rest.split('/').filter(Boolean)
  const sectionKey = parts[0] ?? null
  if (sectionKey === 'users' && parts.length >= 2) {
    const id = parseInt(parts[1], 10)
    return { sectionKey, userId: Number.isFinite(id) ? id : null }
  }
  return { sectionKey, userId: null }
}
