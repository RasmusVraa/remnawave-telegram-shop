// Скриншоты к посту релиза: снимки кабинета с жёлтой рамкой и подписью на том,
// что изменилось. tools/release отправляет их альбомом к посту в Telegram.
//
// Кабинет снимается из собранного dist с подставным API — без базы, бота и
// входа, и в кадр не попадают настоящие пользователи. Данные берутся из фикстуры
// (см. README рядом).
//
// Запуск из корня репозитория, после `cd web/cabinet && npm run build`:
//
//   node tools/cabinet-visual-check/release-shots.mjs internal/cabinet/web/dist \
//     tools/cabinet-visual-check/fixtures/release-connect-holiday.json release-screenshots
//
// Под новый релиз меняются только сценарии в конце файла: какую страницу
// открыть, что нажать и что обвести. Здесь — сценарии релиза 5.23.0 (ночные
// декор-темы и первый шаг гида подключения).
// mark(locator, 'Подпись', side): side — top | bottom | left | inside | inside-low.
// Каждый снимок после съёмки посмотреть глазами: подпись не должна закрывать
// текст интерфейса, рамка — резать соседние строки.

import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { chromium } from 'playwright'

const [, , distDir, fixturesFile, outDir] = process.argv
const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.png': 'image/png', '.webp': 'image/webp' }
const server = http.createServer((req, res) => {
  const url = (req.url ?? '/').split('?')[0]
  let file = path.join(distDir, decodeURIComponent(url).replace(/^\/cabinet/, ''))
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) file = path.join(distDir, 'index.html')
  res.writeHead(200, { 'Content-Type': MIME[path.extname(file)] ?? 'application/octet-stream' })
  res.end(fs.readFileSync(file))
})
await new Promise((r) => server.listen(5604, r))

const base = {
  '/auth/refresh': { access_token: 'stub', token_type: 'Bearer', expires_in: 3600 },
  '/me': { id: 1, email: 'admin@example.com', email_verified: true, language: 'ru', providers: ['password'], has_telegram_link: true, has_password: true, google_oauth_enabled: false, is_admin: true, customer_id: 1 },
  '/admin/bootstrap': { providers: {}, features: {} },
  ...JSON.parse(fs.readFileSync(fixturesFile, 'utf8')),
}
// Декор-тема приходит в /auth/bootstrap: сцена — та же фикстура с другой темой.
const withDecor = (theme) => ({ ...base, '/auth/bootstrap': { ...base['/auth/bootstrap'], decor_theme: theme } })
fs.mkdirSync(outDir, { recursive: true })

const b = await chromium.launch()
async function open(w, h, fx, route) {
  const ctx = await b.newContext({ viewport: { width: w, height: h }, deviceScaleFactor: 2, colorScheme: 'dark', locale: 'ru-RU', timezoneId: 'Europe/Moscow' })
  await ctx.addInitScript(() => {
    localStorage.setItem('onboarding_completed', 'true')
    localStorage.setItem('cab_theme', 'dark')
  })
  await ctx.route('**/cabinet/api/**', (route) => {
    const p = new URL(route.request().url()).pathname.replace('/cabinet/api', '')
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(fx[p] ?? {}) })
  })
  const page = await ctx.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(String(e)))
  await page.goto(`http://localhost:5604/cabinet${route}`, { waitUntil: 'networkidle' })
  // Сцены декор-тем проявляются плавно — даём им дорисоваться.
  await page.waitForTimeout(2600)
  return { ctx, page, errors }
}

// Жёлтая рамка вокруг элемента и подпись рядом. Пустая подпись — только рамка:
// подпись тогда ставится caption() туда, где она не закроет текст.
async function mark(locator, label, where = 'top') {
  await locator.first().evaluate((el, args) => {
    const [text, side] = args
    const r = el.getBoundingClientRect()
    const pad = 6
    const box = document.createElement('div')
    Object.assign(box.style, {
      position: 'fixed', left: `${r.left - pad}px`, top: `${r.top - pad}px`,
      width: `${r.width + pad * 2}px`, height: `${r.height + pad * 2}px`,
      border: '3px solid #FFB020', borderRadius: '14px', boxShadow: '0 0 0 5px rgba(255,176,32,.22)',
      pointerEvents: 'none', zIndex: 2147483646,
    })
    document.body.appendChild(box)
    if (!text) return
    const tag = document.createElement('div')
    tag.textContent = text
    Object.assign(tag.style, {
      position: 'fixed', background: '#FFB020', color: '#1d1400',
      font: '800 15px/1.2 Manrope, Inter, system-ui, sans-serif', padding: '7px 12px',
      borderRadius: '10px', zIndex: 2147483647, whiteSpace: 'nowrap', boxShadow: '0 6px 18px rgba(0,0,0,.35)',
    })
    document.body.appendChild(tag)
    const th = tag.offsetHeight
    const tw = tag.offsetWidth
    let top = side === 'bottom' || r.top - pad - th - 8 < 4 ? r.bottom + pad + 8 : r.top - pad - th - 8
    let left = Math.min(Math.max(4, r.left - pad), window.innerWidth - tw - 8)
    if (side === 'inside') {
      top = r.top + 14
      left = r.right - tw - 64
    }
    if (side === 'inside-low') {
      top = r.top + 40
      left = r.right - tw - 24
    }
    if (side === 'left') {
      top = r.top + (r.height - th) / 2
      left = r.left - pad - tw - 14
    }
    tag.style.top = `${top}px`
    tag.style.left = `${left}px`
  }, [label, where])
}
// Подпись без рамки — когда изменился весь экран (тема, сцена) и обводить нечего.
async function caption(page, label, top = 128) {
  await page.evaluate(([text, y]) => {
    const tag = document.createElement('div')
    tag.textContent = text
    Object.assign(tag.style, {
      position: 'fixed', top: `${y}px`, left: '50%', transform: 'translateX(-50%)',
      background: '#FFB020', color: '#1d1400', font: '800 17px/1.2 Manrope, Inter, system-ui, sans-serif',
      padding: '8px 14px', borderRadius: '10px', zIndex: 2147483647, whiteSpace: 'nowrap',
      boxShadow: '0 6px 18px rgba(0,0,0,.35)',
    })
    document.body.appendChild(tag)
  }, [label, top])
}
const shot = (page, name) => page.screenshot({ path: path.join(outDir, name) })

{
  // Земля со снеговиком прижата к низу окна — нужен высокий кадр, чтобы она не заслоняла карточки.
  const { ctx, page, errors } = await open(1440, 1200, withDecor('new_year_2'), '/subscription')
  // Между гирляндой и карточкой, по центру — заголовок страницы слева не задевает.
  await caption(page, 'Новая тема «Новый год 2»', 96)
  await shot(page, '01-new-year-2.png')
  console.log('1', errors)
  await ctx.close()
}
{
  const { ctx, page, errors } = await open(1440, 900, withDecor('halloween_2'), '/landing')
  await caption(page, 'Новая тема «Хэллоуин 2» — и на лендинге', 140)
  await shot(page, '02-halloween-2-landing.png')
  console.log('2', errors)
  await ctx.close()
}
{
  const { ctx, page, errors } = await open(390, 844, withDecor('off'), '/connections')
  const yes = page.getByRole('button', { name: 'Да, добавить подписку' })
  await yes.scrollIntoViewIfNeeded()
  await page.waitForTimeout(400)
  // Над кнопками — описание шага, под ними — помощь: подпись уходит на пустой низ картинки.
  await mark(yes.locator('xpath=..'), '', 'top')
  const ill = await page.locator('.cg-step-enter > :first-child').first().boundingBox()
  await caption(page, 'Приложение уже есть — один тап', ill ? ill.y + ill.height - 46 : 360)
  await shot(page, '03-connect-phone.png')
  console.log('3', errors)
  await ctx.close()
}
{
  const { ctx, page, errors } = await open(1440, 900, withDecor('off'), '/connections')
  await mark(page.getByRole('button', { name: 'Уже есть — добавить' }), 'Сразу добавить подписку', 'bottom')
  await shot(page, '04-connect-desktop.png')
  console.log('4', errors)
  await ctx.close()
}
await b.close()
server.close()
