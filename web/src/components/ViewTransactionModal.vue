<template>
  <q-dialog class="account-preview-modal" :model-value="true" :position="position" @hide="cancel">
    <q-card class="account-preview-modal-card">
      <q-card-section class="account-preview-modal-section">
        <div class="account-preview-modal-head">
          <div class="account-preview-modal-head-title">{{ $t('pages.account.preview_transaction_modal.header') }}</div>
          <div class="account-preview-modal-head-badge">
            <q-badge
              class="account-preview-modal-head-badge-label"
              :class="(isTransfer(transaction) ? '-transfer' : '')"
            >
              {{ $t('pages.account.preview_transaction_modal.type.' + transaction.type) }}
            </q-badge>
          </div>
        </div>

        <div class="account-preview-modal-account" v-if="accountToShow || isTransfer(transaction)">
          <div class="account-preview-modal-account-title" v-if="isTransfer(transaction)">{{ $t('pages.account.preview_transaction_modal.sender.label') }}</div>
          <q-item class="account-preview-modal-account-item">
            <q-item-section class="account-preview-modal-account-item-avatar" avatar>
              <q-icon class="account-preview-modal-account-item-avatar-icon" :name="accountIcon" />
            </q-item-section>
            <div class="account-preview-modal-account-item-box">
              <q-item-section class="account-preview-modal-account-item-box-name econumo-truncate" :title="accountLabel">{{ accountLabel }}</q-item-section>
              <q-item-section class="account-preview-modal-account-item-box-balance" v-if="senderCurrencyId !== null && senderAmount !== null">{{ moneyFormat(senderAmount, senderCurrencyId, true, false) }}</q-item-section>
            </div>
          </q-item>
        </div>
      </q-card-section>

      <q-card-section class="account-preview-modal-section -info">
        <div v-if="accountRecipientToShow || (isTransfer(transaction) && transaction.accountRecipientId)" class="account-preview-modal-account -recipient">
          <div class="account-preview-modal-account-title" v-if="isTransfer(transaction)">{{ $t('pages.account.preview_transaction_modal.recipient.label') }}</div>
          <q-item class="account-preview-modal-account-item">
            <q-item-section class="account-preview-modal-account-item-avatar" avatar>
              <q-icon class="account-preview-modal-account-item-avatar-icon" :name="accountRecipientIcon" />
            </q-item-section>
            <div class="account-preview-modal-account-item-box">
              <q-item-section class="account-preview-modal-account-item-box-name econumo-truncate" :title="accountRecipientLabel">{{ accountRecipientLabel }}</q-item-section>
              <q-item-section class="account-preview-modal-account-item-box-balance" v-if="recipientCurrencyId !== null && recipientAmount !== null">{{ moneyFormat(recipientAmount, recipientCurrencyId, true, false) }}</q-item-section>
            </div>
          </q-item>
        </div>

        <div class="account-preview-modal-account-info">
          <div class="account-preview-modal-account-info-item" v-if="transaction.category">
            <div class="account-preview-modal-account-info-item-label">{{ $t('pages.account.preview_transaction_modal.category.label') }}</div>
            <div class="account-preview-modal-account-info-item-content econumo-truncate" :title="transaction.category?.name || ''">{{ transaction.category.name }}</div>
          </div>
          <div class="account-preview-modal-account-info-item" v-if="transaction.description">
            <div class="account-preview-modal-account-info-item-label">{{ $t('pages.account.preview_transaction_modal.description.label') }}</div>
            <div class="account-preview-modal-account-info-item-content">{{ transaction.description || "–" }}</div>
          </div>
          <div v-if="!isTransfer(transaction) && transaction.payee">
            <div class="account-preview-modal-account-info-item">
              <div class="account-preview-modal-account-info-item-label">{{ $t('pages.account.preview_transaction_modal.payee.label') }}</div>
              <div class="account-preview-modal-account-info-item-content econumo-truncate" :title="transaction.payee?.name || '–'">{{ transaction.payee?.name || "–" }}</div>
            </div>
          </div>
          <div v-if="!isTransfer(transaction) && transaction.tag">
            <div class="account-preview-modal-account-info-item">
              <div class="account-preview-modal-account-info-item-label">{{ $t('pages.account.preview_transaction_modal.tags.label') }}</div>
              <div class="account-preview-modal-account-info-item-content -tags econumo-truncate" :title="transaction.tag?.name || '–'">{{ transaction.tag?.name || "–" }}</div>
            </div>
          </div>
          <div
            class="account-preview-modal-account-info-item"
            v-if="(accountToShow?.sharedAccess?.length || 0) > 0 || (isTransfer(transaction) && (accountRecipientToShow?.sharedAccess?.length || 0) > 0)"
          >
            <div class="account-preview-modal-account-info-item-label">{{ $t('pages.account.preview_transaction_modal.author.label') }}</div>
            <div class="account-preview-modal-account-info-item-content">
              <q-chip>
                <q-avatar>
                  <img :src="avatarUrl(transaction.author.avatar, 20)" width="20" height="20">
                </q-avatar>
                {{ transaction.author.name }}
              </q-chip>
            </div>
          </div>
          <div class="account-preview-modal-account-info-item">
            <div class="account-preview-modal-account-info-item-label">{{ $t('pages.account.preview_transaction_modal.created_at.label') }}</div>
            <div class="account-preview-modal-account-info-item-content">{{ transaction.date || "–" }}</div>
          </div>
        </div>
      </q-card-section>

      <q-card-actions class="preview-modal-account-info-access-actions">
        <q-btn class="econumo-btn -large -grey preview-modal-account-info-access-actions-btn" :title="$t('elements.button.delete.label')" flat icon="delete" @click="openDeleteModal(transaction.id)" :disable="!canChangeTransaction || (transaction.type === 'transfer' && (!transaction.account || !transaction.accountRecipient))" />
        <q-btn class="econumo-btn -large -grey preview-modal-account-info-access-actions-btn -wide" :label="$t('elements.button.edit.label')" flat @click="openUpdateModal(transaction.id)" :disable="!canChangeTransaction || (transaction.type === 'transfer' && (!transaction.account || !transaction.accountRecipient))" />
        <q-btn class="econumo-btn -large -grey preview-modal-account-info-access-actions-btn" :title="$t('elements.button.cancel.label')" flat icon="expand_more" v-close-popup @click="cancel" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useQuasar } from 'quasar';
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { AccountDto } from '@shared/dto/account.dto';
import { useAvatar } from '../composables/useAvatar';
import { useMoney } from '../composables/useMoney';
import { Id } from '../modules/types';
import { TransactionDto } from '@shared/dto/transaction.dto';
import { CategoryDto } from '@shared/dto/category.dto';
import { PayeeDto } from '@shared/dto/payee.dto';
import { TagDto } from '@shared/dto/tag.dto';

const $q = useQuasar();
const { t } = useI18n();

interface Transaction extends TransactionDto {
  category: CategoryDto;
  payee: PayeeDto;
  tag: TagDto;
  account?: AccountDto | null;
  accountRecipient?: AccountDto | null;
}

const { avatarUrl } = useAvatar();
const { moneyFormat } = useMoney();

const props = defineProps<{
  position: 'standard' | 'top' | 'right' | 'bottom' | 'left' | undefined;
  transaction: Transaction;
  account?: AccountDto | null;
  accountRecipient?: AccountDto | null;
  canChangeTransaction: boolean;
}>();

const emit = defineEmits<{
  (e: 'cancel'): void;
  (e: 'update', id: Id): void;
  (e: 'delete', id: Id): void;
}>();

const cancel = () => {
  emit('cancel');
};

const openUpdateModal = (id: Id) => {
  emit('update', id);
};

const openDeleteModal = (id: Id) => {
  emit('delete', id);
};

const isTransfer = (transaction: Transaction) => {
  return transaction.type === 'transfer';
};

const accountToShow = computed(() => props.account ?? props.transaction.account ?? null);
const accountRecipientToShow = computed(() => props.accountRecipient ?? props.transaction.accountRecipient ?? null);
const accountLabel = computed(() => accountToShow.value?.name || t('elements.account.name_hidden'));
const accountRecipientLabel = computed(() => accountRecipientToShow.value?.name || t('elements.account.name_hidden'));
const accountIcon = computed(() => accountToShow.value?.icon || 'lock');
const accountRecipientIcon = computed(() => accountRecipientToShow.value?.icon || 'lock');
const senderCurrencyId = computed(() => {
  if (accountToShow.value?.currency?.id) {
    return accountToShow.value.currency.id;
  }
  if (isTransfer(props.transaction)) {
    return accountRecipientToShow.value?.currency?.id || props.transaction.currencyId || null;
  }
  return props.transaction.currencyId || null;
});

const recipientCurrencyId = computed(() => {
  if (accountRecipientToShow.value?.currency?.id) {
    return accountRecipientToShow.value.currency.id;
  }
  if (isTransfer(props.transaction)) {
    return accountToShow.value?.currency?.id || props.transaction.currencyId || null;
  }
  return props.transaction.currencyId || null;
});

const senderAmount = computed(() => {
  if (accountToShow.value || !isTransfer(props.transaction)) {
    return props.transaction.amount;
  }
  return props.transaction.amountRecipient ?? props.transaction.amount;
});

const recipientAmount = computed(() => {
  if (accountRecipientToShow.value || !isTransfer(props.transaction)) {
    return props.transaction.amountRecipient ?? null;
  }
  return props.transaction.amount;
});
</script>
