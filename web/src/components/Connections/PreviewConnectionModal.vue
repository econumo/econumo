<template>
  <q-dialog class="settings-connections-modal" :model-value="isOpened" @update:model-value="$emit('update:isOpened', $event)" @hide="onHide" :position="$q.screen.gt.md ? 'standard' : 'bottom'">
    <q-card class="settings-connections-modal-card" v-if="connection">
        <q-card-section class="settings-connections-modal-section">
        <div class="settings-connections-modal-card-container">
          <div class="settings-connections-modal-card-label">{{ $t('modules.connections.modals.preview_connection.budgets') }}</div>
          <div class="settings-connections-modal-card-user">
            <q-avatar class="settings-connections-modal-card-user-avatar">
              <img class="settings-connections-modal-card-user-avatar-img" :src="connection.user.avatar">
            </q-avatar>
            <div class="settings-connections-modal-card-user-name">
              {{ connection.user.name }}
            </div>
          </div>
        </div>

        <div class="settings-connections-modal-card-item" v-if="connection.sharedBudgets.length === 0">
            <div class="settings-connections-modal-card-item-label">{{ $t('modules.connections.modals.preview_connection.budgets_empty') }}</div>
            <q-list class="settings-connections-modal-card-item-list">
                <q-item v-ripple class="settings-connections-modal-card-item-item">
                    <q-item-section class="settings-connections-modal-card-item-item-name-block">
                        {{ $t('modules.connections.modals.preview_connection.budgets_empty') }}
                    </q-item-section>
                </q-item>
            </q-list>
        </div>
        <div class="settings-connections-modal-card-item" v-else>
          <div class="settings-connections-modal-card-item-label">{{ $t('modules.connections.modals.preview_connection.budgets') }}</div>
          <div class="settings-connections-modal-card-item-hint">{{ $t('modules.connections.modals.preview_connection.tap_to_manage') }}</div>
          <q-list class="settings-connections-modal-card-item-list">
            <q-item v-for="budget in connection.sharedBudgets" v-bind:key="budget.id" clickable v-ripple class="settings-connections-modal-card-item-item" @click="onBudgetClick(budget)">
              <q-item-section class="settings-connections-modal-card-item-item-avatar" avatar>
                <q-icon name="menu_book" />
              </q-item-section>
              <q-item-section class="settings-connections-modal-card-item-item-name-block">
                <div class="settings-connections-modal-card-item-item-name-row">
                  <span class="settings-connections-modal-card-item-item-name">
                    {{ budget.name }}
                  </span>
                  <span v-if="isBudgetOwnedByCurrentUser(budget)" class="settings-connections-modal-card-item-item-owner-hint settings-connections-modal-card-item-item-badge">{{ $t('modules.connections.modals.preview_connection.your_budget') }}</span>
                  <span v-if="!isBudgetOwnedByCurrentUser(budget)" class="settings-connections-modal-card-item-item-shared-hint settings-connections-modal-card-item-item-badge">{{ $t('modules.connections.modals.preview_connection.shared_with_you') }}</span>
                </div>
                <span class="settings-connections-modal-card-item-item-role">{{ $t('modules.connections.budgets.roles.' + budget.role) }}</span>
              </q-item-section>

              <q-item-section side class="settings-connections-modal-card-item-item-shared">
                <q-avatar class="settings-connections-modal-card-item-item-shared-avatar">
                  <img :src="getBudgetOwnerAvatar(budget)" width="30" height="30" :alt="isBudgetOwnedByCurrentUser(budget) ? currentUserName : connection.user.name" :title="isBudgetOwnedByCurrentUser(budget) ? currentUserName : connection.user.name">
                </q-avatar>
              </q-item-section>
            </q-item>
          </q-list>
        </div>
      </q-card-section>

      <q-card-section class="settings-connections-modal-section">
        <div class="settings-connections-modal-card-container">
          <div class="settings-connections-modal-card-label">{{ $t('modules.connections.modals.preview_connection.accounts') }}</div>
        </div>

        <div class="settings-connections-modal-card-item settings-connections-modal-card-item-last" v-if="connection.sharedAccounts.length === 0">
            <div class="settings-connections-modal-card-item-label">{{ $t('modules.connections.modals.preview_connection.accounts_empty') }}</div>
            <q-list class="settings-connections-modal-card-item-list">
                <q-item v-ripple class="settings-connections-modal-card-item-item">
                    <q-item-section class="settings-connections-modal-card-item-item-name-block">
                        {{ $t('modules.connections.modals.preview_connection.accounts_empty') }}
                    </q-item-section>
                </q-item>
            </q-list>
        </div>
        <div class="settings-connections-modal-card-item  settings-connections-modal-card-item-last" v-else>
          <div class="settings-connections-modal-card-item-label">{{ $t('modules.connections.modals.preview_connection.accounts') }}</div>
          <div class="settings-connections-modal-card-item-hint">{{ $t('modules.connections.modals.preview_connection.tap_to_manage') }}</div>
          <q-list class="settings-connections-modal-card-item-list">
            <q-item v-for="account in connection.sharedAccounts" v-bind:key="account.id" clickable v-ripple class="settings-connections-modal-card-item-item" @click="onAccountClick(account)">
              <q-item-section class="settings-connections-modal-card-item-item-avatar" avatar>
                <q-icon :name="account.icon" />
              </q-item-section>
              <q-item-section class="settings-connections-modal-card-item-item-name-block">
                <div class="settings-connections-modal-card-item-item-name-row">
                  <span class="settings-connections-modal-card-item-item-name">
                    {{ account.name }}
                  </span>
                  <span v-if="isAccountOwnedByCurrentUser(account)" class="settings-connections-modal-card-item-item-owner-hint settings-connections-modal-card-item-item-badge">{{ $t('modules.connections.modals.preview_connection.your_account') }}</span>
                  <span v-if="!isAccountOwnedByCurrentUser(account)" class="settings-connections-modal-card-item-item-shared-hint settings-connections-modal-card-item-item-badge">{{ $t('modules.connections.modals.preview_connection.shared_with_you') }}</span>
                </div>
                <span class="settings-connections-modal-card-item-item-role">{{ $t('modules.connections.accounts.roles.' + account.role) }}</span>
              </q-item-section>

              <q-item-section side class="settings-connections-modal-card-item-item-shared">
                <q-avatar class="settings-connections-modal-card-item-item-shared-avatar">
                  <img :src="getAccountOwnerAvatar(account)" width="30" height="30" :alt="isAccountOwnedByCurrentUser(account) ? currentUserName : connection.user.name" :title="isAccountOwnedByCurrentUser(account) ? currentUserName : connection.user.name" />
                </q-avatar>
              </q-item-section>
            </q-item>
          </q-list>
        </div>
      </q-card-section>
      <q-card-actions class="responsive-modal-actions">
        <q-btn
          class="econumo-btn -medium -grey responsive-modal-actions-button"
          flat
          :label="$t('elements.button.delete.label')"
          icon="delete"
          @click="onDelete"
          v-if="allowChanges" />

        <q-btn
          class="econumo-btn -medium -grey responsive-modal-actions-button"
          :label="$t('elements.button.ok.label')"
          flat
          v-close-popup />
      </q-card-actions>
    </q-card>
  </q-dialog>

  <DeclineAccountAccessModal
    v-if="selectedAccount && selectedAccountOwner && !isAccountOwnedByCurrentUser(selectedAccount)"
    :is-opened="isDeclineModalOpened"
    @update:is-opened="isDeclineModalOpened = $event"
    :owner="selectedAccountOwner"
    :account-id="selectedAccount.id"
    :account-name="selectedAccount.name"
    @decline="onDeclineAccess"
    @cancel="onCancelDecline"
  />

  <AccessLevelDialogModal
    v-if="selectedAccount && isAccountOwnedByCurrentUser(selectedAccount)"
    :user="connection.user"
    :item-id="selectedAccount.id"
    :role="selectedAccount.role"
    @allow="onAllowAccess"
    @revoke="onRevokeAccess"
    @cancel="onCancelAccessLevel"
  />

  <BudgetAccessLevelModal
    v-if="selectedBudget && isBudgetOwnedByCurrentUser(selectedBudget)"
    :budget-id="selectedBudget.id"
    :user="connection.user"
    :role="selectedBudget.role"
    @share="onShareBudgetAccess"
    @revoke="onRevokeBudgetAccess"
    @cancel="onCancelBudgetAccessLevel"
  />

  <DeclineBudgetAccessModal
    v-if="selectedBudget && selectedBudgetOwner && !isBudgetOwnedByCurrentUser(selectedBudget)"
    :is-opened="isDeclineBudgetModalOpened"
    @update:is-opened="isDeclineBudgetModalOpened = $event"
    :owner="selectedBudgetOwner"
    :budget-id="selectedBudget.id"
    :budget-name="selectedBudget.name"
    @decline="onDeclineBudgetAccess"
    @cancel="onCancelDeclineBudget"
  />
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useQuasar } from 'quasar';
import type { SharedBudget, SharedAccount } from '../../stores/connections';
import { useAvatar } from '../../composables/useAvatar';
import { useUsersStore } from '../../stores/users';
import { useAccountsStore } from '../../stores/accounts';
import { useBudgetsStore } from '../../stores/budgets';
import DeclineAccountAccessModal from './DeclineAccountAccessModal.vue';
import DeclineBudgetAccessModal from './DeclineBudgetAccessModal.vue';
import AccessLevelDialogModal from '../AccessLevelDialogModal.vue';
import BudgetAccessLevelModal from '../Budget/BudgetAccessLevelModal.vue';

defineOptions({
  name: 'PreviewConnectionModal'
});

interface User {
  id: string;
  name: string;
  avatar: string;
}

interface Connection {
  user: User;
  sharedAccounts: SharedAccount[];
  sharedBudgets: SharedBudget[];
}

interface Props {
  isOpened: boolean;
  connection: Connection;
  allowChanges: boolean;
}

const props = defineProps<Props>();
const emit = defineEmits<{
  'update:isOpened': [value: boolean];
  'hide': [];
  'delete': [userId: string];
  'decline-account': [accountId: string];
  'decline-budget': [budgetId: string];
  'allow-account': [userId: string, accountId: string, role: string];
  'revoke-account': [userId: string, accountId: string];
  'share-budget': [userId: string, budgetId: string, role: string];
  'revoke-budget': [userId: string, budgetId: string];
}>();

const $q = useQuasar();
const usersStore = useUsersStore();
const accountsStore = useAccountsStore();
const budgetsStore = useBudgetsStore();

const { avatarUrl } = useAvatar();

const isDeclineModalOpened = ref(false);
const isAccessLevelModalOpened = ref(false);
const selectedAccount = ref<SharedAccount | null>(null);
const selectedAccountOwner = ref<User | null>(null);
const isDeclineBudgetModalOpened = ref(false);
const selectedBudget = ref<SharedBudget | null>(null);
const selectedBudgetOwner = ref<User | null>(null);

const currentUserId = computed(() => usersStore.userId);
const currentUserAvatar = computed(() => usersStore.userAvatar);
const currentUserName = computed(() => usersStore.userName);

const isAccountOwnedByCurrentUser = (account: SharedAccount) => {
  // Look up the full account to check ownership
  const fullAccount = accountsStore.accounts.find(a => a.id === account.id);
  if (!fullAccount) {
    return false;
  }
  return fullAccount.owner.id === currentUserId.value;
};

const getAccountOwner = (account: SharedAccount): User => {
  const fullAccount = accountsStore.accounts.find(a => a.id === account.id);
  if (!fullAccount) {
    return props.connection.user; // Fallback
  }
  return {
    id: fullAccount.owner.id,
    name: fullAccount.owner.name,
    avatar: fullAccount.owner.avatar
  };
};

const getAccountOwnerAvatar = (account: SharedAccount): string => {
  if (isAccountOwnedByCurrentUser(account)) {
    return avatarUrl(currentUserAvatar.value || '', 30);
  }
  return avatarUrl(props.connection.user.avatar, 30);
};

const isBudgetOwnedByCurrentUser = (budget: SharedBudget) => {
  // Look up the full budget to check ownership
  const fullBudget = budgetsStore.budgets.find(b => b.id === budget.id);
  if (!fullBudget) {
    return false;
  }
  return fullBudget.ownerUserId === currentUserId.value;
};

const getBudgetOwner = (budget: SharedBudget): User => {
  const fullBudget = budgetsStore.budgets.find(b => b.id === budget.id);
  if (!fullBudget) {
    return props.connection.user; // Fallback
  }
  // For budgets, we need to construct the owner from the ownerUserId
  // If it's the current user, use current user info
  if (fullBudget.ownerUserId === currentUserId.value) {
    return {
      id: currentUserId.value,
      name: currentUserName.value,
      avatar: currentUserAvatar.value || ''
    };
  }
  // Otherwise, it's the connected user
  return props.connection.user;
};

const getBudgetOwnerAvatar = (budget: SharedBudget): string => {
  if (isBudgetOwnedByCurrentUser(budget)) {
    return avatarUrl(currentUserAvatar.value || '', 30);
  }
  return avatarUrl(props.connection.user.avatar, 30);
};

const onHide = () => {
  emit('hide');
  emit('update:isOpened', false);
};

const onDelete = () => {
  emit('delete', props.connection.user.id);
};

const onAccountClick = (account: SharedAccount) => {
  selectedAccount.value = account;
  selectedAccountOwner.value = getAccountOwner(account);

  if (isAccountOwnedByCurrentUser(account)) {
    // Account I own and share with the connected user - show access level dialog
    isAccessLevelModalOpened.value = true;
  } else {
    // Account shared with me - show decline dialog
    isDeclineModalOpened.value = true;
  }
};

const onDeclineAccess = (accountId: string) => {
  isDeclineModalOpened.value = false;
  selectedAccount.value = null;
  selectedAccountOwner.value = null;
  emit('decline-account', accountId);
};

const onCancelDecline = () => {
  isDeclineModalOpened.value = false;
  selectedAccount.value = null;
  selectedAccountOwner.value = null;
};

const onAllowAccess = (userId: string, accountId: string, role: string) => {
  isAccessLevelModalOpened.value = false;
  selectedAccount.value = null;
  selectedAccountOwner.value = null;
  emit('allow-account', userId, accountId, role);
};

const onRevokeAccess = (userId: string, accountId: string) => {
  isAccessLevelModalOpened.value = false;
  selectedAccount.value = null;
  selectedAccountOwner.value = null;
  emit('revoke-account', userId, accountId);
};

const onCancelAccessLevel = () => {
  isAccessLevelModalOpened.value = false;
  selectedAccount.value = null;
  selectedAccountOwner.value = null;
};

const onBudgetClick = (budget: SharedBudget) => {
  selectedBudget.value = budget;
  selectedBudgetOwner.value = getBudgetOwner(budget);

  if (isBudgetOwnedByCurrentUser(budget)) {
    // Budget I own and share with the connected user - show access level dialog
    // The modal will appear via the v-if condition in the template
  } else {
    // Budget shared with me - show decline dialog
    isDeclineBudgetModalOpened.value = true;
  }
};

const onShareBudgetAccess = (budgetId: string, userId: string, role: string) => {
  selectedBudget.value = null;
  emit('share-budget', userId, budgetId, role);
};

const onRevokeBudgetAccess = (budgetId: string, userId: string) => {
  selectedBudget.value = null;
  emit('revoke-budget', userId, budgetId);
};

const onCancelBudgetAccessLevel = () => {
  selectedBudget.value = null;
};

const onDeclineBudgetAccess = (budgetId: string) => {
  isDeclineBudgetModalOpened.value = false;
  selectedBudget.value = null;
  selectedBudgetOwner.value = null;
  emit('decline-budget', budgetId);
};

const onCancelDeclineBudget = () => {
  isDeclineBudgetModalOpened.value = false;
  selectedBudget.value = null;
  selectedBudgetOwner.value = null;
};
</script>

<style scoped>
.settings-connections-modal-card-item-item-name-block {
  min-width: 0;
}

.settings-connections-modal-card-item-item-name-row {
  align-items: center;
  display: flex;
  flex-wrap: nowrap;
  gap: 6px;
  min-width: 0;
  width: 100%;
}

.settings-connections-modal-card-item-item-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-connections-modal-card-item-item-badge {
  flex: 0 0 auto;
  margin-right: 16px;
  opacity: 0.5;
  white-space: nowrap;
}

.settings-connections-modal-card-item-item-shared {
  align-items: center;
  display: flex;
}
</style>
