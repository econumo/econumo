import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PromptDialog } from '@/components/PromptDialog'
import { isNotEmpty, isValidPayeeName } from '@/lib/validation'
import type { PayeeDto } from '@/api/dto/payee'
import { useUserData } from '@/features/user/queries'
import { ClassificationList } from './ClassificationList'
import { usePayees, useCreatePayee, useUpdatePayee, useArchivePayee, useUnarchivePayee, useDeletePayee, useOrderPayees } from './queries'

export function PayeesPage() {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: payees = [] } = usePayees()
  const createPayee = useCreatePayee()
  const updatePayee = useUpdatePayee()
  const archivePayee = useArchivePayee()
  const unarchivePayee = useUnarchivePayee()
  const deletePayee = useDeletePayee()
  const orderPayees = useOrderPayees()

  const [dialog, setDialog] = useState<{ open: boolean; payee: PayeeDto | null }>({ open: false, payee: null })
  const own = payees.filter((p) => !user || p.ownerUserId === user.id)

  const validate = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('classifications.payees.forms.payee.name.validation.required_field')
    }
    if (!isValidPayeeName(value)) {
      return t('classifications.payees.forms.payee.name.validation.invalid_name')
    }
    return null
  }

  return (
    <>
      <ClassificationList
        title={t('classifications.payees.pages.settings.header')}
        info={t('classifications.payees.pages.settings.info')}
        heading={t('classifications.payees.pages.settings.menu_item')}
        createLabel={t('classifications.payees.pages.settings.create_payee')}
        deleteTitle={t('classifications.payees.modals.delete.title')}
        items={own}
        storageKey="settings.payees.activeOnly"
        onCreate={() => setDialog({ open: true, payee: null })}
        onEdit={(payee) => setDialog({ open: true, payee })}
        onDelete={(id) => deletePayee.mutate(id)}
        onToggleArchive={(payee) => (payee.isArchived === 0 ? archivePayee.mutate(payee.id) : unarchivePayee.mutate(payee.id))}
        onOrder={(changes) => orderPayees.mutate(changes)}
      />
      <PromptDialog
        open={dialog.open}
        onClose={() => setDialog({ open: false, payee: null })}
        onSubmit={(name) => {
          if (dialog.payee) {
            // resolved by id, not by original name (Vue looks the record up by name — a latent bug)
            updatePayee.mutate({ id: dialog.payee.id, name }, { onSuccess: () => setDialog({ open: false, payee: null }) })
          } else {
            createPayee.mutate({ name, ownerUserId: user?.id }, { onSuccess: () => setDialog({ open: false, payee: null }) })
          }
        }}
        title={dialog.payee ? t('classifications.payees.modals.edit.header') : t('classifications.payees.modals.create.header')}
        inputLabel={t('classifications.payees.forms.payee.name.label')}
        initialValue={dialog.payee?.name ?? ''}
        validate={validate}
        submitLabel={dialog.payee ? t('common.button.update.label') : t('common.button.create.label')}
        cancelLabel={t('common.button.cancel.label')}
      />
    </>
  )
}
