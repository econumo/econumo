<template>
  <q-page class="settings-classification">
    <div class="settings-classification-wrapper">
      <!-- toolbar for mobile -->
      <div class="settings-toolbar-mobile settings-classification-toolbar-mobile">
        <div>
          <q-btn class="settings-toolbar-mobile-button" flat icon="arrow_back" @click="navigateToSettings(true)" />
        </div>
        <div>
          <h4 class="settings-toolbar-mobile-title">{{ $t('modules.budget.page.settings.header') }}</h4>
        </div>
        <div class="settings-toolbar-mobile-container">
          <q-btn class="settings-toolbar-mobile-button" flat icon="add_circle_outline" @click="openCreateBudgetModal" />
        </div>
      </div>

      <!-- toolbar for desktop -->
      <div class="settings-toolbar-desktop">
        <div class="settings-breadcrumbs">
          <div class="settings-breadcrumbs-item" @click="navigateToSettings(true)">
            {{ $t('pages.settings.settings.header_desktop') }}
          </div>
        </div>
      </div>

      <!-- main block -->
      <h4 class="settings-label-header settings-classification-header">{{ $t('modules.budget.page.settings.menu_item') }}</h4>
      <div class="settings-classification-create-class">
        <q-btn class="settings-classification-create-class-btn econumo-btn -small -magenta" flat
               :label="$t('modules.budget.page.settings.create_budget')" @click="openCreateBudgetModal" />
      </div>

      <div class="settings-classification-container">
        <div class="settings-classification-empty" v-if="!budgets.length">
          {{ $t('blocks.list.list_empty') }}
        </div>
        <div v-if="budgets.length > 0">
          <q-list class="settings-classification-list">
            <q-item v-for="element in budgets" :key="element.id"
                    class="settings-classification-list-item" clickable
                    @click="handleItemClick(element.id)">
              <q-item-section side class="settings-classification-list-item-avatar" avatar>
                <q-btn square flat disable icon="turned_in" v-if="userDefaultBudgetId === element.id" />
                <q-btn square flat icon="turned_in_not" v-else-if="isAccepted(element)" @click.stop="setAsDefault(element.id)" />
                <q-btn square flat disable icon="turned_in_not" v-else />
              </q-item-section>
              <q-item-section
                :class="'settings-classification-list-item-text ' + (!hasAccess(element) ? '-archived' : '')">
                <span class="settings-classification-list-item-name econumo-truncate" :title="element.name">{{ element.name }}</span>
                <div class="settings-classification-list-item-description-archived" v-if="!isAccepted(element)">
                  {{ $t('modules.budget.page.settings.level.' + getRole(element)) }} -
                  {{ $t('modules.budget.page.settings.not_accepted') }}
                </div>
              </q-item-section>
              <q-item-section class="settings-classification-list-item-shared" v-if="element.access.length > 1 && econumoPackage.includesSharedAccess"
                              side>
                <q-avatar class="settings-classification-list-item-shared-avatar" v-for="access in element.access"
                          v-bind:key="access.user.id">
                  <img :src="avatarUrl(access.user.avatar, 30)" :alt="access.user.name" :title="access.user.name" width="30" height="30" />
                </q-avatar>
              </q-item-section>
              <q-item-section side v-if="$q.screen.gt.md"
                              class="cursor-pointer settings-classification-list-item-check-section">
                <q-btn square flat icon="more_vert" class="account-transactions-item-check-button" @click.stop>
                  <q-menu cover auto-close class="account-transactions-item-check-button-menu" :ref="(el) => setMenuRef(el, element.id)">
                    <q-list class="account-transactions-item-check-button-list">
                      <q-item clickable @click="acceptAccess(element.id)" v-if="!isAccepted(element) && econumoPackage.includesSharedAccess"
                              class="account-transactions-item-check-button-item">
                        <q-item-section class="account-transactions-item-check-button-section">
                          {{ $t('elements.button.accept.label') }}
                        </q-item-section>
                      </q-item>
                      <q-item clickable @click="goToBudget(element.id)" v-if="isAccepted(element)"
                              class="account-transactions-item-check-button-item">
                        <q-item-section class="account-transactions-item-check-button-section">
                          {{ $t('modules.budget.page.settings.list_actions.go_to') }}
                        </q-item-section>
                      </q-item>
                      <q-item clickable @click="openSharedAccessModal(element.id)" v-if="hasAdminAccess(element) && econumoPackage.includesSharedAccess"
                              class="account-transactions-item-check-button-item">
                        <q-item-section class="account-transactions-item-check-button-section">
                          {{ $t('modules.budget.page.settings.list_actions.access') }}
                        </q-item-section>
                      </q-item>
                      <q-item clickable @click="openDeclineAccessBudgetModal(element.id)" v-if="!hasOwnerAccess(element) && econumoPackage.includesSharedAccess"
                              class="account-transactions-item-check-button-item">
                        <q-item-section class="account-transactions-item-check-button-section -delete">
                          {{ $t('elements.button.decline.label') }}
                        </q-item-section>
                      </q-item>
                      <q-item clickable @click="openDeleteBudgetModal(element.id)" v-if="hasAdminAccess(element)"
                              class="account-transactions-item-check-button-item">
                        <q-item-section class="account-transactions-item-check-button-section -delete">
                          {{ $t('elements.button.delete.label') }}
                        </q-item-section>
                      </q-item>
                    </q-list>
                  </q-menu>
                </q-btn>
              </q-item-section>
            </q-item>
          </q-list>
        </div>
      </div>

      <teleport to="body">
        <context-menu-modal v-if="contextMenuModal.isOpened"
                            :header-label="contextMenuModal.budget?.name"
                            :actions="[
                              {label: $t('elements.button.accept.label'), value: 'accept', context: contextMenuModal.budget?.id, isHidden: isAccepted(contextMenuModal.budget)},
                              {label: $t('modules.budget.page.settings.list_actions.go_to'), value: 'go_to', context: contextMenuModal.budget?.id, isHidden: !hasAccess(contextMenuModal.budget) || !isAccepted(contextMenuModal.budget)},
                              {label: $t('elements.button.set_as_default.label'), value: 'set_as_default', context: contextMenuModal.budget?.id, isHidden: !hasAccess(contextMenuModal.budget) || userDefaultBudgetId === contextMenuModal.budget.id},
                              {label: $t('pages.settings.accounts.list_actions.access'), value: 'access', context: contextMenuModal.budget?.id, isHidden: !hasAdminAccess(contextMenuModal.budget)},
                              {label: $t('elements.button.delete.label'), value: 'delete', context: contextMenuModal.budget?.id},
                              {label: $t('elements.button.cancel.label'), value: 'cancel', context: contextMenuModal.budget?.id}
                              ]"
                            v-on:cancel="closeContextMenuModal"
                            v-on:proceed="openNextModal"
        />

        <confirmation-dialog-modal v-if="modalDeleteBudget.isOpened"
                                   :question-title="$t('modules.budget.page.settings.delete_modal.title')"
                                   :question-label="$t('modules.budget.page.settings.delete_modal.question', {name: modalDeleteBudget.budget?.name})"
                                   :action-label="$t('elements.button.delete.label')"
                                   :cancel-label="$t('elements.button.cancel.label')"
                                   v-on:cancel="closeDeleteBudgetModal"
                                   v-on:proceed="deleteBudget(modalDeleteBudget.budget.id)" />

        <confirmation-dialog-modal v-if="modalDeclineAccessBudget.isOpened"
                                   :question-title="$t('modules.budget.page.settings.decline_access_modal.title')"
                                   :question-label="$t('modules.budget.page.settings.decline_access_modal.question', {name: modalDeclineAccessBudget.budget?.name})"
                                   :action-label="$t('elements.button.decline.label')"
                                   :cancel-label="$t('elements.button.cancel.label')"
                                   v-on:cancel="closeDeclineAccessBudgetModal"
                                   v-on:proceed="declineAccessBudget(modalDeclineAccessBudget.budget.id)" />

        <budget-create-form-modal :id="modalCreateBudget.budgetId" :name="modalCreateBudget.budgetName"
                                  :excluded="modalCreateBudget.accountsExcluded"
                                  :currency-id="modalCreateBudget.currencyId"
                                  v-on:update-name="value => modalCreateBudget.budgetName = value"
                                  v-on:update-excluded="value => modalCreateBudget.accountsExcluded = value"
                                  v-on:update-currency="value => modalCreateBudget.currencyId = value"
                                  v-on:submit="createBudget"
                                  v-on:close="closeCreateBudgetModal"
                                  v-if="modalCreateBudget.isOpened"
        />

        <budget-access-modal v-if="sharedAccessModal.isOpened"
                             :budget-id="sharedAccessModal.budget.id"
                             :budget-name="sharedAccessModal.budget.name"
                             :access="sharedAccessModal.budget.access"
                             :connections="connections"
                             v-on:cancel="closeSharedAccessModal"
                             v-on:user-selected="openSharedAccessLevelModal"
        />
        <budget-access-level-modal v-if="accessLevelModal.isOpened"
                                   :user="accessLevelModal.user"
                                   :budget-id="accessLevelModal.budgetId"
                                   :role="accessLevelModal.role"
                                   v-on:cancel="closeSharedAccessLevelModal"
                                   v-on:share="updateSharedAccess"
                                   v-on:revoke="revokeSharedAccess"
        />

        <error-modal v-if="modalError.isOpened"
                     :header="modalError.header"
                     :description="modalError.description"
                     v-on:close="closeErrorModal"
        />
      </teleport>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, defineComponent, onMounted, reactive, ref, watch } from 'vue';
import _ from 'lodash';
import ConfirmationDialogModal from '../../components/ConfirmationDialogModal.vue';
import { useUsersStore } from 'stores/users';
import { useBudgetsStore } from 'stores/budgets';
import { useConnectionsStore } from 'stores/connections';
import { useActiveAreaStore } from 'stores/active-area';
import { type RouteLocationNormalized, useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { RouterPage } from '../../router/constants';
import {
  AcceptAccessResponseDto,
  BudgetMetaDto,
  CreateBudgetRequestDto,
  CreateBudgetResponseDto, DeleteBudgetResponseDto, GrantAccessResponseDto, RevokeAccessResponseDto,
  UpdateBudgetRequestDto,
} from 'modules/api/v1/dto/budget.dto';
import { Id } from '@shared/types';
import BudgetCreateFormModal from 'components/Budget/BudgetCreateFormModal.vue';
import { AccessRole, AcceptanceStatus } from '@shared/dto/access.dto';
import BudgetAccessLevelModal from 'components/Budget/BudgetAccessLevelModal.vue';
import BudgetAccessModal from 'components/Budget/BudgetAccessModal.vue';
import ErrorModal from 'components/ErrorModal.vue';
import ContextMenuModal from 'components/ContextMenuModal.vue';
import { econumoPackage } from '../../modules/package';
import { useQuasar } from 'quasar';
import { useAvatar } from '../../composables/useAvatar';

defineOptions({
  name: 'SettingsBudgetsPage'
});

const router = useRouter();
const route = useRoute();
const budgetStore = useBudgetsStore();
const userStore = useUsersStore();
const { t } = useI18n();
const $q = useQuasar();
const { avatarUrl } = useAvatar();

onMounted(() => {
  const currentRouteName = router.currentRoute.value.name;
  if (currentRouteName === RouterPage.SETTINGS_BUDGETS) {
    useActiveAreaStore().setWorkspaceActiveArea();
  } else if (currentRouteName === RouterPage.HOME) {
    useActiveAreaStore().setSidebarActiveArea();
  }
});

watch(
  () => route,
  (to: RouteLocationNormalized) => {
    if (to.name === RouterPage.SETTINGS_BUDGETS) {
      useActiveAreaStore().setWorkspaceActiveArea();
    } else if (to.name === RouterPage.HOME) {
      useActiveAreaStore().setSidebarActiveArea();
    }
  },
  { immediate: true, deep: true }
);

const isBudgetsLoaded = computed(() => budgetStore.isBudgetsLoaded);
if (!isBudgetsLoaded.value) {
  budgetStore.fetchBudgets();
}

const connections = computed(() => useConnectionsStore().connections);
const budgets = computed(() => _.orderBy(budgetStore.budgets, 'name'));
const userId = computed(() => userStore.userId);
const userDefaultBudgetId = computed(() => userStore.userDefaultBudgetId);
const userCurrencyId = computed(() => userStore.userCurrencyId);

const menuRefs = ref<Map<Id, any>>(new Map());

function setMenuRef(el: any, budgetId: Id) {
  if (el) {
    menuRefs.value.set(budgetId, el);
  }
}

function handleItemClick(budgetId: Id) {
  if ($q.screen.gt.md) {
    // Desktop: open menu
    const menu = menuRefs.value.get(budgetId);
    if (menu) {
      menu.show();
    }
  } else {
    // Mobile: open context menu modal
    openContextMenuModal(budgetId);
  }
}

function navigateToSettings(isSidebarActive = false) {
  if (isSidebarActive) {
    useActiveAreaStore().setSidebarActiveArea();
  } else {
    useActiveAreaStore().setWorkspaceActiveArea();
  }
  router.push({ name: RouterPage.SETTINGS });
}

const modalError = reactive({
  isOpened: false,
  header: '',
  description: ''
});

function openErrorModal(header: string, description: string) {
  modalError.header = header;
  modalError.description = description;
  modalError.isOpened = true;
}

function closeErrorModal() {
  modalError.isOpened = false;
  modalError.header = '';
  modalError.description = '';
}

const contextMenuModal = reactive({
  isOpened: false,
  budget: null
});

function openContextMenuModal(budgetId: Id) {
  contextMenuModal.budget = _.cloneDeep(_.find(budgets.value, { id: budgetId }));
  contextMenuModal.isOpened = true;
}

function closeContextMenuModal() {
  contextMenuModal.budget = null;
  contextMenuModal.isOpened = false;
}

function openNextModal(value: string, budgetId: Id) {
  closeContextMenuModal();
  if (value === 'cancel') {
    // Just close the modal, which is already done above
    return;
  } else if (value === 'access') {
    openSharedAccessModal(budgetId);
  } else if (value === 'go_to') {
    goToBudget(budgetId);
  } else if (value === 'set_as_default') {
    setAsDefault(budgetId);
  } else if (value === 'delete') {
    openDeleteBudgetModal(budgetId);
  } else if (value === 'accept') {
    acceptAccess(budgetId);
  }
}

const modalDeleteBudget = reactive({
  isOpened: false,
  budget: null
});

function openDeleteBudgetModal(budgetId: Id) {
  modalDeleteBudget.budget = _.cloneDeep(_.find(budgets.value, { id: budgetId }));
  modalDeleteBudget.isOpened = true;
}

function closeDeleteBudgetModal() {
  modalDeleteBudget.isOpened = false;
  modalDeleteBudget.budget = null;
}

function deleteBudget(budgetId: Id) {
  budgetStore.deleteBudget(budgetId)
    .then((response: DeleteBudgetResponseDto) => {
      if (!response.success) {
        openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
      }
    })
    .catch(() => {
      openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
    })
    .finally(() => {
      closeDeleteBudgetModal();
    });
}

const modalDeclineAccessBudget = reactive({
  isOpened: false,
  budget: null
});

function openDeclineAccessBudgetModal(budgetId: Id) {
  modalDeclineAccessBudget.budget = _.cloneDeep(_.find(budgets.value, { id: budgetId }));
  modalDeclineAccessBudget.isOpened = true;
}

function closeDeclineAccessBudgetModal() {
  modalDeclineAccessBudget.isOpened = false;
  modalDeclineAccessBudget.budget = null;
}

function declineAccessBudget(budgetId: Id) {
  closeDeclineAccessBudgetModal();
  useBudgetsStore().declineAccess({ budgetId: budgetId });
}

function hasAccess(budget: BudgetMetaDto) {
  if (budget.ownerUserId === userId.value) {
    return true;
  }
  let result = false;
  budget.access.forEach((item) => {
    if (item.user?.id === userId.value && !!item.isAccepted) {
      result = true;
    }
  });
  return result;
}

function isAccepted(budget: BudgetMetaDto): boolean {
  if (budget.ownerUserId === userId.value) {
    return true;
  }
  let result = false;
  budget.access.forEach((item) => {
    if (item.user?.id === userId.value) {
      result = item.isAccepted === AcceptanceStatus.ACCEPTED;
    }
  });
  return result;
}

function getRole(budget: BudgetMetaDto): AccessRole | null {
  if (budget.ownerUserId === userId.value) {
    return AccessRole.OWNER;
  }
  let result = null;
  budget.access.forEach((item) => {
    if (item.user?.id === userId.value) {
      result = item.role;
    }
  });
  return result;
}

function hasAdminAccess(budget: BudgetMetaDto) {
  if (budget.ownerUserId === userId.value) {
    return true;
  }
  let result = false;
  budget.access.forEach((item) => {
    if (item.user?.id === userId.value && item.role === 'admin' && item.isAccepted) {
      result = true;
    }
  });
  return result;
}

function hasOwnerAccess(budget: BudgetMetaDto) {
  return budget.ownerUserId === userId.value;
}

function setAsDefault(budgetId: Id) {
  useUsersStore().userUpdateDefaultBudget(budgetId);
}


const modalCreateBudget = reactive({
  isOpened: false,
  budgetId: null,
  budgetName: '',
  accountsExcluded: [],
  currencyId: userCurrencyId.value
});

function openCreateBudgetModal() {
  modalCreateBudget.budgetId = budgetStore.generateId();
  modalCreateBudget.currencyId = userCurrencyId.value;
  modalCreateBudget.isOpened = true;
}

function closeCreateBudgetModal() {
  modalCreateBudget.isOpened = false;
}

function resetCreateBudgetModal() {
  modalCreateBudget.budgetId = null;
  modalCreateBudget.budgetName = '';
  modalCreateBudget.accountsExcluded = [];
  modalCreateBudget.currencyId = userCurrencyId.value;
}

function createBudget(dto: CreateBudgetRequestDto) {
  budgetStore.createBudget(dto).then((response: CreateBudgetResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    } else {
      resetCreateBudgetModal();
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeCreateBudgetModal();
  });
}


const sharedAccessModal = reactive({
  isOpened: false,
  budget: null
});

const accessLevelModal = reactive({
  isOpened: false,
  user: null,
  budgetId: null,
  role: null
});

function openSharedAccessModal(budgetId: Id) {
  sharedAccessModal.budget = _.cloneDeep(_.find(budgets.value, { id: budgetId }));
  sharedAccessModal.isOpened = true;
}

function closeSharedAccessModal() {
  sharedAccessModal.isOpened = false;
  sharedAccessModal.budget = null;
  accessLevelModal.isOpened = false;
  accessLevelModal.user = null;
  accessLevelModal.budgetId = null;
  accessLevelModal.role = null;
}

function openSharedAccessLevelModal(budgetId: Id, userId: Id, role: AccessRole) {
  if (role === AccessRole.OWNER) {
    return;
  }
  const budget = _.cloneDeep(_.find(budgets.value, { id: budgetId }));
  if (!budget) return;
  
  const sharedAccess = _.find(budget.access, { user: { id: userId } });
  accessLevelModal.budgetId = budget.id;
  let tmpUser = _.find(connections.value, { user: { id: userId } });
  if (tmpUser) {
    accessLevelModal.user = tmpUser.user;
  } else {
    accessLevelModal.user = sharedAccess?.user || null;
  }
  accessLevelModal.role = sharedAccess?.role || null;
  accessLevelModal.isOpened = true;
}

function closeSharedAccessLevelModal() {
  accessLevelModal.isOpened = false;
  accessLevelModal.user = null;
  accessLevelModal.budgetId = null;
  accessLevelModal.role = null;
}

function updateSharedAccess(budgetId: Id, userId: Id, role: AccessRole) {
  closeSharedAccessModal();
  budgetStore.grantAccess({
    budgetId: budgetId,
    userId: userId,
    role: role
  })
    .then((response: GrantAccessResponseDto) => {
      if (!response.success) {
        openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
      }
    })
    .catch(() => {
      openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
    });
}

function revokeSharedAccess(budgetId: Id, userId: Id) {
  closeSharedAccessModal();
  budgetStore.revokeAccess({
    budgetId: budgetId,
    userId: userId
  })
    .then((response: RevokeAccessResponseDto) => {
      if (!response.success) {
        openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
      }
    })
    .catch(() => {
      openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
    });
}

function acceptAccess(budgetId: Id) {
  budgetStore.acceptAccess({
    budgetId: budgetId
  })
    .then((response: AcceptAccessResponseDto) => {
      if (!response.success) {
        openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
      }
    })
    .catch(() => {
      openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
    });
}

function goToBudget(budgetId: Id) {
  const budget = _.find(budgets.value, { id: budgetId });
  
  if (!budget) {
    openErrorModal(
      t('modules.budget.modal.generic_error.header'), 
      t('modules.budget.modal.generic_error.description')
    );
    return;
  }

  if (!hasAccess(budget) || !isAccepted(budget)) {
    openErrorModal(
      t('modules.budget.modal.access_error.header'), 
      t('modules.budget.modal.access_error.description')
    );
    return;
  }

  useUsersStore().userUpdateDefaultBudget(budgetId).then(() => {
    useActiveAreaStore().setWorkspaceActiveArea();
    router.push({ name: RouterPage.BUDGET });
    useBudgetsStore().fetchUserBudget();
  });
}

</script>
