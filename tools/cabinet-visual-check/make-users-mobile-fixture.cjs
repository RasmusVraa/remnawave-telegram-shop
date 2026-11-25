// Фикстура списка пользователей админки: даты считаются от «сейчас», чтобы красные/жёлтые подписи были честными.
const fs = require('fs')
const day = 86400000
const at = (n) => (n == null ? null : new Date(Date.now() + n * day).toISOString())
const mk = (id, name, st, n, tar, photo) => ({
  id, telegram_id: String(1000000 + id), telegram_username: name, language: 'ru', expire_at: at(n),
  created_at: '2026-09-04T09:17:00Z', extra_hwid: 0, current_tariff_id: tar, loyalty_xp: 0, is_web_only: false, status: st,
  ...(photo ? { avatar_url: 'data:image/svg+xml;utf8,' + encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="36" height="36"><defs><linearGradient id="g"><stop offset="0" stop-color="${photo[0]}"/><stop offset="1" stop-color="${photo[1]}"/></linearGradient></defs><rect width="36" height="36" fill="url(#g)"/></svg>`) } : {}),
})
const items = [
  mk(2104, null, 'inactive', null, null),
  mk(2102, 'Pampers66608', 'trial', 2, null, ['#f6a35b', '#d8456b']),
  mk(2101, 'Petuhdima1', 'trial', 1, null),
  mk(2100, null, 'expired', -12, 1),
  mk(2099, 'Maloy_Kavani', 'trial', 3, null, ['#5bc0f6', '#4558d8']),
  mk(2097, null, 'active', 35, 1),
  mk(2096, 'Aleksandra_0627', 'active', 5, 2, ['#8be0a4', '#2a9d8f']),
  mk(2095, 'ivan_k', 'active', 187, 2),
]
const out = {
  '/admin/bootstrap': { sales_mode: 'tariffs', loyalty_enabled: false, fortune_enabled: false, version: 'dev-visual' },
  '/admin/tariffs': [{ id: 1, slug: 'std', name: 'Стандарт' }, { id: 2, slug: 'unl', name: 'Безлимит' }],
  '/admin/reviews': { items: [], total: 0, counts: { pending: 0, all: 0 } },
  '/admin/users': { items, total: 1602, page: 1, limit: 50 },
}
// Страницы по 50 с разными id — для проверки подгрузки списка.
const names = ['alex', 'maria_k', null, 'dima77', 'sveta', null, 'oleg_v', 'nika']
for (let p = 1; p <= 6; p++) {
  out[`/admin/users?page=${p}`] = {
    items: Array.from({ length: 50 }, (_, i) => {
      const id = 2200 - (p - 1) * 50 - i
      const k = id % 4
      return mk(id, names[id % names.length], k === 0 ? 'expired' : k === 1 ? 'trial' : 'active', k === 0 ? -(id % 30) - 1 : (id % 90) + 1, (id % 2) + 1)
    }),
    total: 1602, page: p, limit: 50,
  }
}
fs.writeFileSync(process.argv[2], JSON.stringify(out, null, 2))
