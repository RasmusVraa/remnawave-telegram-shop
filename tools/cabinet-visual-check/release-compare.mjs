// Сборка кадров к посту релиза: плашка-подпись над снимком, а для пары «было → стало»
// два снимка рядом. Описание кадров — JSON-файл:
//
//   node tools/cabinet-visual-check/release-compare.mjs <frames.json> <out-dir>
//
// [{ "out": "01-x.jpg", "title": "Заголовок", "width": 1440,
//    "items": [{ "src": "a.png", "label": "Сейчас · 5.31.0", "tone": "old" },
//              { "src": "b.png", "label": "В 5.32.0", "tone": "new" }] }]
// width — ширина одного снимка в кадре (CSS px), снимки сняты с deviceScaleFactor 2.
import fs from 'node:fs'
import path from 'node:path'
import { chromium } from 'playwright'

const [, , framesFile, outDir] = process.argv
const frames = JSON.parse(fs.readFileSync(framesFile, 'utf8'))
const base = path.dirname(path.resolve(framesFile))
fs.mkdirSync(outDir, { recursive: true })

const b = await chromium.launch()
const page = await b.newPage({ viewport: { width: 1600, height: 900 }, deviceScaleFactor: 2 })
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;')
for (const f of frames) {
  const w = f.width ?? 1440
  const cells = f.items
    .map((it) => {
      const data = fs.readFileSync(path.resolve(base, it.src)).toString('base64')
      return `<div class="cell"><div class="lbl ${it.tone ?? ''}">${esc(it.label ?? '')}</div><img src="data:image/png;base64,${data}" style="width:${w}px"></div>`
    })
    .join('')
  await page.setContent(`<!doctype html><html><head><meta charset="utf-8"><style>
    body{margin:0;background:#070b16;font-family:Inter,'Segoe UI',system-ui,sans-serif}
    #f{display:inline-block;padding:26px 28px 28px;background:linear-gradient(180deg,#0d1426,#070b16)}
    h1{margin:0 0 18px;color:#eef3fb;font-size:30px;font-weight:800;letter-spacing:-.01em}
    .row{display:flex;gap:28px;align-items:flex-start}
    .cell{display:flex;flex-direction:column;gap:12px}
    .lbl{align-self:flex-start;font-size:20px;font-weight:800;padding:8px 14px;border-radius:12px;background:#1c2438;color:#aab6cc}
    .lbl:empty{display:none}
    .lbl.new{background:#FFB020;color:#1d1400}
    img{display:block;border-radius:18px;border:1px solid #233049}
  </style></head><body><div id="f">${f.title ? `<h1>${esc(f.title)}</h1>` : ''}<div class="row">${cells}</div></div></body></html>`)
  await page.waitForTimeout(200)
  const out = path.join(outDir, f.out)
  await page.locator('#f').screenshot({ path: out, type: 'jpeg', quality: 88 })
  console.log(out)
}
await b.close()
