<template>
  <q-page>
    <div class="budget">
      <!-- toolbar for mobile -->
      <div class="budget-toolbar-mobile">
        <div class="budget-toolbar-mobile-head">
          <div>
            <q-btn class="budget-toolbar-mobile-head-button" flat icon="arrow_back"
                   @click="navigateToHome(true)" />
          </div>
          <div>
            <h4 class="budget-toolbar-mobile-head-title" v-if="budgetMeta">
              <span class="budget-toolbar-mobile-head-name econumo-truncate" :title="budgetMeta.name">{{ budgetMeta.name }}</span>
            </h4>
            <h4 class="budget-toolbar-mobile-head-title" v-else>
              {{ $t('modules.budget.page.budget.empty.header') }}
            </h4>
          </div>
          <div>
            <q-btn class="budget-toolbar-mobile-head-button" flat icon="settings" v-if="!isEditMode" @click="openContextEditBudgetMenu" />
            <q-btn class="budget-toolbar-mobile-head-button" flat icon="check" v-else @click="isEditMode = false" />
          </div>
        </div>
      </div>

      <!-- toolbar for desktop -->
      <div class="budget-toolbar-desktop">
        <div class="budget-toolbar-desktop-head">
          <h4 class="budget-toolbar-desktop-head-title" v-if="budgetMeta">
            <span class="budget-toolbar-desktop-head-name econumo-truncate" :title="budgetMeta.name">{{ budgetMeta.name }}</span>
            <template v-if="isCurrenciesLoaded">
              <q-avatar size="24px"
                        :class="'budget-toolbar-desktop-head-icon ' + (selectedCurrencyId === currencyId ? 'budget-toolbar-desktop-head-icon-active' : '')"
                        v-for="currencyId in budgetCurrencies" v-bind:key="currencyId"
                        :title="currencies[currencyId].name"
                        @click="selectedCurrencyId = (currencyId === selectedCurrencyId ? null : currencyId)">
                {{ currencies[currencyId].symbol }}
              </q-avatar>
            </template>
          </h4>
          <h4 class="budget-toolbar-desktop-head-title" v-else>
            {{ $t('modules.budget.page.budget.empty.header') }}
          </h4>
        </div>
        <div class="budget-toolbar-desktop-settings">
          <q-btn flat class="budget-toolbar-desktop-settings-btn econumo-btn -small -yellow" v-if="isEditMode"
                 :label="$t('modules.budget.page.budget.settings.menu.edit_structure_done')"
                 @click="isEditMode = false"
          />
          <q-btn flat class="budget-toolbar-desktop-settings-btn" v-if="!isEditMode"
                 :label="$t('modules.budget.page.budget.settings.button')">
            <q-menu cover auto-close>
              <q-list>
                <q-item clickable class="context-menu-button-item" @click="openUpdateBudgetModal()" v-if="budgetMeta">
                  <q-item-section class="context-menu-button-section">
                    {{ $t('modules.budget.page.budget.settings.menu.edit') }}
                  </q-item-section>
                </q-item>
                <q-item :clickable="canUserConfigureBudget" :disable="!canUserConfigureBudget"
                        class="context-menu-button-item" @click="isEditMode = true" v-if="budgetMeta">
                  <q-item-section class="context-menu-button-section">
                    {{ $t('modules.budget.page.budget.settings.menu.edit_structure') }}
                  </q-item-section>
                </q-item>
                <q-item clickable class="context-menu-button-item" @click="navigateToSettings(true)">
                  <q-item-section class="context-menu-button-section">
                    {{ $t('modules.budget.page.budget.settings.menu.budget_list') }}
                  </q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </div>
      </div>

      <budget-onboarding v-if="isBudgetLoaded && !budget"
        @create-budget="openCreateBudgetModal"
        @create-account="openCreateAccountModal" />

      <div class="row" v-if="budgetMeta">
        <div class="budget-toolbar-period" ref="periodScroll">
          <!--          <div class="col budget-toolbar-period-expand">-->
          <!--            <q-btn flat square :icon="selectedBudgetCurrencyWidget.isOpened ? 'expand_more' : 'navigate_next'"-->
          <!--                   @click="toggleBudgetCurrencyWidget()" />-->
          <!--          </div>-->
          <!--          <div class="col-auto budget-toolbar-period" >-->
          <div
            :class="'budget-toolbar-period-item' + (period.isActive ? ' budget-toolbar-period-item-active' : '') + (period.outsideBudget ? ' budget-toolbar-period-item-outside' : '')"
            v-for="period in budgetRange" :key="period.value" @click="changeBudgetPeriod(period.value)">
            {{ period.label }}
          </div>
          <!--          </div>-->
        </div>
      </div>
      <div class="row" v-if="isEditMode && isBudgetLoaded && budget">
        <q-btn class="econumo-btn -small -magenta" flat
               :label="$t('modules.budget.page.budget.structure.action.create_folder')" @click="openCreateFolderModal()" />
      </div>

      <div class="row" v-if="isBudgetLoaded && budget && isCurrenciesLoaded">
        <div class="col">
          <div class="budget-table">
            <div class="row">
              <budget-expense-widget
                :currency-id="selectedCurrencyId ?? null"
              />
            </div>
      
            <budget-table-folder v-for="(folder, index) in budgetWithFolder" v-bind:key="folder.id"
                                 :edit-mode="isEditMode"
                                 :can-update-limits="canUserUpdateLimits"
                                 :budget-currency-id="budgetMeta.currencyId"
                                 :folder-id="folder.id"
                                 :folder-name="folder.name"
                                 :elements="folder.elements"
                                 :stats="folder.stats"
                                 :access="budgetMeta.access"
                                 :is-first="index === 0"
                                 :is-last="index === budgetWithFolder.length - 1"
                                 @update-budget="updateBudgetedValue"
                                 @show-transaction-list="showTransactionListModal"
                                 @delete-folder="initDeleteFolder"
                                 @order-elements="(event) => moveElement(event, folder.id)"
                                 @order-folders="orderFolders"
                                 @open-update-folder-modal="openUpdateFolderModal(folder.id, folder.name)"
                                 @open-create-envelope-modal="openCreateEnvelopeModal"
                                 @open-update-element-modal="initUpdateElement"
                                 @open-delete-element-modal="initDeleteElement"
                                 @open-change-currency-modal="openChangeCurrencyModal"
                                 @open-set-limit-modal="openSetLimitModal"
            />

            <budget-table-folder v-if="budgetWithoutFolder.elements.length"
                                 :edit-mode="isEditMode"
                                 :can-update-limits="canUserUpdateLimits"
                                 :budget-currency-id="budgetMeta.currencyId"
                                 :folder-id="null"
                                 :folder-name="$t('modules.budget.page.budget.structure.no_folder')"
                                 :elements="budgetWithoutFolder.elements"
                                 :stats="budgetWithoutFolder.stats"
                                 :access="budgetMeta.access"
                                 :is-first="null"
                                 :is-last="null"
                                 @update-budget="updateBudgetedValue"
                                 @show-transaction-list="showTransactionListModal"
                                 @delete-folder="() => {}"
                                 @order-elements="(event) => moveElement(event, null)"
                                 @open-create-envelope-modal="openCreateEnvelopeModal"
                                 @open-update-element-modal="initUpdateElement"
                                 @open-delete-element-modal="initDeleteElement"
                                 @open-change-currency-modal="openChangeCurrencyModal"
                                 @open-set-limit-modal="openSetLimitModal"
            />

            <budget-table-folder v-if="budgetInArchive.elements.length"
                                 :edit-mode="false"
                                 :can-update-limits="canUserUpdateLimits"
                                 :budget-currency-id="budgetMeta.currencyId"
                                 :folder-id="null"
                                 :folder-name="$t('modules.budget.page.budget.structure.in_archive')"
                                 :elements="budgetInArchive.elements"
                                 :stats="budgetInArchive.stats"
                                 :access="budgetMeta.access"
                                 :is-first="null"
                                 :is-last="null"
                                 @update-budget="updateBudgetedValue"
                                 @show-transaction-list="showTransactionListModal"
                                 @delete-folder="() => {}"
                                 @order-elements="() => {}"
                                 @open-create-envelope-modal="() => {}"
                                 @open-update-element-modal="initUpdateElement"
                                 @open-delete-element-modal="initDeleteElement"
                                 @open-change-currency-modal="openChangeCurrencyModal"
                                 @open-set-limit-modal="openSetLimitModal"
            />

            <budget-total :budget-stats="budgetStats" :budget-currency-id="budgetMeta.currencyId" />
          </div>
        </div>
      </div>
    </div>

    <teleport to="body">
      <budget-create-form-modal :id="modalCreateBudget.budgetId"
                                :name="modalCreateBudget.budgetName"
                                :currency-id="modalCreateBudget.currencyId"
                                :excluded="modalCreateBudget.accountsExcluded"
                                v-on:update-name="value => modalCreateBudget.budgetName = value"
                                v-on:update-excluded="value => modalCreateBudget.accountsExcluded = value"
                                v-on:update-currency="value => modalCreateBudget.currencyId = value"
                                v-on:submit="createBudget"
                                v-on:close="closeCreateBudgetModal"
                                v-if="modalCreateBudget.isOpened"
      />

      <budget-update-form-modal :id="modalUpdateBudget.budgetId"
                                :name="modalUpdateBudget.budgetName"
                                :excluded="modalUpdateBudget.accountsExcluded"
                                :currency-id="modalUpdateBudget.currencyId"
                                :can-update-name="canUserConfigureBudget"
                                v-on:update-name="value => modalUpdateBudget.budgetName = value"
                                v-on:update-excluded="value => modalUpdateBudget.accountsExcluded = value"
                                v-on:update-currency="value => modalUpdateBudget.currencyId = value"
                                v-on:submit="updateBudget"
                                v-on:close="closeUpdateBudgetModal"
                                v-if="modalUpdateBudget.isOpened"
      />

      <prompt-dialog-modal :header-label="$t('modules.budget.modal.create_folder_form.header')"
                           :input-value="modalCreateFolder.folderName"
                           :input-label="$t('modules.budget.form.budget.folder_name.placeholder')"
                           :cancel-label="$t('elements.button.cancel.label')"
                           :submit-label="$t('elements.button.create.label')"
                           :validation="[
                              val => validation.isNotEmpty(val) || $t('modules.budget.form.budget.folder_name.validation.required_field'),
                              val => validation.isValidBudgetFolderName(val) || $t('modules.budget.form.budget.folder_name.validation.invalid_name')
                           ]"
                           :id="modalCreateFolder.folderId"
                           v-on:close="closeCreateFolderModal"
                           v-on:submit="createFolder"
                           v-if="modalCreateFolder.isOpened"
      />

      <prompt-dialog-modal :header-label="$t('modules.budget.modal.update_folder_form.header')"
                           :input-value="modalUpdateFolder.folderName"
                           :input-label="$t('modules.budget.form.budget.folder_name.placeholder')"
                           :cancel-label="$t('elements.button.cancel.label')"
                           :submit-label="$t('elements.button.save.label')"
                           :validation="[
                              val => validation.isNotEmpty(val) || $t('modules.budget.form.budget.folder_name.validation.required_field'),
                              val => validation.isValidBudgetFolderName(val) || $t('modules.budget.form.budget.folder_name.validation.invalid_name')
                           ]"
                           :id="modalUpdateFolder.folderId"
                           v-on:close="closeUpdateFolderModal"
                           v-on:submit="updateFolder"
                           v-if="modalUpdateFolder.isOpened"
      />

      <budget-transaction-list-modal :budget-id="modalTransactionList.budgetId"
                                     :period-start="modalTransactionList.periodStart"
                                     :category-id="modalTransactionList.categoryId"
                                     :tag-id="modalTransactionList.tagId"
                                     :envelope-id="modalTransactionList.envelopeId"
                                     :selected-element="modalTransactionList.selectedElement"
                                     :currency-id="modalTransactionList.currencyId"
                                     :current-user-id="modalTransactionList.currentUserId"
                                     :access="modalTransactionList.access"
                                     v-on:close="closeTransactionListModal"
                                     v-if="modalTransactionList.isOpened"
      />

      <budget-create-envelope-modal :id="modalCreateEnvelope.envelopeId"
                                    :folder-id="modalCreateEnvelope.folderId"
                                    :budget-id="modalCreateEnvelope.budgetId"
                                    :budget-currency-id="budget.meta.currencyId"
                                    :elements="budget?.structure.elements"
                                    :access="budget?.meta.access"
                                    v-on:close="closeCreateEnvelopeModal"
                                    v-on:create="createEnvelope"
                                    v-if="modalCreateEnvelope.isOpened"
      />

      <budget-update-envelope-modal v-if="modalUpdateEnvelope.isOpened"
                                    :budget-id="modalUpdateEnvelope.budgetId"
                                    :id="modalUpdateEnvelope.envelopeId"
                                    :name="modalUpdateEnvelope.name"
                                    :icon="modalUpdateEnvelope.icon"
                                    :currency-id="modalUpdateEnvelope.currencyId"
                                    :is-archived="modalUpdateEnvelope.isArchived"
                                    :categories="modalUpdateEnvelope.categories"
                                    :elements="budget?.structure.elements"
                                    :access="budget?.meta.access"
                                    v-on:close="closeUpdateEnvelopeModal"
                                    v-on:save="updateEnvelope"
      />

      <confirmation-dialog-modal v-if="modalDeleteEnvelope.isOpened"
                                 icon=""
                                 :question-title="$t('modules.budget.modal.delete_envelope.header')"
                                 :question-label="$t('modules.budget.modal.delete_envelope.question')"
                                 :action-label="$t('elements.button.delete.label')"
                                 :cancel-label="$t('elements.button.cancel.label')"
                                 v-on:cancel="closeDeleteEnvelopeModal"
                                 v-on:proceed="deleteEnvelope({budgetId: modalDeleteEnvelope.budgetId, id: modalDeleteEnvelope.envelopeId})"
      />

      <error-modal v-if="modalError.isOpened"
                   :header="modalError.header"
                   :description="modalError.description"
                   v-on:close="closeErrorModal"
        />

        <budget-change-currency-modal v-if="modalChangeCurrency.isOpened"
                                      :id="modalChangeCurrency.elementId"
                                      :currency-id="modalChangeCurrency.currencyId"
                                      :budget-id="budgetMeta.id"
                                      v-on:close="closeChangeCurrencyModal"
                                      v-on:change-currency="changeCurrency"
        />

        <budget-set-limit-modal v-if="modalSetLimit.isOpened"
                                      :element-id="modalSetLimit.elementId"
                                      :budget-id="modalSetLimit.budgetId"
                                      :amount="modalSetLimit.amount"
                                      :period="modalSetLimit.period"
                                      v-on:close="closeSetLimitModal"
                                      v-on:set-limit="setLimit"
        />

      <context-menu-modal v-if="contextEditBudgetMenu.isOpened"
                            :header-label="budgetMeta?.name"
                            :actions="[
                              {label: $t('modules.budget.page.budget.settings.menu.edit'), value: 'edit', context: null, isHidden: !canUserConfigureBudget},
                              {label: $t('modules.budget.page.budget.settings.menu.edit_structure'), value: 'edit_structure', context: null, isHidden: !canUserConfigureBudget},
                              {label: $t('modules.budget.page.budget.settings.menu.budget_list'), value: 'open_settings', context: null, isHidden: false}
                              ]"
                            v-on:cancel="closeContextEditBudgetMenu"
                            v-on:proceed="executeNextContextEditBudgetAction"
        />
    </teleport>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch, Ref } from 'vue';
import { useActiveAreaStore } from 'stores/active-area';
import { useBudgetsStore } from 'stores/budgets';
import type { RouteLocationNormalized } from 'vue-router';
import { useRoute, useRouter } from 'vue-router';
import BudgetCreateFormModal from '../../components/Budget/BudgetCreateFormModal.vue';
import { RouterPage } from '../../router/constants';
import BudgetOnboarding from '../../components/Budget/BudgetOnboarding.vue';
import {
  BudgetBaseElementDto,
  BudgetChildElementDto,
  BudgetElementDto,
  BudgetElementType,
  BudgetFolderDto, ChangeCurrencyRequestDto, ChangeCurrencyResponseDto,
  CreateBudgetEnvelopeRequestDto,
  CreateBudgetFolderResponseDto,
  CreateBudgetRequestDto,
  CreateBudgetResponseDto,
  CreateEnvelopeResponseDto,
  DeleteBudgetEnvelopeRequestDto,
  DeleteBudgetFolderResponseDto,
  DeleteEnvelopeResponseDto,
  MoveElementResponseDto,
  OrderFolderListResponseDto, SetLimitResponseDto,
  UpdateBudgetEnvelopeRequestDto,
  UpdateBudgetFolderResponseDto,
  UpdateBudgetRequestDto,
  UpdateBudgetResponseDto,
  UpdateEnvelopeResponseDto,
  SetElementLimitRequestDto,
  BudgetStatsDto
} from '../../modules/api/v1/dto/budget.dto';
import BudgetTableFolder from 'components/Budget/BudgetTableFolder.vue';
import { date } from 'quasar';
import { Id, DateString } from '@shared/types';
import { useCurrenciesStore } from 'stores/currencies';
import BudgetTransactionListModal from 'components/Budget/BudgetTransactionListModal.vue';
import { useUsersStore } from 'stores/users';
import BudgetUpdateFormModal from 'components/Budget/BudgetUpdateFormModal.vue';
import PromptDialogModal from 'components/PromptDialogModal.vue';
import { useI18n } from 'vue-i18n';
import BudgetCreateEnvelopeModal from 'components/Budget/BudgetCreateEnvelopeModal.vue';
import { isNotEmpty, isValidBudgetFolderName } from '../../modules/helpers/validation';
import BudgetUpdateEnvelopeModal from 'components/Budget/BudgetUpdateEnvelopeModal.vue';
import ConfirmationDialogModal from 'components/ConfirmationDialogModal.vue';
import { DATE_FORMAT } from '../../modules/constants';
import ErrorModal from 'components/ErrorModal.vue';
import ContextMenuModal from 'components/ContextMenuModal.vue';
import BudgetChangeCurrencyModal from 'components/Budget/BudgetChangeCurrencyModal.vue';
import BudgetSetLimitModal from 'components/Budget/BudgetSetLimitModal.vue';
import BudgetTotal from 'components/Budget/BudgetTotal.vue';
import _ from 'lodash';
import { useCurrency } from '../../composables/useCurrency';
import { useAccountModalStore } from 'stores/account-modal';
import { useAccountFoldersStore } from 'stores/account-folders';
import { useValidation } from '../../composables/useValidation';
import { useDecimalNumber } from '../../composables/useDecimalNumber';
import BudgetExpenseWidget from '../../components/Budget/BudgetExpenseWidget.vue';

defineOptions({
  name: 'BudgetPage',
  mixins: []
});


const router = useRouter();
const route = useRoute();
const budgetStore = useBudgetsStore();
const userStore = useUsersStore();
const { t } = useI18n();
const { exchange } = useCurrency();
const validation = useValidation();
const decimalNumber = useDecimalNumber();

const selectedCurrencyId = ref(null) as Ref<Id | null>;
const userCurrencyId = ref(userStore.userCurrencyId);
const budgetDate = computed(() => budgetStore.budgetDate);
const budgetDateRange = {
  left: 23,
  right: 23
};

const budgetRange = computed(() => {
  let result = [];
  const currentYear = date.formatDate(new Date(), 'YYYY');
  let extracted: Date | string = budgetDate.value;
  if (typeof extracted === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(extracted)) {
    extracted = date.extractDate(budgetDate.value, 'YYYY-MM-DD');
  } else {
    extracted = date.extractDate(budgetDate.value, 'YYYY-MM-DD HH:mm:ss');
  }
  const selectedPeriod = date.adjustDate(extracted, {
    date: 1,
    millisecond: 0,
    second: 0,
    minute: 0,
    hour: 0
  });
  const leftRange = budgetDateRange.left;
  const rightRange = budgetDateRange.right;
  let period = null;
  let counter = 0;
  for (let i = leftRange; i >= 0; i--) {
    period = date.subtractFromDate(selectedPeriod, { months: i });
    let periodYear = date.formatDate(period, 'YYYY');
    result.push({
      label: date.formatDate(period, currentYear === periodYear ? 'MMMM' : 'MMM YYYY'),
      value: date.formatDate(period, 'YYYY-MM-01'),
      number: counter++,
      isActive: selectedPeriod.getTime() === period.getTime(),
      outsideBudget: period < budgetStartedAt.value
    });
  }
  for (let i = 1; i <= rightRange; i++) {
    let period = date.addToDate(selectedPeriod, { months: i });
    let periodYear = date.formatDate(period, 'YYYY');
    result.push({
      label: date.formatDate(period, currentYear === periodYear ? 'MMMM' : 'MMM YYYY'),
      value: date.formatDate(period, 'YYYY-MM-01'),
      number: counter++,
      isActive: selectedPeriod.getTime() === period.getTime(),
      outsideBudget: period < budgetStartedAt.value
    });
  }
  return result;
});

const periodScroll = ref(null);
const centerActiveDate = () => {
  if (!periodScroll.value) return;

  const container = periodScroll.value as HTMLElement;
  const activeItem = container.children[budgetDateRange.left - 1];

  if (activeItem) {
    const containerWidth = container.offsetWidth;
    const itemWidth = (activeItem as HTMLElement).offsetWidth;
    const itemLeft = (activeItem as HTMLElement).offsetLeft;
    const scrollLeft = itemLeft - (containerWidth / 2) + itemWidth * 1.5;
    container.scrollTo({
      left: scrollLeft,
      behavior: 'smooth'
    });
  }
};

onMounted(() => {
  const currentRouteName = router.currentRoute.value.name;
  if (currentRouteName === RouterPage.BUDGET) {
    useActiveAreaStore().setWorkspaceActiveArea();
  } else if (currentRouteName === RouterPage.HOME) {
    useActiveAreaStore().setSidebarActiveArea();
  }
  setTimeout(() => {
    centerActiveDate();
  }, 100);
});

watch(
  () => route,
  (to: RouteLocationNormalized) => {
    if (to.name === RouterPage.BUDGET) {
      useActiveAreaStore().setWorkspaceActiveArea();
    } else if (to.name === RouterPage.HOME) {
      useActiveAreaStore().setSidebarActiveArea();
    }
    setTimeout(() => {
      centerActiveDate();
    }, 100);
  },
  { immediate: true, deep: true }
);

function navigateToSettings(isSidebarActive = false) {
  if (isSidebarActive) {
    useActiveAreaStore().setSidebarActiveArea();
  } else {
    useActiveAreaStore().setWorkspaceActiveArea();
  }
  router.push({ name: RouterPage.SETTINGS_BUDGETS });
}

function navigateToHome(isSidebarActive = false) {
  if (isSidebarActive) {
    useActiveAreaStore().setSidebarActiveArea();
  } else {
    useActiveAreaStore().setWorkspaceActiveArea();
  }
  router.push({ name: RouterPage.HOME });
}

const isEditMode = ref(false);
const isBudgetLoaded = computed(() => budgetStore.isBudgetLoaded);
const canUserConfigureBudget = computed(() => budgetStore.canUserConfigureBudget);
const budget = computed(() => budgetStore.budget);

const canUserUpdateLimits = computed(() => {
  if (budgetStore.canUserUpdateLimits) {
    const budgetDate = date.extractDate(budgetStore.budgetDate, DATE_FORMAT);
    if (budget.value && new Date(budget.value.meta.startedAt) <= budgetDate) {
      return true;
    }
  }
  return false;
});

watch(budget, (newValue) => {
  if (newValue) {
    setTimeout(() => {
      centerActiveDate();
    }, 100);
  }
});

if (!isBudgetLoaded.value) {
  budgetStore.fetchUserBudget().finally(() => {
    setTimeout(() => {
      centerActiveDate();
    }, 100);
  });
}

const currencyStore = useCurrenciesStore();
const currencies = computed(() => currencyStore.currenciesHash);
const isCurrenciesLoaded = computed(() => currencyStore.isCurrenciesLoaded);

const isBudgetLoading = computed(() => budgetStore.isBudgetLoading);

interface FolderResult {
  [key: string]: FolderResultItem;
}

interface FolderResultItem {
  id: string;
  elements: any[];
  stats: BudgetStatsDto;
  name?: string;
  position?: number;
}

const budgetStats = computed(() => {
  const budget = budgetStore.budget;
  if (!budget) return [];

  const result = { budgeted: 0, spent: 0, available: 0 };
  budgetWithFolder.value.forEach((folder: FolderResultItem) => {
    result.budgeted += folder.stats.budgeted;
    result.spent += folder.stats.spent;
    result.available += folder.stats.available;
  });

  result.budgeted += budgetWithoutFolder.value.stats.budgeted + budgetInArchive.value.stats.budgeted;
  result.spent += budgetWithoutFolder.value.stats.spent + budgetInArchive.value.stats.spent;
  result.available += budgetWithoutFolder.value.stats.available + budgetInArchive.value.stats.available;

  return result;
});

// Update the computed property
const budgetWithFolder = computed(() => {
  const budget = budgetStore.budget;
  if (!budget) return [];

  const result: FolderResult = {};
  let count = 0;
  _.orderBy(budget.structure.folders, 'position').forEach((folder: BudgetFolderDto) => {
    result[folder.id] = {
      id: folder.id,
      name: folder.name,
      position: folder.position,
      elements: [],
      stats: {
        budgeted: 0.0,
        spent: 0.0,
        available: 0.0
      }
    };
    count++;
  });
  _.orderBy(budget.structure.elements, 'position').forEach((element: BudgetElementDto) => {
    if (element.folderId !== null && !element.isArchived) {
      if (!result[element.folderId]) {
        result[element.folderId] = {
          id: element.folderId,
          elements: [],
          stats: { budgeted: 0, spent: 0, available: 0 }
        };
      }
      result[element.folderId].elements.push(_.cloneDeep(element));
      result[element.folderId].stats.budgeted += exchange(element.currencyId ?? budget.meta.currencyId, budget.meta.currencyId, element.budgeted, budget.currencyRates);
      result[element.folderId].stats.spent += element.budgetSpent;
      result[element.folderId].stats.available += exchange(element.currencyId ?? budget.meta.currencyId, budget.meta.currencyId, (element.available + element.budgeted), budget.currencyRates);
    }
  });

  return count === 0 ? [] : Object.values(result);
});

const budgetWithoutFolder = computed(() => {
  const budget = budgetStore.budget;
  if (!budget) return [];

  const result = {
    elements: [] as BudgetElementDto[],
    stats: {
      budgeted: 0.0,
      spent: 0.0,
      available: 0.0
    }
  };
  _.orderBy(budget.structure.elements, 'position').forEach((element: BudgetElementDto) => {
    if (element.folderId === null && !element.isArchived) {
      result.elements.push(_.cloneDeep(element));
      result.stats.budgeted += exchange(element.currencyId ?? budget.meta.currencyId, budget.meta.currencyId, element.budgeted, budget.currencyRates);
      result.stats.spent += element.budgetSpent;
      result.stats.available += exchange(element.currencyId ?? budget.meta.currencyId, budget.meta.currencyId, (element.available + element.budgeted), budget.currencyRates);
    }
  });
  return result;
});

const budgetInArchive = computed(() => {
  const budget = budgetStore.budget;
  if (!budget) return [];

  const result = {
    elements: [] as BudgetElementDto[],
    stats: {
      budgeted: 0.0,
      spent: 0.0,
      available: 0.0
    }
  };
  _.orderBy(budget.structure.elements, 'name').forEach((element: BudgetElementDto) => {
    if (element.isArchived) {
      result.elements.push(_.cloneDeep(element));
      result.stats.budgeted += exchange(element.currencyId ?? budget.meta.currencyId, budget.meta.currencyId, element.budgeted, budget.currencyRates);
      result.stats.spent += element.budgetSpent;
      result.stats.available += exchange(element.currencyId ?? budget.meta.currencyId, budget.meta.currencyId, (element.available + element.budgeted), budget.currencyRates);
    }
  });
  return result;
});

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

const modalCreateBudget = reactive({
  isOpened: false,
  budgetId: null,
  budgetName: '',
  accountsluded: [],
  currencyId: null
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
  modalCreateBudget.currencyId = null;
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
    centerActiveDate();
  });
}

const modalUpdateBudget = reactive({
  isOpened: false,
  budgetId: null,
  budgetName: '',
  accountsExcluded: [],
  currencyId: null
});

function openUpdateBudgetModal() {
  modalUpdateBudget.budgetId = budget.value.meta.id;
  modalUpdateBudget.budgetName = budget.value.meta.name;
  modalUpdateBudget.accountsExcluded = budget.value.filters.excludedAccountsIds;
  modalUpdateBudget.currencyId = budget.value.meta.currencyId;
  modalUpdateBudget.isOpened = true;
}

function closeUpdateBudgetModal() {
  modalUpdateBudget.isOpened = false;
  resetUpdateBudgetModal();
}

function resetUpdateBudgetModal() {
  modalUpdateBudget.budgetId = null;
  modalUpdateBudget.budgetName = '';
  modalUpdateBudget.accountsExcluded = [];
  modalUpdateBudget.currencyId = null;
}

function updateBudget(dto: UpdateBudgetRequestDto) {
  budgetStore.updateBudget(dto).then((response: UpdateBudgetResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    } else {
      resetUpdateBudgetModal();
      budgetStore.fetchUserBudget().finally(() => {
        centerActiveDate();
      });
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeUpdateBudgetModal();
  });
}


const modalCreateFolder = reactive({
  isOpened: false,
  folderId: null,
  folderName: ''
});

function openCreateFolderModal() {
  modalCreateFolder.folderId = budgetStore.generateId();
  modalCreateFolder.isOpened = true;
}

function closeCreateFolderModal() {
  modalCreateFolder.isOpened = false;
}

function resetCreateFolderModal() {
  modalCreateFolder.folderId = null;
  modalCreateFolder.folderName = '';
}

function createFolder(value: string, originalValue: string, id: Id) {
  budgetStore.createFolder({
    budgetId: budgetMeta.value.id,
    id: id,
    name: value
  }).then((response: CreateBudgetFolderResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    } else {
      resetCreateFolderModal();
      budgetStore.fetchUserBudget().finally(() => {
        centerActiveDate();
      });
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeCreateFolderModal();
  });
}


const modalUpdateFolder = reactive({
  isOpened: false,
  folderId: null,
  folderName: null
});

function openUpdateFolderModal(folderId: Id, folderName: string) {
  modalUpdateFolder.folderId = folderId;
  modalUpdateFolder.folderName = folderName;
  modalUpdateFolder.isOpened = true;
}

function closeUpdateFolderModal() {
  modalUpdateFolder.isOpened = false;
  resetUpdateFolderModal();
}

function resetUpdateFolderModal() {
  modalUpdateFolder.folderId = null;
  modalUpdateFolder.folderName = null;
}

function updateFolder(value: string, originalValue: string, id: Id) {
  budgetStore.updateFolder({
    budgetId: budgetMeta.value.id,
    id: id,
    name: value
  }).then((response: UpdateBudgetFolderResponseDto) => {
    if (!response.success) {
      closeUpdateFolderModal();
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeUpdateFolderModal();
  });
}


function initDeleteFolder(folderId: Id, needConfirmation: boolean) {
  if (needConfirmation) {
    return;
  } else {
    deleteFolder(folderId);
  }
}

function deleteFolder(id: Id) {
  budgetStore.deleteFolder(budget.value?.meta.id, id).then((response: DeleteBudgetFolderResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    } else {
      resetCreateFolderModal();
      budgetStore.fetchUserBudget().finally(() => {
        centerActiveDate();
      });
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeCreateFolderModal();
  });
}


const modalCreateEnvelope = reactive({
  isOpened: false,
  envelopeId: null,
  budgetId: null,
  folderId: null
});

function openCreateEnvelopeModal(folderId: Id | null) {
  modalCreateEnvelope.folderId = folderId;
  modalCreateEnvelope.envelopeId = budgetStore.generateId();
  modalCreateEnvelope.budgetId = budgetMeta.value.id;
  modalCreateEnvelope.isOpened = true;
}

function closeCreateEnvelopeModal() {
  modalCreateEnvelope.isOpened = false;
  resetCreateEnvelopeModal();
}

function resetCreateEnvelopeModal() {
  modalCreateEnvelope.folderId = null;
  modalCreateEnvelope.envelopeId = null;
  modalCreateEnvelope.budgetId = null;
}

function createEnvelope(item: CreateBudgetEnvelopeRequestDto) {
  budgetStore.createEnvelope(item).then((response: CreateEnvelopeResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    } else {
      resetCreateEnvelopeModal();
      if (item.categories.length > 0) {
        budgetStore.fetchUserBudget().finally(() => {
          centerActiveDate();
        });
      }
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeCreateEnvelopeModal();
  });
}


function initUpdateElement(element: BudgetElementDto) {
  if (element.type === BudgetElementType.ENVELOPE) {
    openUpdateEnvelopeModal(element);
  }
}


const modalUpdateEnvelope = reactive({
  isOpened: false,
  budgetId: null,
  envelopeId: null,
  name: '',
  icon: '',
  currencyId: null,
  isArchived: null,
  categories: []
});

function openUpdateEnvelopeModal(element: BudgetElementDto) {
  modalUpdateEnvelope.budgetId = budgetMeta.value.id;
  modalUpdateEnvelope.envelopeId = element.id;
  modalUpdateEnvelope.name = element.name;
  modalUpdateEnvelope.icon = element.icon;
  modalUpdateEnvelope.currencyId = element.currencyId;
  modalUpdateEnvelope.isArchived = element.isArchived;
  modalUpdateEnvelope.categories = [];
  element.children.forEach((child: BudgetChildElementDto) => {
    modalUpdateEnvelope.categories.push(child.id);
  });
  modalUpdateEnvelope.isOpened = true;
}

function closeUpdateEnvelopeModal() {
  modalUpdateEnvelope.isOpened = false;
  resetUpdateEnvelopeModal();
}

function resetUpdateEnvelopeModal() {
  modalUpdateEnvelope.folderId = null;
  modalUpdateEnvelope.budgetId = null;
  modalUpdateEnvelope.envelopeId = null;
  modalUpdateEnvelope.name = '';
  modalUpdateEnvelope.icon = '';
  modalUpdateEnvelope.currencyId = null;
  modalUpdateEnvelope.isArchived = null;
  modalUpdateEnvelope.categories = [];
}

function updateEnvelope(item: UpdateBudgetEnvelopeRequestDto) {
  budgetStore.updateEnvelope(item).then((response: UpdateEnvelopeResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    } else {
      resetUpdateEnvelopeModal();
      budgetStore.fetchUserBudget().finally(() => {
        centerActiveDate();
      });
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeUpdateEnvelopeModal();
  });
}


const modalDeleteEnvelope = reactive({
  isOpened: false,
  budgetId: null,
  envelopeId: null
});

function openDeleteEnvelopeModal(element: BudgetElementDto) {
  modalDeleteEnvelope.budgetId = budgetMeta.value.id;
  modalDeleteEnvelope.envelopeId = element.id;
  modalDeleteEnvelope.isOpened = true;
}

function closeDeleteEnvelopeModal() {
  modalDeleteEnvelope.isOpened = false;
  resetDeleteEnvelopeModal();
}

function resetDeleteEnvelopeModal() {
  modalDeleteEnvelope.budgetId = null;
  modalDeleteEnvelope.envelopeId = null;
}

function deleteEnvelope(item: DeleteBudgetEnvelopeRequestDto) {
  budgetStore.deleteEnvelope(item).then((response: DeleteEnvelopeResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    } else {
      resetDeleteEnvelopeModal();
      budgetStore.fetchUserBudget().finally(() => {
        centerActiveDate();
      });
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeDeleteEnvelopeModal();
  });
}


function initDeleteElement(element: BudgetElementDto) {
  if (element.type === BudgetElementType.ENVELOPE) {
    openDeleteEnvelopeModal(element);
  }
}


function changeBudgetPeriod(date) {
  budgetStore.changeBudgetPeriod(date);
  budgetStore.fetchUserBudget().then(() => {
    centerActiveDate();
  });
}

const budgetMeta = computed(() => budgetStore.budgetMeta);
const budgetCurrencies = computed(() => budgetStore.budgetCurrencies);
const budgetStartedAt = computed(() => new Date(budgetMeta.value.startedAt));

const modalTransactionList = reactive({
  isOpened: false,
  budgetId: null,
  periodStart: '',
  categoryId: null,
  tagId: null,
  envelopeId: null,
  selectedElement: null,
  currencyId: null,
  currentUserId: null,
  access: []
});

function showTransactionListModal(selectedElement: BudgetBaseElementDto, tagId: Id | null, categoryId: Id | null, envelopeId: Id | null, currencyId: Id) {
  modalTransactionList.budgetId = budgetMeta.value.id;
  modalTransactionList.periodStart = budgetDate.value;
  modalTransactionList.selectedElement = selectedElement;
  modalTransactionList.categoryId = categoryId;
  modalTransactionList.tagId = tagId;
  modalTransactionList.envelopeId = envelopeId;
  modalTransactionList.currencyId = currencyId;
  modalTransactionList.currentUserId = userStore.userId;
  modalTransactionList.access = budgetMeta.value.access;
  modalTransactionList.isOpened = true;
}

function closeTransactionListModal() {
  modalTransactionList.isOpened = false;
  modalTransactionList.budgetId = null;
  modalTransactionList.periodStart = '';
  modalTransactionList.selectedElement = null;
  modalTransactionList.categoryId = null;
  modalTransactionList.tagId = null;
  modalTransactionList.envelopeId = null;
  modalTransactionList.currencyId = null;
  modalTransactionList.currentUserId = null;
  modalTransactionList.access = [];
}

function moveElement(event: any, _) {
  const newFolderId = event.to.id || null,
    elementId = event.item.__draggable_context.element.id || null,
    newIndex = event.newIndex;
  budgetStore.moveElement(elementId, newIndex, newFolderId).then((response: MoveElementResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  });
}

function orderFolders(folderId: Id, direction: number) {
  const folder = budget.value.structure.folders.find(folder => folder.id === folderId);
  budgetStore.orderFolders(folderId, folder.position + direction).then((response: OrderFolderListResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  });
}


function updateBudgetedValue(elementId: Id, value: number | string) {
  if (value == 0 || isNaN(value)) {
    value = null;
  } else {
    value = decimalNumber.normalizeNumber(value);
  }
  const budgetDate = date.extractDate(budgetStore.budgetDate, DATE_FORMAT);
  budgetStore.setLimit({
    budgetId: budgetMeta.value.id,
    elementId: elementId,
    amount: value,
    period: date.formatDate(budgetDate, DATE_FORMAT)
  }).then((response: SetLimitResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  });
}


const modalChangeCurrency = reactive({
  isOpened: false,
  elementId: null,
  currencyId: null
});

function openChangeCurrencyModal(element: BudgetElementDto) {
  modalChangeCurrency.currencyId = element.currencyId || budgetMeta.value.currencyId;
  modalChangeCurrency.elementId = element.id;
  modalChangeCurrency.isOpened = true;
}

function closeChangeCurrencyModal() {
  modalChangeCurrency.isOpened = false;
  resetChangeCurrencyModal();
}

function resetChangeCurrencyModal() {
  modalChangeCurrency.currencyId = null;
  modalChangeCurrency.elementId = null;
}

function changeCurrency(item: ChangeCurrencyRequestDto) {
  budgetStore.changeElementCurrency(item).then((response: ChangeCurrencyResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    } else {
      resetChangeCurrencyModal();
      budgetStore.fetchUserBudget();
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeChangeCurrencyModal();
  });
}

const contextEditBudgetMenu = reactive({
  isOpened: false
});

function openContextEditBudgetMenu() {
  contextEditBudgetMenu.isOpened = true;
}

function closeContextEditBudgetMenu() {
  contextEditBudgetMenu.isOpened = false;
}

function executeNextContextEditBudgetAction(value: string) {
  closeContextEditBudgetMenu();
  if (value === 'edit') {
    openUpdateBudgetModal();
  } else if (value === 'edit_structure') {
    isEditMode.value = true;
  } else if (value === 'open_settings') {
    navigateToSettings(true);
  }
}

const modalSetLimit = reactive({
  isOpened: false,
  budgetId: null as Id | null,
  elementId: null as Id | null,
  period: null as DateString | null,
  amount: null as number | string | null
});

function openSetLimitModal(element: BudgetElementDto) {
  if (!budgetMeta.value?.id || !element?.id) return;
  modalSetLimit.budgetId = budgetMeta.value.id;
  modalSetLimit.elementId = element.id;
  modalSetLimit.period = budgetDate.value;
  modalSetLimit.amount = decimalNumber.normalizeNumber(element.budgeted);
  modalSetLimit.isOpened = true;
}

function closeSetLimitModal() {
  modalSetLimit.isOpened = false;
  modalSetLimit.elementId = null;
  modalSetLimit.period = '';
  modalSetLimit.amount = 0;
}

function setLimit(item: SetElementLimitRequestDto) {
  budgetStore.setLimit(item).then((response: SetLimitResponseDto) => {
    if (!response.success) {
      openErrorModal(t('modules.budget.modal.generic_error.header'), response.message);
    }
  }).catch(() => {
    openErrorModal(t('modules.budget.modal.generic_error.header'), t('modules.budget.modal.generic_error.description'));
  }).finally(() => {
    closeSetLimitModal();
  });
}

function openCreateAccountModal() {
  const accountModalStore = useAccountModalStore();
  const accountFoldersStore = useAccountFoldersStore();
  const firstFolder = accountFoldersStore.accountFoldersOrdered[0];
  accountModalStore.openAccountModal({ folderId: firstFolder?.id ?? null });
}

centerActiveDate();

</script>
