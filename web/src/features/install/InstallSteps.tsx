import { useInstallStepsContent } from './useInstallStepsContent'
import type { InstructionMode } from './installPrompt'

export function InstallSteps({ mode }: { mode: InstructionMode }) {
  const { steps, hint } = useInstallStepsContent(mode)
  return (
    <>
      <ol className="flex flex-col gap-3">
        {steps.map(([Icon, text], i) => (
          <li key={i} className="flex items-center gap-3 text-sm">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-econumo-card">
              <Icon className="size-5 text-econumo-link" />
            </span>
            <span>{text}</span>
          </li>
        ))}
      </ol>
      <p className="mt-4 text-sm text-muted-foreground">{hint}</p>
    </>
  )
}
