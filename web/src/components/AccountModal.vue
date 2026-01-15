<template>
  <teleport to="body">
    <q-dialog class="responsive-modal" v-model="isModalOpened" no-backdrop-dismiss>
      <q-card class="responsive-modal-card">
        <q-form
          ref="accountForm"
          @submit="submit"
          @reset="closeModal"
          class="responsive-modal-form"
        >
          <q-card-section>
            <div class="settings-toolbar-mobile">
              <div>
                <h4 class="settings-toolbar-mobile-title" v-if="isAccountModalCreation">{{ $t('modals.account.create_form.header') }}</h4>
                <h4 class="settings-toolbar-mobile-title" v-else>{{ $t('modals.account.update_form.header') }}</h4>
              </div>
            </div>

            <div class="responsive-modal-header" v-if="isAccountModalCreation">{{ $t('modals.account.create_form.header') }}</div>
            <div class="responsive-modal-header" v-else>{{ $t('modals.account.update_form.header') }}</div>

            <div class="responsive-modal-control">
              <q-input
                class="form-input full-width"
                outlined
                :placeholder="$t('elements.form.account.name.placeholder')"
                :autofocus="isAccountModalCreation"
                v-model="name"
                :label="$t('elements.form.account.name.label')"
                lazy-rules
                :rules="[
                  (val: string) => isNotEmpty(val.toString()) || $t('elements.validation.required_field'),
                  (val: string) => isValidAccountName(val) || $t('elements.form.account.name.validation.invalid_name')
                ]"
                maxlength="64">
                <template v-slot:before>
                  <div class="responsive-modal-control-icon">
                    <q-icon class="responsive-modal-control-icon-img" :name="icon"/>
                  </div>
                </template>
              </q-input>
            </div>
            <div class="responsive-modal-control">
              <calculator-input
                class="responsive-modal-control-input"
                v-model="balance"
                :autofocus="!isAccountModalCreation"
                lazy-rules
                :rules="[
                  (val: string | number) => isNotEmpty(val.toString()) || $t('elements.validation.required_field'),
                  (val: string | number) => isValidNumber(val.toString()) || $t('elements.validation.invalid_number'),
                  (val: string | number) => isValidDecimalNumber(val.toString()) || $t('elements.validation.invalid_decimal_number'),
                  (val: string | number) => isValidFormula(val.toString()) || $t('elements.validation.invalid_formula')
                ]"
                :label="$t('elements.form.account.balance.label')"
                :placeholder="$t('elements.form.account.balance.placeholder')" />
            </div>
            <div class="responsive-modal-control">
              <currency-select
                v-model="currency"
                :outlined="true"
                custom-class="form-select"
                :label="$t('elements.form.account.currency.label')"
              />
            </div>
          </q-card-section>

          <q-card-section class="responsive-modal-section-icon">
            <responsive-modal-icons 
              class="responsive-modal-icons-container" 
              :header="$t('modals.account.form.icon.label')" 
              :icon="icon" 
              @update-icon="value => icon = value" 
            />
          </q-card-section>

          <q-card-actions class="responsive-modal-actions">
            <q-btn class="econumo-btn -medium -grey responsive-modal-actions-button" flat :label="$t('elements.button.cancel.label')" v-close-popup @close="closeModal"/>
            <q-btn class="econumo-btn -medium -magenta responsive-modal-actions-button" flat :label="$t('elements.button.add.label')" type="submit" v-if="isAccountModalCreation"/>
            <q-btn class="econumo-btn -medium -magenta responsive-modal-actions-button" flat :label="$t('elements.button.update.label')" type="submit" v-else/>
          </q-card-actions>
        </q-form>
      </q-card>
    </q-dialog>
  </teleport>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useValidation } from '../composables/useValidation'
import { useAccountsStore } from '../stores/accounts'
import { useAccountModalStore } from '../stores/account-modal'
import { useCurrenciesStore } from '../stores/currencies'
import { useConnectionsStore } from '../stores/connections'
import ResponsiveModalIcons from '../components/ResponsiveModal/Icons.vue'
import CurrencySelect from '../components/CurrencySelect.vue'
import CalculatorInput from './Calculator/CalculatorInput.vue'
import { Icon, Id } from '@shared/types'
import { AccountCreateDto, AccountUpdateDto } from '../modules/api/v1/dto/account.dto'
import { CurrencyDto } from '@shared/dto/currency.dto'
import _ from 'lodash'

interface AccountForm extends HTMLFormElement {
  resetValidation: () => void
}

interface CurrencyOptionDto {
  label: string
  value: Id
}

const accountForm = ref<AccountForm | null>(null)
const iconScreen = ref('icons-0')

const accountsStore = useAccountsStore()
const accountModalStore = useAccountModalStore()
const currenciesStore = useCurrenciesStore()
const connectionsStore = useConnectionsStore()
const { isNotEmpty, isValidNumber, isValidDecimalNumber, isValidFormula, isValidAccountName } = useValidation()

const isModalOpened = computed({
  get: () => accountModalStore.isAccountModalOpened,
  set: (value: boolean) => {
    if (value) {
      accountModalStore.openAccountModal({})
    } else {
      accountModalStore.closeAccountModal()
    }
  }
})

const name = computed({
  get: () => accountModalStore.accountModalName ?? '',
  set: (value: string) => accountModalStore.changeAccountModalName(value)
})

const balance = computed({
  get: () => accountModalStore.accountModalBalance ?? 0,
  set: (value: number) => accountModalStore.changeAccountModalBalance(value)
})

const icon = computed({
  get: () => accountModalStore.accountModalIcon ?? '',
  set: (value: Icon) => accountModalStore.changeAccountModalIcon(value)
})

const currency = computed({
  get: () => {
    const currencyId = accountModalStore.accountModalCurrencyId
    if (!currencyId) return null
    
    const currency = _.find(currenciesStore.currencies, { id: currencyId }) as CurrencyDto | undefined
    if (!currency) {
      return null
    }
    return {
      label: currency.code,
      value: currency.id
    } as CurrencyOptionDto
  },
  set: (item: CurrencyOptionDto | null) => {
    accountModalStore.changeAccountModalCurrencyId(item?.value ?? '')
  }
})

const isAccountModalCreation = computed(() => accountModalStore.isAccountModalCreation)

const submit = () => {
  if (isAccountModalCreation.value) {
    const form: AccountCreateDto = {
      id: accountModalStore.accountModalId ?? '',
      name: name.value,
      balance: balance.value,
      icon: icon.value,
      folderId: accountModalStore.accountModalFolderId ?? null,
      currencyId: currency.value?.value ?? ''
    }
    accountsStore.createAccount(form).then(() => {
      closeModal()
    })
  } else {
    const form: AccountUpdateDto = {
      id: accountModalStore.accountModalId ?? '',
      name: name.value,
      balance: balance.value,
      icon: icon.value,
      currencyId: currency.value?.value ?? '',
      updatedAt: new Date().toISOString()
    }
    accountsStore.updateAccount(form).then(() => {
      closeModal()
    })
  }
}

const closeModal = () => {
  if (accountForm.value) {
    accountForm.value.resetValidation()
  }
  accountModalStore.closeAccountModal()
}

const deleteAccount = () => {
  if (accountModalStore.accountModalId) {
    accountsStore.deleteAccount(accountModalStore.accountModalId).then(() => {
      closeModal()
    })
  }
}
</script>
