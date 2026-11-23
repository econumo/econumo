import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Switch } from '@/components/ui/switch'
import { useOnline } from '@/hooks/useOnline'

export interface AiSuggestionSwitches {
  suggestRules: boolean
  suggestFill: boolean
}

export function AiDialog({ open, onClose, enabled, onChange, suggestRules, suggestFill, onSuggestionsChange }: {
  open: boolean
  onClose: () => void
  enabled: boolean
  onChange: (enabled: boolean) => void
  suggestRules: boolean
  suggestFill: boolean
  onSuggestionsChange: (next: AiSuggestionSwitches) => void
}) {
  const { t } = useTranslation()
  const online = useOnline()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('user.page.settings.profile.ai.menu_item')}>
      <div className="flex flex-col gap-4">
        {/* laid out like the analytics dialog: the privacy promise leads */}
        <div className="flex flex-col gap-2 text-sm">
          <p>{t('user.page.settings.profile.ai.promise')}</p>
          <p className="text-muted-foreground">{t('user.page.settings.profile.ai.description')}</p>
        </div>
        <div className="flex items-center justify-between gap-4 rounded-lg bg-econumo-card px-4 py-3.5">
          <label htmlFor="profile-ai" className="text-sm">
            {t('user.page.settings.profile.ai.label')}
          </label>
          <Switch id="profile-ai" disabled={!online} checked={enabled} onCheckedChange={onChange} />
        </div>
        {enabled ? (
          <div className="flex flex-col gap-2">
            <div className="text-sm text-muted-foreground">{t('user.page.settings.profile.ai.imports_heading')}</div>
            <div className="flex items-center justify-between gap-4 rounded-lg bg-econumo-card px-4 py-3.5">
              <label htmlFor="profile-ai-suggest-rules" className="text-sm">
                {t('user.page.settings.profile.ai.suggest_rules_label')}
              </label>
              <Switch
                id="profile-ai-suggest-rules"
                disabled={!online}
                checked={suggestRules}
                onCheckedChange={(checked) => onSuggestionsChange({ suggestRules: checked, suggestFill })}
              />
            </div>
            <div className="flex items-center justify-between gap-4 rounded-lg bg-econumo-card px-4 py-3.5">
              <label htmlFor="profile-ai-suggest-fill" className="text-sm">
                {t('user.page.settings.profile.ai.suggest_fill_label')}
              </label>
              <Switch
                id="profile-ai-suggest-fill"
                disabled={!online}
                checked={suggestFill}
                onCheckedChange={(checked) => onSuggestionsChange({ suggestRules, suggestFill: checked })}
              />
            </div>
          </div>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}
