// Скриншоты к посту релиза 5.29.0: картинки в чате поддержки, «Оповещения админу»,
// честная плашка статуса серверов. tools/release отправляет их альбомом к посту.
//
// Кабинет снимается из собранного dist с подставным API — без базы, бота и входа.
// Картинки в чате — выдуманные экраны VPN-приложения, их рисует сам скрипт.
//
// Запуск из корня репозитория, после `cd web/cabinet && npm run build`:
//
//   node tools/cabinet-visual-check/release-shots-5-29.mjs internal/cabinet/web/dist \
//     tools/cabinet-visual-check/fixtures release-screenshots
//
// Второй аргумент — папка фикстур: сцены берут support-typing.json, admin-alerts.json,
// status-partial.json. mark(locator, 'Подпись', side): side — top | bottom | left | inside.
// RAW=1 — без рамок и подписей, ONLY=1,3 — только эти сцены.

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
await new Promise((r) => server.listen(5605, r))

const base = {
  '/auth/refresh': { access_token: 'stub', token_type: 'Bearer', expires_in: 3600 },
  '/me': { id: 1, email: 'admin@example.com', email_verified: true, language: 'ru', providers: ['password'], has_telegram_link: true, has_password: true, google_oauth_enabled: false, is_admin: true, customer_id: 1 },
  '/admin/bootstrap': { providers: {}, features: {} },
}
const loadFx = (name) => JSON.parse(fs.readFileSync(path.join(fxDir, name), 'utf8'))
const scene = (name, extra = {}) => ({ ...base, ...loadFx(name), ...extra })
fs.mkdirSync(outDir, { recursive: true })

const b = await chromium.launch()

// Картинки для чата поддержки: id вложения → PNG.
const images = {}
async function drawImage(id, w, h, html) {
  const ctx = await b.newContext({ viewport: { width: w, height: h }, deviceScaleFactor: 1 })
  const page = await ctx.newPage()
  await page.setContent(`<html><body style="margin:0;width:${w}px;height:${h}px;overflow:hidden;font-family:Inter,system-ui,sans-serif">${html}</body></html>`)
  images[id] = await page.screenshot({ type: 'png' })
  await ctx.close()
}
// Клиент: экран VPN-приложения с ошибкой подключения.
await drawImage(11, 540, 960, `
  <div style="height:100%;background:linear-gradient(#141a2b,#0b0f1a);color:#e8ecf5;padding:56px 36px;box-sizing:border-box">
    <div style="font-size:30px;font-weight:800">VPN</div>
    <div style="margin-top:220px;text-align:center">
      <div style="margin:0 auto;width:150px;height:150px;border-radius:50%;border:8px solid #f07178;display:flex;align-items:center;justify-content:center;font-size:72px;color:#f07178">!</div>
      <div style="margin-top:36px;font-size:30px;font-weight:700">Не удалось подключиться</div>
      <div style="margin-top:14px;font-size:22px;color:#98a2b8">context deadline exceeded</div>
    </div>
    <div style="position:absolute;left:36px;right:36px;bottom:70px;height:76px;border-radius:38px;background:#2a3350;display:flex;align-items:center;justify-content:center;font-size:24px;font-weight:700">Подключить</div>
  </div>`)
// Поддержка: где обновить подписку.
await drawImage(12, 900, 400, `
  <div style="height:100%;background:#0f1424;color:#e8ecf5;padding:40px 120px;box-sizing:border-box">
    <div style="font-size:28px;font-weight:800">Профили</div>
    <div style="margin-top:30px;border-radius:22px;background:#1b2338;padding:26px 28px;display:flex;align-items:center;justify-content:space-between">
      <div><div style="font-size:24px;font-weight:700">Моя подписка</div><div style="margin-top:8px;font-size:18px;color:#98a2b8">обновлено 12 дней назад</div></div>
      <div style="border:4px solid #FFB020;border-radius:18px;padding:16px 22px;font-size:20px;font-weight:700;color:#FFB020">⟳ Обновить</div>
    </div>
    <div style="margin-top:22px;border-radius:22px;background:#1b2338;padding:26px 28px;font-size:20px;color:#98a2b8">Серверы: Германия, Швеция, Эстония…</div>
  </div>`)
// Клиент: два снимка «после» — сеткой.
await drawImage(13, 540, 960, `
  <div style="height:100%;background:linear-gradient(#10261d,#0b0f1a);color:#e8ecf5;padding:56px 36px;box-sizing:border-box;text-align:center">
    <div style="text-align:left;font-size:30px;font-weight:800">VPN</div>
    <div style="margin:220px auto 0;width:150px;height:150px;border-radius:50%;border:8px solid #3dd68c;display:flex;align-items:center;justify-content:center;font-size:70px;color:#3dd68c">✓</div>
    <div style="margin-top:36px;font-size:30px;font-weight:700">Подключено</div>
    <div style="margin-top:14px;font-size:22px;color:#98a2b8">Германия · 38 мс</div>
  </div>`)
await drawImage(14, 540, 960, `
  <div style="height:100%;background:#0b0f1a;color:#e8ecf5;padding:56px 36px;box-sizing:border-box">
    <div style="font-size:30px;font-weight:800">Скорость</div>
    <div style="margin-top:200px;text-align:center;font-size:96px;font-weight:800">184</div>
    <div style="text-align:center;font-size:26px;color:#98a2b8">Мбит/с</div>
  </div>`)

async function open(w, h, fx, route, prefix = '/cabinet') {
  const ctx = await b.newContext({ viewport: { width: w, height: h }, deviceScaleFactor: 2, colorScheme: 'dark', locale: 'ru-RU', timezoneId: 'Europe/Moscow' })
  await ctx.addInitScript(() => {
    localStorage.setItem('onboarding_completed', 'true')
    localStorage.setItem('cab_theme', 'dark')
  })
  await ctx.route('**/cabinet/api/**', (route) => {
    const p = new URL(route.request().url()).pathname.replace('/cabinet/api', '')
    const img = p.match(/^\/support\/attachments\/(\d+)$/)
    if (img && images[img[1]]) {
      route.fulfill({ status: 200, contentType: 'image/png', body: images[img[1]] })
      return
    }
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(fx[p] ?? {}) })
  })
  const page = await ctx.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(String(e)))
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text().slice(0, 300)) })
  await page.goto(`http://localhost:5605${prefix}${route}`, { waitUntil: 'networkidle' })
  await page.waitForTimeout(2200)
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
    if (side === 'inside-bottom') {
      top = r.bottom - th - 5
      left = r.right - tw - 16
    }
    if (side === 'inline-right') {
      top = r.top + (r.height - th) / 2
      left = r.right - tw - 16
    }
    if (side === 'above-right') {
      top = r.top - pad - th - 10
      left = r.right - tw - 96
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
// Прокрутить так, чтобы блок оказался под шапкой, и обвести его.
async function scrollToBlock(page, locator, offset = 120) {
  await locator.first().evaluate((el, off) => {
    const y = el.getBoundingClientRect().top + window.scrollY - off
    window.scrollTo(0, y)
  }, offset)
  await page.waitForTimeout(500)
}
// RAW=1 — снимки без рамок и подписей: посмотреть, что вообще в кадре.
const RAW = process.env.RAW === '1'
const ONLY = (process.env.ONLY ?? '').split(',').filter(Boolean)
const want = (n) => ONLY.length === 0 || ONLY.includes(String(n))
// Имя вида '1/05-…': подпапка — номер альбома, альбом N уходит ответом на N-е сообщение поста.
const shot = (page, name) => {
  const file = path.join(outDir, name.replace('.png', '.jpg'))
  fs.mkdirSync(path.dirname(file), { recursive: true })
  return page.screenshot({ path: file, type: 'jpeg', quality: 86 })
}
const M = RAW ? async () => {} : mark
const C = RAW ? async () => {} : caption
const minutesAgo = (m) => new Date(Date.now() - m * 60 * 1000).toISOString()

if (want(1)) {
  // 1. Чат поддержки на телефоне: клиент прислал скриншот, поддержка ответила картинкой.
  const fx = scene('support-typing.json')
  fx['/auth/bootstrap'] = { ...fx['/auth/bootstrap'], support_chat_enabled: true, support_attachments_enabled: true, support_attachments_max_files: 5, support_attachments_max_mb: 10 }
  // Блок «Информация» под чатом читает список серверов: без него страница падает на подставном API.
  fx['/public/status'] = { available: true, nodes: [] }
  fx['/support/conversation'] = {
    has_open_ticket: true, ticket_status: 'open', unread_count: 0, support_typing: false,
    messages: [
      { id: 1, direction: 'in', text: 'Не подключается VPN на телефоне', delivery_status: 'sent', created_at: minutesAgo(18) },
      { id: 2, direction: 'out', text: 'Похоже, в приложении старая подписка. Обновите её вот здесь:', author_label: 'Поддержка', created_at: minutesAgo(15),
        attachments: [{ id: 12, mime: 'image/png', width: 900, height: 400 }] },
      { id: 3, direction: 'in', text: 'Заработало, спасибо!', delivery_status: 'sent', created_at: minutesAgo(9),
        attachments: [{ id: 13, mime: 'image/png', width: 540, height: 960 }, { id: 14, mime: 'image/png', width: 540, height: 960 }] },
    ],
  }
  const { ctx, page, errors } = await open(390, 844, fx, '/support#chat')
  await page.waitForTimeout(1200)
  // Лента — снизу: картинки клиента сеткой и ответ поддержки с картинкой.
  await C(page, 'Картинки прямо в чате поддержки', 70)
  await shot(page, '01-support-images.png')
  console.log('1', errors)
  await ctx.close()
}
if (want(2)) {
  // 2. «Оповещения админу»: общий чат, у каждого вида тема и проверка.
  const fx = scene('admin-alerts.json')
  const { ctx, page, errors } = await open(1440, 1100, fx, '/admin/settings')
  await page.getByRole('tab', { name: 'Уведомления' }).click()
  await page.waitForTimeout(500)
  await page.getByRole('button', { name: 'Оповещения админу' }).click()
  await page.waitForTimeout(700)
  await page.getByRole('button', { name: 'Тест' }).first().click()
  await page.waitForTimeout(900)
  const card = page.locator('div.rounded-lg', { hasText: 'Система' }).filter({ hasText: 'Куда уйдёт' }).first()
  await scrollToBlock(page, page.getByText('Чат для оповещений'), 150)
  await M(page.getByText('Чат для оповещений').locator('xpath=..'), 'Один чат на всё, тема на каждый вид', 'inside')
  await M(card, 'Проверка в один клик', 'inside-bottom')
  const payments = page.locator('div.rounded-lg', { hasText: 'Оплаты клиентов' }).filter({ hasText: 'Что присылать' }).first()
  await M(payments, 'Галочки событий', 'inside-bottom')
  await shot(page, '02-admin-alerts.png')
  console.log('2', errors)
  await ctx.close()
}
if (want(3)) {
  // 3. Публичный статус: перебои из России больше не прячутся за «Все на связи».
  const fx = scene('status-partial.json')
  for (const n of fx['/public/status'].nodes) if (n.probe) n.probe.measured_at = minutesAgo(4)
  fx['/auth/bootstrap'] = { ...(fx['/auth/bootstrap'] ?? {}), status_nav_visible: true }
  // Без карты заголовок и таблица помещаются в один кадр.
  fx['/public/status'] = { ...fx['/public/status'], show_map: false }
  const { ctx, page, errors } = await open(1440, 900, fx, '/status', '')
  await page.mouse.move(2, 890)
  await scrollToBlock(page, page.locator('h1').first(), 170)
  await M(page.locator('h1').first().locator('xpath=..'), 'Перебои из России видны сразу', 'top')
  // Ячейка «Из России» у Германии — целиком, без подписи: подпись закрыла бы соседние строки.
  const de = page.locator('li', { hasText: 'Германия' }).first()
  await M(de.getByText('С перебоями').first().locator('xpath=..'), '')
  await shot(page, '03-status-partial.png')
  console.log('3', errors)
  await ctx.close()
}

if (want(4) || want(5) || want(6)) {
  // 4–6. «Мониторинг»: лента событий, плашка «Требует внимания» и история в карточке клиента.
  const mon = scene('admin-monitoring.json')
  mon['/admin/monitoring/events'] = { ...mon['/admin/monitoring/events'], bot_actions_enabled: false }
  mon['/admin/monitoring/events'].items = mon['/admin/monitoring/events'].items.filter((e) => !String(e.type).startsWith('bot.'))
  if (want(4)) {
    const { ctx, page, errors } = await open(1440, 900, mon, '/admin/monitoring')
    await M(page.locator('section', { hasText: 'Требует внимания' }).first(), 'Что горит — сразу на виду', 'above-right')
    await shot(page, '04-monitoring-pc.png')
    console.log('4', errors)
    await ctx.close()
  }
  if (want(5)) {
    const { ctx, page, errors } = await open(390, 844, mon, '/admin/monitoring')
    await C(page, 'События в реальном времени', 770)
    await shot(page, '05-monitoring-phone.png')
    console.log('5', errors)
    await ctx.close()
  }
  if (want(6)) {
    const fx = scene('admin-user.json')
    fx['/admin/monitoring/events'] = { ...mon['/admin/monitoring/events'], total: 12 }
    const { ctx, page, errors } = await open(1440, 900, fx, '/admin/users/1560')
    await page.getByRole('button', { name: /^История/ }).first().click()
    await page.waitForTimeout(900)
    await M(page.getByRole('button', { name: 'Фильтр событий' }), 'Вся история клиента', 'left')
    await shot(page, '06-client-history.png')
    console.log('6', errors)
    await ctx.close()
  }
}

await b.close()
server.close()
