import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import type { AuthBootstrapResponse } from '@/lib/api'

export interface LandingCopy {
  heroTitle: string
  heroSubtitle: string
  note: string
  trafficValue: string
  trafficLabel: string
  devicesValue: string
  devicesLabel: string
  showTariffs: boolean
  showSteps: boolean
  showFeatures: boolean
  showFaq: boolean
}

function text(value: string | undefined): string {
  return (value ?? '').trim()
}

/** Публичные тексты лендинга из bootstrap. Пустая строка — на странице остаётся перевод. */
export function useLandingCopy(): LandingCopy {
  const { data } = useAuthBootstrap()
  const landing: NonNullable<AuthBootstrapResponse['landing']> = data?.landing ?? {}
  return {
    heroTitle: text(landing.hero_title),
    heroSubtitle: text(landing.hero_subtitle),
    note: text(landing.note),
    trafficValue: text(landing.stat_traffic_value),
    trafficLabel: text(landing.stat_traffic_label),
    devicesValue: text(landing.stat_devices_value),
    devicesLabel: text(landing.stat_devices_label),
    showTariffs: landing.show_tariffs !== false,
    showSteps: landing.show_steps !== false,
    showFeatures: landing.show_features !== false,
    showFaq: landing.show_faq !== false,
  }
}
