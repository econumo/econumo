import { render, screen } from '@testing-library/react'
import { I18nextProvider, useTranslation } from 'react-i18next'
import i18n from './i18n'

function Probe() {
  const { t } = useTranslation()
  return (
    <>
      <span>{t('user.form.email.validation.required_field')}</span>
      <span>{t('settings.accounts.delete_account_modal.question', { account: 'Cash' })}</span>
    </>
  )
}

it('resolves dotted keys and single-brace interpolation', () => {
  render(
    <I18nextProvider i18n={i18n}>
      <Probe />
    </I18nextProvider>,
  )
  expect(screen.getByText('Required field')).toBeInTheDocument()
  expect(screen.getByText('Are you sure you want to delete the account “Cash”?')).toBeInTheDocument()
})

it('serves Russian translations after changeLanguage', async () => {
  await i18n.changeLanguage('ru')
  expect(i18n.t('common.button.ok.label')).not.toBe('OK')
  await i18n.changeLanguage('en')
  expect(i18n.t('common.button.ok.label')).toBe('OK')
})
