<template>
  <div class="budget-form-wrapper">
    <div class="budget-form-wrapper-control">
      <q-input
        class="form-input full-width"
        outlined
        autofocus
        :placeholder="$t('modules.budget.form.budget.name.placeholder')"
        v-model="name"
        :disable="!canUpdateName"
        :label="$t('modules.budget.form.budget.name.label')"
        lazy-rules
        :rules="[
          val => isNotEmpty(val.toString()) || $t('modules.budget.form.budget.name.validation.required_field'),
          val => isValidBudgetName(val.toString()) || $t('modules.budget.form.budget.name.validation.invalid_name')
        ]"
        maxlength="64">
      </q-input>
    </div>

    <div class="budget-form-wrapper-control">
      <currency-select
        v-model="selectedCurrency"
        outlined
        custom-class="form-select"
        :label="$t('modules.budget.form.budget_envelope.currency.label')"
        :rules="[val => isNotEmpty(val.toString()) || $t('modules.budget.form.budget_envelope.currency.validation.required_field')]"
      />
    </div>

    <h5 class="budget-form-wrapper-accounts-header">{{ $t('modules.budget.modal.budget_form.accounts') }}</h5>
    <div class="budget-form-wrapper-accounts">
      <div class="budget-form-wrapper-accounts-item-wrapper" v-for="account in accounts" :key="account.id">
        <q-item-section class="budget-form-wrapper-accounts-item-avatar" avatar>
          <q-icon class="budget-form-wrapper-accounts-item-avatar-icon" :name="account.icon" />
        </q-item-section>
        <div class="budget-form-wrapper-accounts-item-info">
          <q-item-section class="budget-form-wrapper-accounts-item-name">
            {{ account.name }}
          </q-item-section>
          <div class="budget-form-wrapper-accounts-item-info-container">
            <q-item-section class="budget-form-wrapper-accounts-item-balance">
              {{ moneyFormat(account.balance, account.currency.id, true, true) }}
            </q-item-section>
          </div>
        </div>
        <q-item-section class="budget-form-wrapper-accounts-item-toggle" side>
          <q-toggle :model-value="account.included" @click="includeToBudget(account.id, !account.included)" />
        </q-item-section>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useAccountsStore } from 'stores/accounts';
import { Id } from '../../modules/types';
import { useValidation } from '../../composables/useValidation';
import { includes, remove } from 'lodash-es';
import { useMoney } from '../../composables/useMoney';
import CurrencySelect from '../CurrencySelect.vue';
import { useCurrenciesStore } from 'stores/currencies';

const { isNotEmpty, isValidBudgetName } = useValidation();
const { moneyFormat } = useMoney();
const currenciesStore = useCurrenciesStore();

defineOptions({
  name: 'BudgetForm'
});

const props = withDefaults(defineProps<{
  name: string,
  excluded: Id[],
  canUpdateName: boolean,
  currencyId: Id,
}>(), {
  excluded: () => [],
  currencyId: undefined
});

const emit = defineEmits<{
  'update-name': [value: string],
  'update-excluded': [value: Id[]],
  'update-currency': [value: Id]
}>();

interface CurrencyOptionDto {
  label: string;
  value: Id;
}

const selectedCurrency = computed({
  get() {
    if (!props.currencyId) return null;
    const currency = currenciesStore.currenciesHash[props.currencyId];
    if (!currency) return null;
    return {
      label: currency.code,
      value: currency.id
    };
  },
  set(value: CurrencyOptionDto | null) {
    emit('update-currency', value?.value ?? '');
  }
});

const accountStore = useAccountsStore();
const name = computed({
  get() {
    return props.name
  },
  set(value) {
    emit('update-name', value);
  }
});

const accountsExcluded = ref<Id[]>([...(props.excluded || [])]);

// Keep accountsExcluded in sync with props
watch(() => props.excluded, (newValue) => {
  accountsExcluded.value = [...(newValue || [])];
}, { deep: true });

interface BudgetAccount {
  id: Id;
  name: string;
  icon: string;
  balance: number;
  currency: {
    symbol: string;
    [key: string]: any;
  };
  included: boolean;
}

const accounts = computed(() => {
  const result: Array<BudgetAccount> = [];
  if (!accountStore.ownAccounts) return result;
  
  accountStore.ownAccounts.forEach((account) => {
    const { id, name, icon, balance, currency } = account;
    const included = !includes(accountsExcluded.value, id);
    const item: BudgetAccount = { id, name, icon, balance, currency, included };
    result.push(item);
  });
  return result;
});

function includeToBudget(accountId: Id, include: boolean) {
  const excluded = includes(accountsExcluded.value, accountId);
  if (excluded && include) {
    remove(accountsExcluded.value, (item) => item === accountId);
  } else if (!excluded && !include) {
    accountsExcluded.value.push(accountId);
  }
  emit('update-excluded', accountsExcluded.value);
}
</script>

