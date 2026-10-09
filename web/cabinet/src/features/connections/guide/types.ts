/**
 * Формат translations/cabinet/app-config.json (шаблон subscription-page
 * Remnawave). Каждый владелец проекта кладёт свой файл со своими текстами,
 * поэтому всё, что приходит отсюда, рисуется как есть.
 */
export type Lang = 'ru' | 'en'
export type PlatformKey = string

export type LText = Partial<Record<Lang, string>> & Record<string, string>
export type LinkButton = { buttonLink: string; buttonText: LText }
export type GuideStep = { title?: LText; description?: LText; buttons?: LinkButton[] }

export type AppGuide = {
  id: string
  name: string
  isFeatured?: boolean
  urlScheme?: string
  installationStep: GuideStep
  addSubscriptionStep: GuideStep
  additionalAfterAddSubscriptionStep?: GuideStep
  /** Необязателен: без него гид показывает два шага. */
  connectAndUseStep?: GuideStep
  isNeedBase64Encoding?: boolean
}

export type AppConfig = {
  config: { branding?: { name?: string; logoUrl?: string; supportUrl?: string } }
  platforms: Partial<Record<PlatformKey, AppGuide[]>>
}

export function pickText(text: LText | undefined, lang: Lang): string {
  if (!text) return ''
  return text[lang] || text.ru || text.en || Object.values(text)[0] || ''
}
