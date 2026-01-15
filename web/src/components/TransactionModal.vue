<template>
  <teleport to="body">
    <q-dialog class="transaction-modal" v-model="isModalOpened" no-backdrop-dismiss>
      <q-card class="transaction-modal-card">
        <q-form
          ref="transactionForm"
          @submit="submit"
          @reset="closeModal"
        >
          <q-card-section class="transaction-modal-header">
            <div class="transaction-modal-header-wrapper">
              <div class="transaction-modal-header-title">
                {{
                  isTransactionModalCreation ? $t('modals.transaction.create_form.header') : $t('modals.transaction.update_form.header')
                }}
              </div>
              <div>
                <div class="transaction-modal-header-calendar">
                  <q-input class="transaction-modal-header-calendar-input" v-model="transactionDate">
                    <template v-slot:append>
                      <q-icon class="transaction-modal-header-calendar-icon">
                        <q-popup-proxy ref="qDateProxy" cover transition-show="scale" transition-hide="scale">
                          <q-date class="transaction-modal-header-calendar-date" v-model="transactionDate" today-btn
                                  mask="YYYY-MM-DD" first-day-of-week="1">
                            <div>
                              <q-btn class="transaction-modal-header-calendar-date-btn" v-close-popup
                                     :label="$t('elements.button.ok.label')" flat/>
                            </div>
                          </q-date>
                        </q-popup-proxy>
                      </q-icon>
                    </template>
                  </q-input>
                </div>
                <div class="transaction-modal-header-previous-day">
                  <q-btn flat size="sm" icon="arrow_back" @click="previousDay()"/>
                </div>
              </div>
            </div>
          </q-card-section>

          <q-card-section class="transaction-modal-toggle">
            <div class="transaction-modal-toggle-wrapper">
              <q-btn-toggle
                class="transaction-modal-toggle-btn"
                v-model="transactionType"
                :options="[
                            {label: $t('modals.transaction.transaction_type.income'), value: 'income'},
                            {label: $t('modals.transaction.transaction_type.transfer'), value: 'transfer'},
                            {label: $t('modals.transaction.transaction_type.expense'), value: 'expense'}
                          ]"
              />
            </div>
          </q-card-section>

          <q-card-section class="transaction-modal-main">
            <div class="transaction-modal-main-account" v-if="!isTransfer">
              <q-select class="transaction-modal-main-account-select" v-model="account" :options="accountsOptions"
                        ref="accountSelect"
                        :title="account?.name"
                        :label="account.name"
                        popup-content-class="modal-popup"
                        :popup-content-style="accountPopupStyle"
                        options-selected-class="modal-popup-selected"
                        @popup-show="setAccountPopupWidth"
                        hide-dropdown-icon>
                <template v-slot:prepend>
                  <q-icon :name="account.icon"/>
                </template>
                <template v-slot:selected-item="scope">
                  <span class="transaction-modal-select-balance">
                    {{ scope.opt.balanceLabel }}
                  </span>
                </template>
                <template v-slot:option="scope">
                  <q-item v-bind="scope.itemProps">
                    <q-item-section>
                      <q-item-label class="transaction-modal-select-value">
                        <span class="econumo-truncate transaction-modal-select-name">
                          {{ scope.opt.name || scope.opt.label }}
                        </span>
                        <span class="transaction-modal-select-balance">
                          {{ scope.opt.balanceLabel }}
                        </span>
                      </q-item-label>
                    </q-item-section>
                  </q-item>
                </template>
              </q-select>
            </div>

            <div class="transaction-modal-main-amount">
              <calculator-input
                ref="amountInput"
                :class="'transaction-modal-main-amount-input amount ' + (isTransfer ? 'transfer' : '')"
                v-model="amount"
                :label="$t('modals.transaction.form.amount.label')"
                :placeholder="$t('modals.transaction.form.amount.label')"
                lazy-rules
                autofocus
                :rules="[
                  val => isNotEmpty(val.toString()) || $t('elements.validation.required_field'),
                  val => isValidNumber(val.toString()) || $t('elements.validation.invalid_number'),
                  val => isValidDecimalNumber(val.toString()) || $t('elements.validation.invalid_decimal_number'),
                  val => isValidFormula(val.toString()) || $t('elements.validation.invalid_formula')
                ]"
              />
            </div>

            <div class="transaction-modal-main-amount"
                 v-if="isTransfer && account && accountRecipient && account.currencyId !== accountRecipient.currencyId">
              <q-input class="transaction-modal-options-input amount-recipient" inputmode="decimal" step="0.01"
                       v-model="amountRecipient" :label="$t('modals.transaction.form.amount_recipient.label')"
                       lazy-rules
                       :class="{ 'transfer': isTransfer }"
                       :rules="[
                         val => isNotEmpty(val.toString()) || $t('elements.validation.required_field'),
                         val => isValidNumber(val.toString()) || $t('elements.validation.invalid_number'),
                         val => isValidDecimalNumber(val.toString()) || $t('elements.validation.invalid_decimal_number')
                       ]"
              />
            </div>

            <div class="transaction-modal-main-category" v-if="!isTransfer">
              <q-select class="form-select transaction-modal-main-category-select" outlined bottom-slots
                        v-model="category" :use-input="canChangeAccountData"
                        :input-value="categoryInputValue"
                        ref="categorySelect"
                        :title="category?.label"
                        popup-content-class="modal-popup transaction-modal-category-popup"
                        :popup-content-style="categoryPopupStyle"
                        options-selected-class="modal-popup-selected"
                        options-cover
                        @focus="onCategoryFocus"
                        @blur="restoreCategoryIfEmpty"
                        @popup-show="setCategoryPopupWidth"
                        @popup-hide="syncCategoryInputValue"
                        @update:input-value="onCategoryInputValueUpdate"
                        @update:model-value="blurSelect('categorySelect')"
                        @new-value="createCategory" @filter="filterCategories"
                        @filter-abort="filterCategoriesAbort" :options="categoriesOptions"
                        :label="$t('modals.transaction.form.category.label')"
                        :rules="[val => !!val || $t('modals.transaction.form.category.validation.required_field')]"
              >
                <template v-slot:before>
                  <q-icon class="form-select-icon transaction-modal-main-category-select-icon"
                          :name="category?.icon || 'pending'"></q-icon>
                </template>
                <template v-slot:option="scope">
                  <q-item v-bind="scope.itemProps">
                    <q-item-section>
                      <q-item-label class="econumo-truncate transaction-modal-select-name">
                        {{ scope.opt.label }}
                      </q-item-label>
                    </q-item-section>
                  </q-item>
                </template>
                <!--  @thinking -->
                <!--                  <template v-slot:append>-->
                <!--                    <q-icon class="form-select-icon transaction-modal-main-category-select-icon" name="close" @click.stop="category = ''" v-show="category"/>-->
                <!--                  </template>-->
              </q-select>
            </div>

            <div class="transaction-modal-main-from-account" v-if="isTransfer">
              <q-select class="form-select transaction-modal-main-from-account-select" outlined bottom-slots
                        v-model="account"
                        :disable="!isTransactionModalCreation"
                        ref="fromAccountSelect"
                        :title="account?.name"
                        popup-content-class="modal-popup"
                        :popup-content-style="fromAccountPopupStyle"
                        options-selected-class="modal-popup-selected"
                        @popup-show="setFromAccountPopupWidth"
                        :options="accountsOptions" :label="$t('modals.transaction.form.from.label')"
                        :option-disable="opt => opt.value === transactionModalAccountRecipientId">
                <template v-slot:before>
                  <q-icon class="form-select-icon transaction-modal-main-from-account-select-icon"
                          :name="account?.icon || 'pending'"></q-icon>
                </template>
                <template v-slot:after>
                  <q-icon class="form-select-icon transaction-modal-main-from-account-select-icon after cursor-pointer"
                          @click="swapAccounts()" :name="'swap_vert'"></q-icon>
                </template>
                <template v-slot:selected-item="scope">
                  <div class="transaction-modal-select-value">
                    <span class="econumo-truncate transaction-modal-select-name">
                      {{ scope.opt.name || scope.opt.label }}
                    </span>
                    <span class="transaction-modal-select-balance">
                      {{ scope.opt.balanceLabel }}
                    </span>
                  </div>
                </template>
                <template v-slot:option="scope">
                  <q-item v-bind="scope.itemProps">
                    <q-item-section>
                      <q-item-label class="transaction-modal-select-value">
                        <span class="econumo-truncate transaction-modal-select-name">
                          {{ scope.opt.name || scope.opt.label }}
                        </span>
                        <span class="transaction-modal-select-balance">
                          {{ scope.opt.balanceLabel }}
                        </span>
                      </q-item-label>
                    </q-item-section>
                  </q-item>
                </template>
              </q-select>
            </div>

            <div class="transaction-modal-main-recipient" v-if="isTransfer">
              <q-select class="form-select transaction-modal-main-recipient-select" outlined bottom-slots
                        v-model="accountRecipient"
                        ref="recipientAccountSelect"
                        :title="accountRecipient?.label"
                        popup-content-class="modal-popup"
                        :popup-content-style="recipientAccountPopupStyle"
                        options-selected-class="modal-popup-selected"
                        @popup-show="setRecipientAccountPopupWidth"
                        :options="accountsOptions" :label="$t('modals.transaction.form.to.label')"
                        :option-disable="opt => opt.value === transactionModalAccountId">
                <template v-slot:before>
                  <q-icon class="form-select-icon transaction-modal-main-recipient-select-icon"
                          :name="accountRecipient?.icon || 'pending'"></q-icon>
                </template>
                <template v-slot:selected-item="scope">
                  <div class="transaction-modal-select-value">
                    <span class="econumo-truncate transaction-modal-select-name">
                      {{ scope.opt.name || scope.opt.label }}
                    </span>
                    <span class="transaction-modal-select-balance">
                      {{ scope.opt.balanceLabel }}
                    </span>
                  </div>
                </template>
                <template v-slot:option="scope">
                  <q-item v-bind="scope.itemProps">
                    <q-item-section>
                      <q-item-label class="transaction-modal-select-value">
                        <span class="econumo-truncate transaction-modal-select-name">
                          {{ scope.opt.name || scope.opt.label }}
                        </span>
                        <span class="transaction-modal-select-balance">
                          {{ scope.opt.balanceLabel }}
                        </span>
                      </q-item-label>
                    </q-item-section>
                  </q-item>
                </template>
              </q-select>
            </div>
          </q-card-section>

          <q-card-section class="transaction-modal-options">
            <div class="transaction-modal-options-title">{{ $t('modals.transaction.form.options.header') }}</div>
            <div>
              <q-input class="transaction-modal-options-input" v-model="description" autogrow
                       :placeholder="$t('modals.transaction.form.description.placeholder')"
                       :label="$t('modals.transaction.form.description.label')"/>
            </div>

            <div class="transaction-modal-options-payee" v-if="!isTransfer">
              <q-select class="transaction-modal-options-payee-select form-select" outlined v-model="payee"
                        :use-input="canChangeAccountData" input-debounce="0"
                        :input-value="payeeInputValue"
                        ref="payeeSelect"
                        :title="payee?.label"
                        popup-content-class="modal-popup transaction-modal-payee-popup"
                        :popup-content-style="payeePopupStyle"
                        options-selected-class="modal-popup-selected"
                        options-cover
                        clearable
                        @keydown="onPayeeKeydown"
                        @blur="restorePayeeIfEmpty"
                        @popup-show="setPayeePopupWidth"
                        @popup-hide="syncPayeeInputValue"
                        @update:input-value="onPayeeInputValueUpdate"
                        @clear="onPayeeClear"
                        @update:model-value="onPayeeModelValueUpdate"
                        @new-value="createPayee" @filter="filterPayees"
                        @filter-abort="filterPayeesAbort" :options="payeesOptions"
                        :label="$t('modals.transaction.form.payee.' + transactionType)">
                <template v-slot:option="scope">
                  <q-item v-bind="scope.itemProps">
                    <q-item-section>
                      <q-item-label class="econumo-truncate transaction-modal-select-name">
                        {{ scope.opt.label }}
                      </q-item-label>
                    </q-item-section>
                  </q-item>
                </template>
                <!--                <template v-slot:append>-->
                <!--                  <q-icon name="close" @click.stop="payee = ''" class="cursor-pointer" v-show="payee"/>-->
                <!--                </template>-->
              </q-select>
            </div>

            <div class="transaction-modal-options-tags" v-if="isExpense">
              <q-btn class="transaction-modal-options-tags-tag-btn" @click="selectTag(item.id)"
                     :title="item.name"
                     :style="(transactionModalTagId === item.id ? 'background: #F4CFFF; color: #93489F' : 'background: #F5F5F5; color: #666666')"
                     :ripple="false" v-for="item in accountOwnerTags"
                     v-bind:key="item.id">
                <span class="econumo-truncate transaction-modal-tag-label">{{ item.name }}</span>
              </q-btn>
              <q-btn class="transaction-modal-options-tags-tag-add-btn" round unelevated :ripple="false"
                     @click="openAddTagModal" v-if="canChangeAccountData">
                <q-avatar class="transaction-modal-options-tags-tag-add-btn-icon">
                  <img src="~assets/add-tad-btn.svg"  alt=""/>
                </q-avatar>
              </q-btn>
            </div>
          </q-card-section>

          <q-card-actions class="transaction-modal-actions">
            <q-btn class="econumo-btn -medium -grey transaction-modal-actions-btn" flat
                   :label="$t('elements.button.cancel.label')" v-close-popup @close="closeModal"/>
            <q-btn class="econumo-btn -medium -magenta transaction-modal-actions-btn" flat
                   :label="$t('elements.button.add.label')" type="submit"
                   v-if="isTransactionModalCreation"/>
            <q-btn class="econumo-btn -medium -magenta transaction-modal-actions-btn" flat
                   :label="$t('elements.button.update.label')" type="submit"
                   v-if="!isTransactionModalCreation"/>
          </q-card-actions>
        </q-form>
      </q-card>
    </q-dialog>

    <q-dialog v-model="isAddTag" minimized @hide="closeAddTagModal">
      <div class="econumo-modal transaction-modal-add-tag">
        <q-select class="form-select -narrow full-width" outlined bottom-slots v-model="tag" :use-input="true"
                  popup-content-class="modal-popup"
                  options-selected-class="modal-popup-selected"
                  @new-value="createTagFromSelect" @filter="filterTags"
                  @filter-abort="filterTagsAbort" :options="tagsOptions"
                  :label="$t('modals.transaction.dialog.new_tag.name.label')"
                  :rules="[
                    val => !!val || $t('modals.transaction.dialog.new_tag.name.validation.required_field'),
                  ]"
        >
          <template v-slot:option="scope">
            <q-item v-bind="scope.itemProps">
              <q-item-section>
                <q-item-label class="econumo-truncate transaction-modal-select-name">
                  {{ scope.opt.label }}
                </q-item-label>
              </q-item-section>
            </q-item>
          </template>
        </q-select>
        <div class="transaction-modal-add-tag-actions">
          <q-btn class="econumo-btn -medium -grey transaction-modal-add-tag-button"
                 :label="$t('elements.button.cancel.label')" type="button" @click="closeAddTagModal"
                 flat/>
          <q-btn class="econumo-btn -medium -magenta transaction-modal-add-tag-button"
                 :label="$t('elements.button.add.label')" type="button" @click="createNewTag"
                 flat/>
        </div>
      </div>
    </q-dialog>
  </teleport>
</template>

<script>
import {defineComponent} from 'vue'
import {useValidation} from '../composables/useValidation';
import {useMoney} from '../composables/useMoney';
import {useDecimalNumber} from '../composables/useDecimalNumber';
import {useCurrency} from '../composables/useCurrency';
import _ from 'lodash';
import {useAccountsStore} from '../stores/accounts';
import {useTransactionModalStore} from '../stores/transaction-modal';
import {useTransactionsStore} from '../stores/transactions';
import {useCategoriesStore} from '../stores/categories';
import {useTagsStore} from '../stores/tags';
import {usePayeesStore} from '../stores/payees';
import {useUsersStore} from '../stores/users';
import {mapState} from 'pinia';
import {useAvatar} from '../composables/useAvatar';
import { useCurrencyRatesStore } from '../stores/currency-rates';
import { useAccountFoldersStore } from '../stores/account-folders';
import CalculatorInput from './Calculator/CalculatorInput.vue';
import {date} from 'quasar';

export default defineComponent({
  components: { CalculatorInput },
  name: 'TransactionModal',
  setup() {
    const { moneyFormat } = useMoney();
    const { formatNumber, isScientificNotation } = useDecimalNumber();
    const validation = useValidation();
    const { avatarUrl } = useAvatar();
    const { exchange } = useCurrency();

    return {
      moneyFormat,
      formatNumber,
      isScientificNotation,
      ...validation,
      avatarUrl,
      exchange
    };
  },
  data() {
    return {
      categorySearchFilter: '',
      tagSearchFilter: '',
      payeeSearchFilter: '',
      payeeInputValue: '',
      categoryInputValue: '',
      lastSelectedCategory: null,
      lastSelectedPayee: null,
      isPayeeCleared: false,
      skipPayeeRestoreOnce: false,
      payeeTypingStarted: false,
      isAddTag: false,
      tag: '',
      customTags: null,
      accountPopupStyle: '',
      fromAccountPopupStyle: '',
      recipientAccountPopupStyle: '',
      categoryPopupStyle: '',
      payeePopupStyle: ''
    }
  },
  computed: {
    ...mapState(useUsersStore, ['userId']),
    ...mapState(useCurrencyRatesStore, ['currencyRates']),
    ...mapState(useTagsStore, ['tags']),
    ...mapState(useCategoriesStore, ['categoriesOrdered']),
    ...mapState(useTransactionModalStore, [
      'isTransactionModalOpened',
      'isTransactionModalCreation',
      'transactionModalOpenDate',
      'transactionModalAccountId',
      'transactionModalAmount',
      'transactionModalAccountRecipientId',
      'transactionModalAmountRecipient',
      'transactionModalCategoryId',
      'transactionModalDescription',
      'transactionModalPayeeId',
      'transactionModalTagId',
      'transactionModalType',
      'transactionModalId']),
    ...mapState(useTransactionModalStore, {accountOwnerId: 'transactionModalUserId'}),
    ...mapState(useAccountsStore, {accounts: 'accountsOrdered'}),
    ...mapState(usePayeesStore, {availablePayees: 'payeesOrdered'}),
    ...mapState(useTagsStore, {availableTags: 'tagsOrdered'}),
    ...mapState(useAccountFoldersStore, {folders: 'accountFoldersOrdered'}),
    isModalOpened: {
      get() {
        return this.isTransactionModalOpened;
      },
      set(value) {
        if (value) {
          useTransactionModalStore().openTransactionModal({type: this.transactionType});
        } else {
          useTransactionModalStore().closeTransactionModal();
        }
      },
    },
    transactionDate: {
      get() {
        const obj = date.extractDate(this.transactionModalOpenDate, 'YYYY-MM-DD HH:mm:ss')
        return date.formatDate(obj, 'YYYY-MM-DD');
      },
      set(value) {
        const obj = date.extractDate(value, 'YYYY-MM-DD')
        useTransactionModalStore().changeTransactionModalOpenDate(date.formatDate(obj, 'YYYY-MM-DD HH:mm:ss'));
      },
    },
    transactionType: {
      get() {
        return this.transactionModalType;
      },
      set(value) {
        if (this.$refs.transactionForm) {
          this.$refs.transactionForm.resetValidation();
        }
        useTransactionModalStore().changeTransactionModalType(value);
        useTransactionModalStore().changeTransactionModalCategory(null);
        this.categoryInputValue = '';
        this.categorySearchFilter = '';
        this.lastSelectedCategory = null;
        this.calculateAmountRecipient(this.amount);
        this.focusAmountInput();
      },
    },
    account: {
      get() {
        const account = _.find(this.accounts, {id: this.transactionModalAccountId});
        return {
          name: account.name,
          label: account.name,
          value: this.transactionModalAccountId,
          icon: account.icon,
          balanceLabel: this.moneyFormat(account.balance, account.currency.id, true, true),
          currencySign: account.currency.symbol,
          currencyId: account.currency.id
        };
      },
      set(item) {
        useTransactionModalStore().changeTransactionModalAccount(item.value);
        if (this.isTransactionModalCreation) {
          this.calculateAmountRecipient(this.amount);
        }
      },
    },
    accountRecipient: {
      get() {
        const account = _.find(this.accounts, {id: this.transactionModalAccountRecipientId});
        if (!account) {
          return null
        }
        return {
          label: account.name,
          value: this.transactionModalAccountRecipientId,
          icon: account.icon,
          balanceLabel: this.moneyFormat(account.balance, account.currency.id, true, true),
          currencySign: account.currency.symbol,
          currencyId: account.currency.id
        };
      },
      set(item) {
        useTransactionModalStore().changeTransactionModalAccountRecipient(item.value);
        this.calculateAmountRecipient(this.amount);
      },
    },
    amount: {
      get() {
        return this.transactionModalAmount;
      },
      set(value) {
        useTransactionModalStore().changeTransactionModalAmount(value);
        this.calculateAmountRecipient(value);
      },
    },
    amountRecipient: {
      get() {
        return this.transactionModalAmountRecipient;
      },
      set(value) {
        useTransactionModalStore().changeTransactionModalAmountRecipient(value);
      },
    },
    category: {
      get() {
        const category = _.find(this.categories, {id: this.transactionModalCategoryId});
        if (!category) {
          return null;
        }
        return {
          label: category.name,
          value: category.id,
          icon: category.icon,
        };
      },
      set(item) {
        useTransactionModalStore().changeTransactionModalCategory(item ? item.value : null);
        this.categoryInputValue = item?.label || '';
        this.lastSelectedCategory = item ? {label: item.label, value: item.value} : this.lastSelectedCategory;
      },
    },
    description: {
      get() {
        return this.transactionModalDescription
      },
      set(value) {
        useTransactionModalStore().changeTransactionModalDescription(value);
      },
    },
    payee: {
      get() {
        const payee = _.find(this.payees, {id: this.transactionModalPayeeId});
        if (!payee) {
          return null;
        }
        return {
          label: payee.name,
          value: payee.id
        };
      },
      set(item) {
        useTransactionModalStore().changeTransactionModalPayee(item ? item.value : null);
        this.payeeInputValue = item?.label || '';
        this.lastSelectedPayee = item ? {label: item.label, value: item.value} : this.lastSelectedPayee;
      },
    },
    accountsOptions: function () {
      let result = [];
      let foldersVisibility = {};
      this.folders.forEach(item => {
        foldersVisibility[item.id] = !!item.isVisible;
      })
      this.accounts.forEach(item => {
        if (this.isTransactionModalCreation && !foldersVisibility[item.folderId]) {
          return;
        }

        result.push({
          label: item.name,
          name: item.name,
          balanceLabel: this.moneyFormat(item.balance, item.currency.id, true, true),
          value: item.id,
          icon: item.icon,
          currencySign: item.currency.symbol,
        })
      })
      return result;
    },
    categoriesOptions: function () {
      let result = [];
      this.categories.forEach(item => {
        if (!this.categorySearchFilter || item.name.toLowerCase().indexOf(this.categorySearchFilter) > -1) {
          result.push({
            label: item.name,
            value: item.id,
            icon: item.icon
          })
        }
      })
      return result;
    },
    tagsOptions: function () {
      let result = [];
      _.filter(this.tags, {ownerUserId: this.accountOwnerId}).forEach(item => {
        if (!this.tagSearchFilter || item.name.toLowerCase().indexOf(this.tagSearchFilter) > -1) {
          result.push({
            label: item.name,
            value: item.id
          })
        }
      })
      return result;
    },
    payeesOptions: function () {
      let result = [];
      this.payees.forEach(item => {
        if (!this.payeeSearchFilter || item.name.toLowerCase().indexOf(this.payeeSearchFilter) > -1) {
          result.push({
            label: item.name,
            value: item.id
          })
        }
      })
      return result;
    },
    isTransfer: function () {
      return this.transactionModalType === 'transfer';
    },
    isExpense: function () {
      return this.transactionModalType === 'expense';
    },
    categories: function () {
      return _.filter(this.categoriesOrdered, {type: this.transactionType, ownerUserId: this.accountOwnerId})
    },
    payees: function () {
      return _.filter(this.availablePayees, {ownerUserId: this.accountOwnerId})
    },
    accountOwnerTags: {
      get() {
        if (this.customTags) {
          return this.customTags;
        }

        let tags = _.filter(this.availableTags, {isArchived: 0, ownerUserId: this.accountOwnerId});
        if (this.transactionModalTagId) {
          let isFound = !!_.findLast(tags, {id: this.transactionModalTagId});
          if (!isFound) {
            const foundTag = _.find(this.tags, {id: this.transactionModalTagId});
            if (foundTag) {
              tags.push(foundTag);
            }
          }
        }
        return tags;
      },
      set(items) {
        this.customTags = items;
      },
    },
    canChangeAccountData: function () {
      if (!this.transactionModalAccountId) {
        return false;
      }
      const account = _.find(this.accounts, {id: this.transactionModalAccountId});
      if (account.owner.id === this.userId) {
        return true;
      }
      let isAdmin = false;
      account.sharedAccess.forEach((item) => {
        if (item.user?.id === this.userId && item.role === 'admin') {
          isAdmin = true;
        }
      });
      return isAdmin;
    }
  },
  methods: {
    focusAmountInput() {
      this.$nextTick(() => {
        const amountInput = this.$refs.amountInput;
        const el = amountInput?.$el || amountInput;
        const input = el?.querySelector?.('input');
        if (input) {
          input.focus();
        }
      });
    },
    setPopupWidth(selectRef, styleKey, popupClass = '') {
      const selectEl = this.$refs[selectRef]?.$el;
      if (!selectEl) {
        return;
      }
      let widthSource = selectEl.querySelector('.q-field__control') || selectEl;
      if (popupClass) {
        const popupEl = document.querySelector(`.${popupClass}`);
        const dialogEl = popupEl?.closest('.q-select__dialog');
        if (dialogEl) {
          this[styleKey] = 'width: 100% !important; max-width: 100% !important;';
          return;
        }
      }
      const width = Math.max(0, Math.round(widthSource.getBoundingClientRect().width));
      this[styleKey] = `width: ${width}px; max-width: ${width}px;`;
    },
    setCategoryPopupWidth() {
      this.setPopupWidth('categorySelect', 'categoryPopupStyle', 'transaction-modal-category-popup');
    },
    onCategoryFocus() {
      if (!this.canChangeAccountData || !this.category?.label) {
        return;
      }
      this.categoryInputValue = '';
      useTransactionModalStore().changeTransactionModalCategory(null);
    },
    onCategoryInputValueUpdate(value) {
      this.categoryInputValue = value;
      this.categorySearchFilter = value.toLowerCase();
      if (this.category && value && value !== this.category.label) {
        useTransactionModalStore().changeTransactionModalCategory(null);
      }
    },
    syncCategoryInputValue() {
      this.$nextTick(() => {
        if (this.category?.label) {
          this.categoryInputValue = this.category.label;
          this.resetSelectInputScroll('categorySelect');
        }
      });
    },
    restoreCategoryIfEmpty() {
      if (this.category || this.categoryInputValue) {
        return;
      }
      if (this.lastSelectedCategory) {
        useTransactionModalStore().changeTransactionModalCategory(this.lastSelectedCategory.value);
        this.categoryInputValue = this.lastSelectedCategory.label;
      }
    },
    setPayeePopupWidth() {
      this.setPopupWidth('payeeSelect', 'payeePopupStyle', 'transaction-modal-payee-popup');
    },
    onPayeeInputValueUpdate(value) {
      if (this.payee && value !== this.payee.label) {
        this.payeeTypingStarted = true;
      }
      this.payeeInputValue = value;
      this.payeeSearchFilter = value.toLowerCase();
      this.isPayeeCleared = false;
      if (this.payee && value && value !== this.payee.label) {
        useTransactionModalStore().changeTransactionModalPayee(null);
      }
    },
    onPayeeModelValueUpdate(value) {
      if (value === null) {
        this.onPayeeClear();
        return;
      }
      this.isPayeeCleared = false;
      this.payeeTypingStarted = false;
      this.blurSelect('payeeSelect');
    },
    onPayeeClear() {
      this.isPayeeCleared = true;
      this.skipPayeeRestoreOnce = true;
      this.payeeTypingStarted = false;
      this.payeeInputValue = '';
      this.payeeSearchFilter = '';
      this.lastSelectedPayee = null;
      useTransactionModalStore().changeTransactionModalPayee(null);
      this.$nextTick(() => {
        this.blurSelect('payeeSelect');
      });
    },
    onPayeeKeydown(event) {
      if (!this.payee || this.payeeTypingStarted) {
        return;
      }
      const isInputKey = event.key.length === 1 || event.key === 'Backspace' || event.key === 'Delete';
      if (!isInputKey) {
        return;
      }
      this.payeeTypingStarted = true;
      this.payeeInputValue = '';
      this.payeeSearchFilter = '';
      useTransactionModalStore().changeTransactionModalPayee(null);
      this.$nextTick(() => {
        const input = this.$refs.payeeSelect?.$el?.querySelector('input');
        if (input && typeof input.setSelectionRange === 'function') {
          input.value = '';
          input.setSelectionRange(0, 0);
        }
      });
    },
    syncPayeeInputValue() {
      this.$nextTick(() => {
        if (this.payee?.label) {
          this.payeeInputValue = this.payee.label;
          this.resetSelectInputScroll('payeeSelect');
        }
      });
    },
    restorePayeeIfEmpty() {
      if (this.skipPayeeRestoreOnce) {
        this.skipPayeeRestoreOnce = false;
        return;
      }
      if (this.isPayeeCleared || this.payee || this.payeeInputValue) {
        return;
      }
      if (this.lastSelectedPayee) {
        useTransactionModalStore().changeTransactionModalPayee(this.lastSelectedPayee.value);
        this.payeeInputValue = this.lastSelectedPayee.label;
      }
    },
    resetSelectInputScroll(refName) {
      this.$nextTick(() => {
        const input = this.$refs[refName]?.$el?.querySelector('input');
        const nativeEl = this.$refs[refName]?.$el?.querySelector('.q-field__native');
        if (input) {
          input.scrollLeft = 0;
          if (typeof input.setSelectionRange === 'function') {
            input.setSelectionRange(0, 0);
          }
        }
        if (nativeEl) {
          nativeEl.scrollLeft = 0;
        }
      });
    },
    blurSelect(refName) {
      this.$nextTick(() => {
        this.$refs[refName]?.blur?.();
      });
    },
    setAccountPopupWidth() {
      this.setPopupWidth('accountSelect', 'accountPopupStyle');
    },
    setFromAccountPopupWidth() {
      this.setPopupWidth('fromAccountSelect', 'fromAccountPopupStyle');
    },
    setRecipientAccountPopupWidth() {
      this.setPopupWidth('recipientAccountSelect', 'recipientAccountPopupStyle');
    },
    calculateAmountRecipient: function (value) {
      if (!this.accountRecipient || !value || isNaN(value)) {
        this.amountRecipient = value;
        return;
      }
      let amount = this.exchange(this.account.currencyId, this.accountRecipient.currencyId, value);
      if (this.isScientificNotation(amount)) {
        amount = this.formatNumber(amount, 8, false);
      }
      this.amountRecipient = amount;
    },
    createCategory: function (name, done) {
      if (!this.canChangeAccountData) {
        done();
        return;
      }
      if (!this.isValidCategoryName(name)) {
        return;
      }
      useCategoriesStore().createCategory({
        name: name,
        type: this.transactionModalType,
        accountId: this.transactionModalAccountId
      }).then(createdCategory => {
        useTransactionModalStore().changeTransactionModalCategory(createdCategory.id);
        done({
          label: createdCategory.name,
          value: createdCategory.id,
          icon: createdCategory.icon
        });
      });
    },
    filterCategories: function (val, update) {
      update(() => {
        this.categorySearchFilter = val.toLowerCase();
      })
    },
    filterCategoriesAbort: function () {
      this.categorySearchFilter = '';
    },
    filterTags: function (val, update) {
      update(() => {
        this.tagSearchFilter = val.toLowerCase();
      })
    },
    filterTagsAbort: function () {
      this.tagSearchFilter = '';
    },

    createPayee: function (name, done) {
      if (!this.canChangeAccountData) {
        done();
        return;
      }
      if (!this.isValidPayeeName(name)) {
        return;
      }
      usePayeesStore().createPayee({
        name: name,
        accountId: this.transactionModalAccountId
      }).then(createdPayee => {
        console.log(createdPayee);
        useTransactionModalStore().changeTransactionModalPayee(createdPayee.id);
        done({
          label: createdPayee.name,
          value: createdPayee.id
        });
      });
    },
    filterPayees: function (val, update) {
      update(() => {
        this.payeeSearchFilter = val.toLowerCase();
      })
    },
    filterPayeesAbort: function () {
      this.payeeSearchFilter = '';
    },

    selectTag: function (id) {
      if (this.transactionModalTagId === id) {
        id = null;
      }
      useTransactionModalStore().changeTransactionModalTag(id);
    },
    createTagFromSelect: function (name, done) {
      if (!this.canChangeAccountData) {
        done();
        return;
      }
      if (!this.isValidTagName(name)) {
        return;
      }
      useTagsStore().createTag({name: name, accountId: this.transactionModalAccountId}).then(createdTag => {
        this.selectTag(createdTag.id);
        done({
          label: createdTag.name,
          value: createdTag.id
        });
        this.closeAddTagModal();
      });
    },
    createNewTag: function () {
      if (!this.canChangeAccountData) {
        done();
        this.closeAddTagModal();
        return;
      }
      this.closeAddTagModal();
      const foundTag = _.find(this.tags, {id: this.tag.value});
      if (_.find(this.accountOwnerTags, {id: foundTag.id})) {
        this.selectTag(foundTag.id);
        return;
      }
      if (foundTag) {
        let tmp = _.cloneDeep(this.accountOwnerTags);
        tmp.push(foundTag);
        this.accountOwnerTags = tmp;
        this.selectTag(foundTag.id);
      } else {
        useTagsStore().createTag({
          name: this.tag.label,
          accountId: this.transactionModalAccountId
        }).then(createdTag => {
          useTransactionModalStore().changeTransactionModalTag(createdTag.id);
        });
      }
    },
    submit: function () {
      const form = {
        id: this.transactionModalId,
        type: this.transactionModalType,
        amount: Number(this.transactionModalAmount),
        amountRecipient: this.transactionModalAmountRecipient === null ? null : Number(this.transactionModalAmountRecipient),
        accountId: this.transactionModalAccountId,
        accountRecipientId: this.transactionModalAccountRecipientId,
        categoryId: this.transactionModalCategoryId,
        description: this.transactionModalDescription || '',
        payeeId: this.transactionModalPayeeId,
        tagId: this.transactionModalTagId,
        date: this.transactionModalOpenDate
      };
      if (this.isTransactionModalCreation) {
        useTransactionsStore().createTransaction(form).then(() => {
          this.closeModal();
          if (form.type === 'transfer') {
            useTransactionModalStore().changeTransactionModalSwitchAccount(form.accountRecipientId);
          }
        });
      } else {
        useTransactionsStore().updateTransaction(form).then(() => {
          this.closeModal();
        });
      }
    },
    closeModal: function () {
      if (this.$refs.transactionForm) {
        this.$refs.transactionForm.resetValidation();
      }
      this.categorySearchFilter = '';
      this.tagSearchFilter = '';
      this.payeeSearchFilter = '';
      this.tag = '';
      this.customTags = null;
      useTransactionModalStore().closeTransactionModal();
    },
    openAddTagModal() {
      this.isAddTag = true;
    },
    closeAddTagModal() {
      this.isAddTag = false;
    },
    swapAccounts() {
      if (this.account && this.accountRecipient) {
        const account = this.account;
        this.account = this.accountRecipient;
        this.accountRecipient = account;
      } else if (this.account && !this.accountRecipient) {
        this.accountRecipient = _.cloneDeep(this.account);
        this.account = null;
      } else if (!this.account && this.accountRecipient) {
        this.account = _.cloneDeep(this.accountRecipient);
        this.accountRecipient = null;
      }
    },
    previousDay() {
      const newDate = date.subtractFromDate(date.extractDate(this.transactionDate, 'YYYY-MM-DD'), {hours: 24});
      this.transactionDate = date.formatDate(newDate, 'YYYY-MM-DD');
    }
  }
})
</script>
