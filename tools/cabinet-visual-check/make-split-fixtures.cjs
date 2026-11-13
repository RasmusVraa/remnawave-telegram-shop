// Фикстуры раздельного трафика: главная в разных состояниях (обход, исчерпание, пакеты).
// Запуск: node tools/cabinet-visual-check/make-split-fixtures.cjs
const fs = require('fs')
const path = require('path')

const dir = path.join(__dirname, 'fixtures')
const base = JSON.parse(fs.readFileSync(path.join(dir, 'home-full.json'), 'utf8'))
const GB = 1024 ** 3

const packs = [
  { id: 11, kind: 'bypass', bytes: 5 * GB, amount_rub: 59 },
  { id: 12, kind: 'bypass', bytes: 20 * GB, amount_rub: 199 },
  { id: 13, kind: 'bypass', bytes: 50 * GB, amount_rub: 399 },
  { id: 21, kind: 'main', bytes: 10 * GB, amount_rub: 49 },
  { id: 22, kind: 'main', bytes: 50 * GB, amount_rub: 179 },
  { id: 23, kind: 'main', bytes: 100 * GB, amount_rub: 299 },
]

function traffic({ mainLimit = 0, mainUsed = 18.4, bypassLimit = 30, bypassUsed = 12, bypassExtra = 0, blocked = false, onlyKind }) {
  const t = {
    enabled: true,
    can_buy: true,
    label: 'Обход',
    reset_at: '2026-12-01T00:00:00Z',
    main: { limit_bytes: mainLimit * GB, base_bytes: mainLimit * GB, extra_bytes: 0, used_bytes: mainUsed * GB },
    extras: bypassExtra ? [{ kind: 'bypass', bytes: bypassExtra * GB, valid_until: '2026-12-01T00:00:00Z' }] : [],
    bypass_blocked: blocked,
    packs: packs.filter((p) => (onlyKind ? p.kind === onlyKind : (p.kind === 'bypass' || mainLimit > 0))),
  }
  if (bypassLimit) {
    t.bypass = {
      limit_bytes: (bypassLimit + bypassExtra) * GB,
      base_bytes: bypassLimit * GB,
      extra_bytes: bypassExtra * GB,
      used_bytes: bypassUsed * GB,
    }
  }
  return t
}

// Расход по дням: 14 дней, часть ушла на обход.
const cats = Array.from({ length: 14 }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`)
const pts = [20, 30, 14, 22, 40, 18, 12, 26, 20, 34, 28, 16, 24, 18].map((v) => v * 0.05 * GB)
const byp = [0, 2, 0, 0, 6, 1, 0, 3, 2, 4, 0, 1, 2, 3].map((v) => v * 0.1 * GB)
const usage = {
  ...base['/me/traffic-usage'],
  period_start: '2026-11-01',
  period_end: '2026-11-14',
  categories: cats,
  points: pts,
  bypass_points: byp,
  total_bytes: pts.reduce((a, b) => a + b, 0),
}

const make = (name, tr, sub = {}) => {
  const f = { ...base, '/me/traffic': tr, '/me/traffic-usage': usage }
  f['/me/subscription'] = { ...base['/me/subscription'], traffic_limit_gb: tr.main.limit_bytes ? tr.main.limit_bytes / GB : null, traffic_used_gb: tr.main.used_bytes / GB, ...sub }
  fs.writeFileSync(path.join(dir, name), JSON.stringify(f, null, 2))
}

make('split-ok.json', traffic({}))
make('split-warn.json', traffic({ bypassUsed: 26 }))
make('split-blocked.json', traffic({ bypassUsed: 30, blocked: true }))
make('split-extra.json', traffic({ bypassUsed: 34, bypassExtra: 20 }))
make('split-both.json', traffic({ mainLimit: 100, mainUsed: 38, bypassUsed: 12 }))
console.log('ok')
