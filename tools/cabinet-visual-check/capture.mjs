#!/usr/bin/env node
/**
 * Снимает страницы собранного кабинета с подставным API.
 *
 * Зачем: сверить свою вёрстку с макетом можно только глазами. Собранный dist
 * поднимается на локальном порту, все запросы к /cabinet/api/** перехватываются
 * и отдаются из файла-фикстуры, поэтому ни база, ни бот, ни авторизация не
 * нужны — и снимок можно сделать за секунды, не выкатывая ветку на стенд.
 *
 * Пример:
 *   node tools/cabinet-visual-check/capture.mjs \
 *     --route /cabinet/admin/stats \
 *     --fixtures tools/cabinet-visual-check/fixtures/admin-stats.json \
 *     --out .tmp/shots \
 *     --step 'Деньги' --step 'Пользователи' --step 'Механики'
 *
 * Полный список ключей — README.md рядом.
 */
import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.webp': 'image/webp',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
}

/**
 * Git Bash на Windows разворачивает аргумент, похожий на unix-путь, в путь
 * установки MSYS: `/cabinet/admin/stats` приезжает как
 * `C:/Program Files/Git/cabinet/admin/stats`. Ключ MSYS_NO_PATHCONV помнят не
 * все, поэтому маршрут чинится здесь — берём всё начиная с `/cabinet`.
 */
function normalizeRoute(raw) {
  const at = raw.indexOf('/cabinet')
  const route = at > 0 ? raw.slice(at) : raw
  return route.startsWith('/') ? route : `/${route}`
}

/** Самый точный ключ фикстуры для пути и параметров запроса (см. обработчик маршрутов). */
function matchFixtureKey(map, path, params) {
  let best = null
  let bestScore = -1
  for (const key of Object.keys(map)) {
    const [kp, kq] = key.split('?')
    if (kp !== path) continue
    const want = new URLSearchParams(kq ?? '')
    let ok = true
    let score = 0
    for (const [k, v] of want) {
      if (params.get(k) !== v) ok = false
      score++
    }
    if (ok && score > bestScore) {
      best = key
      bestScore = score
    }
  }
  return best
}

function parseArgs(argv) {
  const out = {
    dist: 'internal/cabinet/web/dist',
    route: '/cabinet/admin/stats',
    out: '.tmp/cabinet-shots',
    fixtures: null,
    viewports: [],
    steps: [],
    hover: null,
    element: null,
    scroll: null,
    scrollTimes: 0,
    themes: ['dark'],
    locale: 'ru-RU',
    timezone: 'Europe/Moscow',
    port: 5599,
    settle: 1800,
    decor: null,
    css: null,
  }
  for (let i = 2; i < argv.length; i++) {
    const key = argv[i]
    const val = argv[i + 1]
    switch (key) {
      case '--dist': out.dist = val; i++; break
      case '--route': out.route = normalizeRoute(val); i++; break
      case '--out': out.out = val; i++; break
      case '--fixtures': out.fixtures = val; i++; break
      case '--viewport': out.viewports.push(val); i++; break
      case '--step': out.steps.push(val); i++; break
      case '--element': out.element = val; i++; break
      case '--scroll': out.scroll = Number(val); i++; break
      case '--scroll-times': out.scrollTimes = Number(val); i++; break
      case '--decor': out.decor = val; i++; break
      case '--css': out.css = val; i++; break
      case '--theme': out.themes = val.split(','); i++; break
      case '--locale': out.locale = val; i++; break
      case '--timezone': out.timezone = val; i++; break
      case '--port': out.port = Number(val); i++; break
      case '--settle': out.settle = Number(val); i++; break
      case '--hover': out.hover = val; i++; break
      default:
        if (key.startsWith('--')) {
          console.error(`Неизвестный ключ: ${key}`)
          process.exit(2)
        }
    }
  }
  if (out.viewports.length === 0) out.viewports = ['1440x1100', '390x900']
  return out
}

function loadPlaywright() {
  return import('playwright').catch(() => {
    console.error(
      'Не найден playwright. Поставьте его и браузер:\n' +
        '  cd tools/cabinet-visual-check && npm ci && npx playwright install chromium',
    )
    process.exit(3)
  })
}

/**
 * Статика dist с SPA-фолбэком.
 *
 * Префикс /cabinet срезается: в проде его снимает nginx, а vite собирает пути
 * относительно корня. Всё, чего нет на диске, отдаётся index.html — иначе
 * прямой заход на /cabinet/admin/stats вернул бы 404 вместо приложения.
 */
function createServer(distDir) {
  return http.createServer((req, res) => {
    const url = (req.url ?? '/').split('?')[0]
    let file = path.join(distDir, url.replace(/^\/cabinet/, ''))
    if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) {
      file = path.join(distDir, 'index.html')
    }
    res.writeHead(200, {
      'Content-Type': MIME[path.extname(file)] ?? 'application/octet-stream',
    })
    res.end(fs.readFileSync(file))
  })
}

/**
 * Заглушки, без которых кабинет не пустит дальше логина: тихий refresh по
 * cookie и профиль с флагом администратора. Фикстура может их переопределить.
 */
const AUTH_FIXTURES = {
  '/auth/refresh': { access_token: 'stub', token_type: 'Bearer', expires_in: 3600 },
  '/me': {
    id: 1,
    email: 'admin@example.com',
    email_verified: true,
    language: 'ru',
    providers: ['password'],
    has_telegram_link: true,
    has_password: true,
    google_oauth_enabled: false,
    is_admin: true,
    customer_id: 1,
  },
  '/admin/bootstrap': { providers: {}, features: {} },
}

async function main() {
  const args = parseArgs(process.argv)
  const { chromium } = await loadPlaywright()

  const distDir = path.resolve(args.dist)
  if (!fs.existsSync(path.join(distDir, 'index.html'))) {
    console.error(
      `В ${distDir} нет index.html. Соберите кабинет: cd web/cabinet && npm run build`,
    )
    process.exit(4)
  }

  let fixtures = { ...AUTH_FIXTURES }
  if (args.fixtures) {
    const raw = JSON.parse(fs.readFileSync(path.resolve(args.fixtures), 'utf8'))
    fixtures = { ...fixtures, ...raw }
  }

  const outDir = path.resolve(args.out)
  fs.mkdirSync(outDir, { recursive: true })

  const server = createServer(distDir)
  await new Promise((resolve) => server.listen(args.port, resolve))

  const browser = await chromium.launch(process.env.CAB_CHROME ? { executablePath: process.env.CAB_CHROME } : {})
  const written = []

  try {
    for (const theme of args.themes) {
      for (const vp of args.viewports) {
        const [width, height] = vp.split('x').map(Number)
        const ctx = await browser.newContext({
          viewport: { width, height },
          deviceScaleFactor: 2,
          colorScheme: theme === 'light' ? 'light' : 'dark',
          locale: args.locale,
          timezoneId: args.timezone,
        })

        // Гайд по подключению закрывает экран своим оверлеем и перехватывает
        // клики шагов: помечаем его пройденным до загрузки приложения.
        // Тема кабинета живёт в localStorage ('cab_theme'), а не в prefers-color-scheme:
        // без этого --theme light отдаёт тёмный кадр.
        await ctx.addInitScript((mode) => {
          try {
            localStorage.setItem('onboarding_completed', 'true')
            localStorage.setItem('cab_theme', mode)
          } catch {
            /* ignore */
          }
        }, theme === 'light' ? 'light' : 'dark')

        // Всё под /cabinet/api отвечает из фикстуры. Неизвестный путь — пустой
        // объект, а не 404: экран не должен падать из-за метрики, которой в
        // фикстуре просто нет.
        // Ключ фикстуры может уточнять параметры запроса: «/payments/preview?renew_extra_hwid=false»
        // побеждает голый «/payments/preview», если все его параметры совпали. «__delay» — та же
        // схема ключей → задержка ответа в мс: так снимается состояние «идёт пересчёт».
        await ctx.route('**/cabinet/api/**', async (route) => {
          const url = new URL(route.request().url())
          const p = url.pathname.replace('/cabinet/api', '')
          const key = matchFixtureKey(fixtures, p, url.searchParams)
          const delay = matchFixtureKey(fixtures.__delay ?? {}, p, url.searchParams)
          if (delay != null) await new Promise((r) => setTimeout(r, fixtures.__delay[delay]))
          route
            .fulfill({
              status: 200,
              contentType: 'application/json',
              body: JSON.stringify(key != null ? fixtures[key] : {}),
            })
            .catch(() => {})
        })

        const page = await ctx.newPage()
        const errors = []
        page.on('console', (m) => {
          if (m.type() === 'error') errors.push(m.text())
        })
        page.on('pageerror', (e) => errors.push(String(e)))

        await page.goto(`http://localhost:${args.port}${args.route}`, {
          waitUntil: 'networkidle',
        })
        await page.waitForTimeout(args.settle)
        // Декор-тема приходит из конфига; для проверки палитры подменяем атрибут на <html>.
        if (args.css) await page.addStyleTag({ content: fs.readFileSync(args.css, 'utf8') })
        if (args.decor) {
          await page.evaluate((d) => { document.documentElement.dataset.cabinetDecor = d }, args.decor)
          await page.waitForTimeout(400)
        }

        const tag = args.decor ? `${args.decor}-${theme}-${width}` : `${theme}-${width}`
        const shots = [{ name: 'main', run: async () => {} }]
        for (const step of args.steps) {
          shots.push({
            name: step.replace(/[^\p{L}\p{N}_-]+/gu, '-').toLowerCase(),
            run: async () => {
              // `css=` — запасной путь для того, у чего нет роли: строк таблицы,
              // карточек списка. Без него модалку, открывающуюся по клику на
              // строку, снять нельзя вообще.
              const target = step.startsWith('css=')
                ? page.locator(step.slice(4))
                : page
                    .getByRole('tab', { name: step })
                    .or(page.getByRole('button', { name: step }))
                    // Пункты выпадающего меню — role="menuitem", кнопкой их не найти.
                    .or(page.getByRole('menuitem', { name: step }))
              if (await target.count()) {
                await target.first().click()
                await page.waitForTimeout(900)
              } else {
                console.warn(`  шаг «${step}»: элемент не найден, снимок без клика`)
              }
            },
          })
        }

        for (const shot of shots) {
          await shot.run()
          const hoverTarget = args.hover ? page.locator(args.hover) : null
          if (hoverTarget && (await hoverTarget.count())) {
            // Наведение проверяется только так: в статичном кадре :hover не виден.
            await hoverTarget.first().hover()
          } else {
            // Курсор уводится из-под графиков: иначе в кадр попадает подсказка.
            await page.mouse.move(2, 2)
          }
          await page.waitForTimeout(250)
          const file = path.join(outDir, `${tag}-${shot.name}.png`)
          // Длинные списки с подгрузкой: докручиваем до низа N раз, ждём новые строки.
          for (let k = 0; k < args.scrollTimes; k++) {
            await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight))
            await page.waitForTimeout(1200)
          }
          if (args.scrollTimes) console.log(`  высота страницы после ${args.scrollTimes} прокруток: ${await page.evaluate(() => document.documentElement.scrollHeight)}px`)
          if (args.scroll != null) {
            // Кадр окна после прокрутки — проверка липких блоков (sticky); fullPage их не покажет.
            // -1 — не крутить самим: кадр окна там, куда страницу привели шаги.
            if (args.scroll >= 0) await page.evaluate((y) => window.scrollTo(0, y), args.scroll)
            await page.waitForTimeout(800)
            await page.screenshot({ path: file })
          } else if (args.element) {
            await page.locator(args.element).first().screenshot({ path: file })
          } else {
            await page.screenshot({ path: file, fullPage: true })
          }
          written.push(file)
        }

        if (errors.length) {
          console.warn(`[${tag}] ошибок в консоли: ${errors.length}`)
          for (const e of errors.slice(0, 5)) console.warn(`   ${e}`)
        }
        await ctx.close()
      }
    }
  } finally {
    await browser.close()
    server.close()
  }

  for (const f of written) console.log(f)
  console.log(`Готово: ${written.length} снимков в ${outDir}`)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
