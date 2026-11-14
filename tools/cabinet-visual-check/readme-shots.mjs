// Снимки для readme.md: главная, тарифы, статус серверов — на ПК и телефоне, без рамок.
// Кабинет берётся из собранного dist с подставным API (как в release-shots-*.mjs).
//
// Из корня репозитория, после `cd web/cabinet && npm run build`:
//
//   node tools/cabinet-visual-check/readme-shots.mjs internal/cabinet/web/dist \
//     tools/cabinet-visual-check/fixtures documentation/img/readme

import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { chromium } from 'playwright'

const [, , distDir, fxDir, outDir] = process.argv
const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.png': 'image/png', '.webp': 'image/webp' }
const server = http.createServer((req, res) => {
  const url = (req.url ?? '/').split('?')[0]
  let file = path.join(distDir, decodeURIComponent(url).replace(/^\/cabinet/, ''))
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) file = path.join(distDir, 'index.html')
  res.writeHead(200, { 'Content-Type': MIME[path.extname(file)] ?? 'application/octet-stream' })
  res.end(fs.readFileSync(file))
})
await new Promise((r) => server.listen(5606, r))

const base = {
  '/auth/refresh': { access_token: 'stub', token_type: 'Bearer', expires_in: 3600 },
  '/me': { id: 1, email: 'client@example.com', email_verified: true, language: 'ru', providers: ['password'], has_telegram_link: true, has_password: true, google_oauth_enabled: false, is_admin: false, customer_id: 1 },
  '/admin/bootstrap': { providers: {}, features: {} },
}
const scene = (...names) => Object.assign({}, base, ...names.map((n) => JSON.parse(fs.readFileSync(path.join(fxDir, n), 'utf8'))))
fs.mkdirSync(outDir, { recursive: true })
const b = await chromium.launch()

async function shot(name, fx, route, w, h, { prefix = '/cabinet', scale = 1.5 } = {}) {
  const ctx = await b.newContext({ viewport: { width: w, height: h }, deviceScaleFactor: scale, colorScheme: 'dark', locale: 'ru-RU', timezoneId: 'Europe/Moscow' })
  await ctx.addInitScript(() => {
    localStorage.setItem('onboarding_completed', 'true')
    localStorage.setItem('cab_theme', 'dark')
  })
  await ctx.route('**/cabinet/api/**', (r) => {
    const p = new URL(r.request().url()).pathname.replace('/cabinet/api', '')
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(fx[p] ?? {}) })
  })
  const page = await ctx.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(String(e).slice(0, 120)))
  await page.goto(`http://localhost:5606${prefix}${route}`, { waitUntil: 'networkidle' })
  await page.waitForTimeout(2200)
  await page.mouse.move(2, h - 20)
  await page.screenshot({ path: path.join(outDir, name), type: 'png' })
  console.log(name, errors)
  await ctx.close()
}

const home = scene('split-ok.json')
await shot('home-pc.png', home, '/dashboard', 1440, 860)
await shot('home-phone.png', home, '/dashboard', 390, 844)
const tariffs = scene('checkout.json', 'tariffs.json')
// В фикстуре — рабочие названия; для readme подставляем аккуратный набор.
const GB = 1024 ** 3
const mk = (id, slug, name, description, devices, gb, prices, extra = {}) => ({ id, slug, name, description, device_limit: devices, traffic_limit_bytes: gb ? gb * GB : null, prices: prices.map((p, i) => ({ months: [1, 3, 6, 12][i], amount_rub: p })), ...extra })
tariffs['/tariffs'] = {
  ...tariffs['/tariffs'],
  tariffs: [
    mk(1, 'base', 'Базовый', 'Для одного-двух устройств', 2, 200, [150, 400, 750, 1320]),
    mk(2, 'standard', 'Стандарт', 'Безлимитный трафик, все локации', 4, 0, [250, 690, 1290, 2290], { is_popular: true }),
    mk(3, 'premium', 'Премиум', 'Безлимит и обход белых списков', 5, 0, [400, 1090, 2090, 3690]),
  ],
}
await shot('tariffs-pc.png', tariffs, '/tariffs', 1440, 860)
const admin = scene('admin-stats.json')
admin['/me'] = { ...admin['/me'], is_admin: true }
await shot('admin-pc.png', admin, '/admin/stats', 1440, 860)
await b.close()
server.close()
