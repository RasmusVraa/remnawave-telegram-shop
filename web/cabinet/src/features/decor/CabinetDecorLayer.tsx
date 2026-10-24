import { decorEffectForTheme, type DecorThemeId } from './decorThemes'
import { isDecorColorOnlyTheme } from './decorThemeAdmin'
import { useCabinetDecorTheme } from './useCabinetDecorTheme'
import { CabinetDecorScene } from './CabinetDecorScene'
import {
  AuroraEffect,
  BubblesEffect,
  DotsEffect,
  EmbersEffect,
  HeartsEffect,
  MatrixEffect,
  PumpkinsEffect,
  SnowEffect,
  SparksEffect,
  SpringEffect,
  SunraysEffect,
} from './DecorEffects'

/**
 * Слой частиц между фоном (cabinet-shell-gradient) и контентом.
 * Монтировать внутри layout сразу после градиента.
 */
export function CabinetDecorLayer({ theme: themeProp }: { theme?: DecorThemeId } = {}) {
  const fromHook = useCabinetDecorTheme()
  const theme = themeProp ?? fromHook
  const effect = decorEffectForTheme(theme)

  if (theme === 'off' || isDecorColorOnlyTheme(theme)) return null

  let fx = null
  if (effect) {
    switch (effect) {
      case 'snow':
        fx = <SnowEffect />
        break
      case 'sunrays':
        fx = <SunraysEffect />
        break
      case 'pumpkins':
        fx = <PumpkinsEffect />
        break
      case 'hearts':
        fx = <HeartsEffect />
        break
      case 'petals':
        fx = <SpringEffect />
        break
      case 'sparks':
        fx = <SparksEffect />
        break
      case 'aurora':
        fx = <AuroraEffect />
        break
      case 'bubbles':
        fx = <BubblesEffect />
        break
      case 'matrix':
        fx = <MatrixEffect />
        break
      case 'embers':
        fx = <EmbersEffect />
        break
      case 'dots':
        fx = <DotsEffect />
        break
      default:
        break
    }
  }

  return (
    <div className="cabinet-decor-layer" aria-hidden>
      <CabinetDecorScene theme={theme} />
      {fx}
    </div>
  )
}
