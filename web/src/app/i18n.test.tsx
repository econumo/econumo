import { render, screen } from '@testing-library/react'
import { I18nextProvider, useTranslation } from 'react-i18next'
import i18n from './i18n'

function Probe() {
  const { t } = useTranslation()
  return (
    <>
      <span>{t('modules.user.form.user.email.validation.required_field')}</span>
      <span>{t('pages.settings.accounts.delete_account_modal.question', { account: 'Cash' })}</span>
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
