<template>
  <q-page v-if="account">
    <div class="account column">
      <!-- toolbar for mobile -->
      <div class="account-toolbar-mobile">
        <div class="account-toolbar-mobile-head">
          <div>
            <q-btn class="account-toolbar-mobile-head-button" flat icon="arrow_back"
                   @click="navigateTo('home', true)" />
          </div>
          <div>
            <h4 class="account-toolbar-mobile-head-title econumo-truncate" :title="account.name">{{ account.name }}</h4>
          </div>
          <div>
            <q-btn class="account-toolbar-mobile-head-button" flat icon="settings" @click="openAccountSettingsModal()"
                   v-if="canUpdateAccountSettings" />
          </div>
        </div>
        <div class="account-toolbar-mobile-balance">
          <div class="account-toolbar-mobile-balance-container">
            <q-icon class="account-toolbar-mobile-balance-icon" :name="account.icon" />
            <span
              class="account-toolbar-mobile-balance-currency">{{ moneyFormat(account.balance, account.currency.id, true, false)
              }}</span>
          </div>
          <div class="account-toolbar-mobile-balance-shared" v-if="account.sharedAccess.length > 0">
            <q-avatar class="account-toolbar-mobile-balance-shared-avatar">
              <img :src="account.owner.avatar + '?s=30'" />
            </q-avatar>
            <q-avatar class="account-toolbar-mobile-balance-shared-avatar" v-for="access in account.sharedAccess"
                      v-bind:key="access.user.id">
              <img :src="access.user.avatar + '?s=30'" />
            </q-avatar>
          </div>
        </div>
        <div class="account-toolbar-mobile-search">
          <q-input class="form-input -narrow full-width" outlined v-model="search"
                   :label="$t('pages.account.toolbar.search')" />
        </div>
      </div>

      <!-- toolbar for desktop -->
      <div class="account-toolbar-desktop">
        <div class="account-toolbar-desktop-head">
          <h4 class="account-toolbar-desktop-head-title">
            <span class="account-toolbar-desktop-head-name econumo-truncate" :title="account.name">{{ account.name }}</span>
            <q-icon class="account-toolbar-desktop-head-icon" :name="account.icon" />
          </h4>
        </div>
        <div class="account-toolbar-desktop-currency">
          <div class="account-toolbar-desktop-currency-balance">
            <div class="account-toolbar-desktop-currency-balance-check">
              {{ moneyFormat(account.balance, account.currency.id, true, false) }}
            </div>
            <div class="account-toolbar-desktop-currency-shared" v-if="account.sharedAccess.length > 0">
              <!--              <q-icon class="account-toolbar-desktop-currency-shared-icon" name="people" />-->
              <!--              <div class="account-toolbar-desktop-currency-shared-note">{{ $t('pages.account.toolbar.shared_with') }}</div>-->
              <q-avatar class="account-toolbar-desktop-currency-shared-avatar">
                <img :src="account.owner.avatar + '?s=30'" :alt="account.owner.name" :title="account.owner.name" />
              </q-avatar>
              <q-avatar class="account-toolbar-desktop-currency-shared-avatar" v-for="access in account.sharedAccess"
                        v-bind:key="access.user.id">
                <img :src="access.user.avatar + '?s=30'" :title="access.user.name" :alt="access.user.name" />
              </q-avatar>
            </div>
          </div>
          <div>
            <a class="account-toolbar-desktop-currency-settings" href="" @click.prevent="openAccountSettingsModal"
               v-if="canUpdateAccountSettings">{{ $t('pages.account.toolbar.settings') }}</a>
          </div>
        </div>
      </div>

      <div class="account-toolbar-desktop-control">
        <div v-if="canChangeTransaction">
          <q-btn class="econumo-btn -small -magenta account-toolbar-desktop-control-btn" flat
                 :label="$t('pages.account.transaction_list.action.add_transaction')"
                 @click="openCreateTransactionModal()" />
        </div>
        <div>
          <q-input class="form-input -narrow account-toolbar-desktop-control-search" outlined v-model="search"
                   :label="$t('pages.account.toolbar.search')" />
        </div>
      </div>

      <!-- transactions list -->
      <q-virtual-scroll
        class="account-transactions"
        :items="accountTransactionsDailyList"
        ref="transactionList"
      >
        <template v-slot="{ item }">
          <q-item-label class="account-transactions-date" header v-if="item.isSeparator" :key="'separator-' + item.id">
              <span class="account-transactions-date-content" v-show="item.alias !== 'none'" :key="'alias-' + item.id">{{
                  $t('pages.account.transaction_list.' + (item.alias))
                }}</span>
            <span class="account-transactions-date-content" v-show="item.alias === 'none'" :key="'date-' + item.id">{{ item.date }}</span>
          </q-item-label>

          <q-item :class="'account-transactions-item ' + (item.isInFuture ? 'account-transactions-item-future' : '')" v-else :key="item.id" clickable
                  @click="handleTransactionClick(item)">
            <q-item-section class="account-transactions-item-section">
              <div class="account-transactions-item-container">
                <div class="account-transactions-item-info-avatar">
                  <q-icon class="account-transactions-item-info-avatar-icon"
                          :name="(item.type === 'transfer' ? 'sync_alt' : (item.category?.icon || 'question_mark'))" />
                  <q-avatar class="account-transactions-item-info-avatar-shared"
                            v-if="account.sharedAccess.length  > 0">
                    <img :src="item.author.avatar + '?s=30'" :title="item.author.name" :alt="item.author.name" />
                  </q-avatar>
                </div>
                <div class="account-transactions-item-info-description">
                  <div class="account-transactions-item-info-row">
                  <div class="account-transactions-item-info-description-category econumo-truncate" v-if="transactionTitleInfo(item).text" :title="transactionTitleInfo(item).text">
                    {{ transactionTitleInfo(item).text }}
                  </div>
                    <div class="account-transactions-item-check-amount">
                      <span
                        :class="'account-transactions-item-check-income ' + (isIncome(item) ? 'income-color' : 'expense-color')">{{ transactionDisplayAmount(item)
                        }}</span>
                      <span class="account-transactions-item-check-currency">{{ account.currency.symbol }}</span>
                    </div>
                    <div class="account-transactions-item-check-menu">
                      <q-btn square flat icon="more_vert" class="account-transactions-item-check-button" @click.stop
                             v-if="canChangeTransaction && (item.type !== 'transfer' || (item.type === 'transfer' && item.account && item.accountRecipient))">
                        <q-menu cover auto-close class="account-transactions-item-check-button-menu" :ref="(el) => setMenuRef(el, item.id)">
                          <q-list class="account-transactions-item-check-button-list">
                            <q-item clickable @click="openUpdateTransactionModal(item.id)"
                                    class="account-transactions-item-check-button-item">
                              <q-item-section class="account-transactions-item-check-button-section">
                                {{ $t('elements.button.edit.label') }}
                              </q-item-section>
                            </q-item>
                            <q-item clickable @click="openDeleteTransactionModal(item.id)"
                                    class="account-transactions-item-check-button-item">
                              <q-item-section class="account-transactions-item-check-button-section -delete">
                                {{ $t('elements.button.delete.label') }}
                              </q-item-section>
                            </q-item>
                          </q-list>
                        </q-menu>
                      </q-btn>
                    </div>
                  </div>
                  <div class="account-transactions-item-info-description-note" v-if="item.description && transactionTitleInfo(item).source !== 'description'">{{ item.description }}</div>
                  <div class="account-transactions-item-tags">
                    <div class="account-transactions-item-tags-tag-wrapper" v-if="item.tag">
                      <q-badge class="account-transactions-item-tags-tag econumo-truncate" v-if="transactionTitleInfo(item).source !== 'tag'" :title="item.tag?.name || ''">{{ item.tag?.name || '' }}
                      </q-badge>
                    </div>
                    <div class="account-transactions-item-tags-block" v-if="item.payee">
                      <div class="account-transactions-item-tags-payee econumo-truncate" v-if="transactionTitleInfo(item).source !== 'payee'" :title="item.payee?.name || ''">{{ item.payee?.name || '' }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </q-item-section>
          </q-item>
        </template>
      </q-virtual-scroll>

      <div class="account-transactions-add-mobile" v-if="canChangeTransaction">
        <q-btn class="account-transactions-add-mobile-btn econumo-btn -medium -magenta" flat
               :label="$t('pages.account.transaction_list.action.add_transaction')"
               @click="openCreateTransactionModal()" />
      </div>
    </div>

    <teleport to="body">
      <loading-modal v-if="!isTransactionsLoaded" :header-label="$t('modules.app.modal.loading.data_loading')" />

      <account-modal></account-modal>

      <confirmation-dialog-modal v-if="deleteTransactionModal.isOpened"
                                 :question-label="$t('pages.account.delete_transaction_modal.question')"
                                 :action-label="$t('elements.button.delete.label')"
                                 :cancel-label="$t('elements.button.cancel.label')"
                                 v-on:cancel="closeModal"
                                 v-on:proceed="deleteTransaction(deleteTransactionModal.transaction.id)"
      />


      <!-- preview transaction modal -->
      <view-transaction-modal
        v-if="previewTransactionModal.isOpened"
        :position="$q.screen.gt.md ? 'standard' : 'bottom'"
        :transaction="previewTransactionModal.transaction"
        v-bind="{
          ...(previewTransactionModal.account ? { account: previewTransactionModal.account } : {}),
          ...(previewTransactionModal.accountRecipient ? { accountRecipient: previewTransactionModal.accountRecipient } : {})
        }"
        :can-change-transaction="canChangeTransaction"
        v-on:cancel="closeModal"
        v-on:update="openUpdateTransactionModal"
        v-on:delete="openDeleteTransactionModal"
      />
    </teleport>
  </q-page>
</template>

<script>
import { defineComponent } from 'vue';
import _ from 'lodash';
import { transactionMixin } from '../mixins/transactionMixin';
import { navigationMixin } from '../mixins/navigationMixin';
import AccountModal from '../components/AccountModal';
import LoadingModal from '../components/LoadingModal';
import ConfirmationDialogModal from '../components/ConfirmationDialogModal';
import { viewTransactionModalMixin } from '../mixins/viewTransactionModalMixin';
import ViewTransactionModal from '../components/ViewTransactionModal.vue';
import { mapState } from 'pinia';
import { useUsersStore } from 'stores/users';
import { useTransactionsStore } from 'stores/transactions';
import { useAccountsStore } from 'stores/accounts';
import { useAccountModalStore } from 'stores/account-modal';
import { useTransactionModalStore } from 'stores/transaction-modal';
import { useTagsStore } from 'stores/tags';
import { usePayeesStore } from 'stores/payees';
import { useActiveAreaStore } from 'stores/active-area';
import { useMoney } from '../composables/useMoney';
import { useAccount } from '../composables/useAccount';

export default defineComponent({
  name: 'AccountPage',
  components: { AccountModal, ConfirmationDialogModal, LoadingModal, ViewTransactionModal },
  mixins: [transactionMixin, navigationMixin, viewTransactionModalMixin],

  setup() {
    const { moneyFormat } = useMoney();
    const { accountName } = useAccount();

    const transactionFormat = (type, amount, currencyId) => {
      switch (type) {
        case 'expense':
          return '-' + moneyFormat(amount, currencyId, false, false);
        case 'income':
          return '+' + moneyFormat(amount, currencyId, false, false);
        default:
          return moneyFormat(amount, currencyId, false, false);
      }
    };

    return {
      moneyFormat,
      transactionFormat,
      accountName
    };
  },
  data() {
    return {
      search: '',
      menuRefs: new Map(),
      deleteTransactionModal: {
        isOpened: false,
        transaction: null
      }
    };
  },
  created() {
    if (!this.isTransactionsLoaded) {
      useTransactionsStore().fetchTransactions();
    }
    if (this.$router.currentRoute.value.params.id) {
      useAccountsStore().selectAccount(this.$router.currentRoute.value.params.id);
    }
  },
  mounted() {
    if (this.$router.currentRoute.value.params.id) {
      useAccountsStore().selectAccount(this.$router.currentRoute.value.params.id);
    }
  },
  watch: {
    $route(to, from) {
      if (to.name === 'account' && (from.name !== to.name || from.params.id !== to.params.id)) {
        useAccountsStore().selectAccount(to.params.id);
        this.search = '';
      }
    },
    accountTransactionsDailyList() {
      if (this.$refs.transactionList) {
        this.$refs.transactionList.refresh();
      }
    }
  },
  computed: {
    ...mapState(useUsersStore, ['userId', 'userName', 'userAvatar']),
    ...mapState(useAccountsStore, ['accounts', 'accountsOrdered', 'selectedAccountId', 'selectedAccountUserId']),
    ...mapState(useTransactionsStore, ['isTransactionsLoaded', 'transactionsOrdered']),
    ...mapState(useTagsStore, ['tags']),
    ...mapState(usePayeesStore, ['payees']),
    account: function() {
      return _.find(this.accountsOrdered, { id: this.selectedAccountId });
    },
    transactions: function() {
      if (!this.search) {
        return this.transactionsOrdered;
      }
      let results = this.transactionsOrdered;
      this.search.split(' ').forEach((searchItem) => {
        results = _.filter(results, (item) => {
          return item.search.indexOf(_.toLower(searchItem)) !== -1;
        });
      });

      return results;
    },
    sharedAccess: function() {
      let result = this.account.sharedAccess ?? [];
      if (result.length === 1) {
        result.unshift({
          role: 'admin',
          user: {
            id: this.userId,
            avatar: this.userAvatar,
            name: this.userName
          }
        });
      } else if (result > 3) {
        result = _.slice(result, 0, 3);
      }

      return result;
    },
    canUpdateAccountSettings: function() {
      if (this.account.owner.id === this.userId) {
        return true;
      }
      let isAdmin = false;
      this.account.sharedAccess.forEach((item) => {
        if (item.user?.id === this.userId && item.role === 'admin') {
          isAdmin = true;
        }
      });
      return isAdmin;
    },
    canChangeTransaction: function() {
      if (this.account.owner.id === this.userId) {
        return true;
      }
      let isUser = false;
      this.account.sharedAccess.forEach((item) => {
        if (item.user?.id === this.userId && (item.role === 'admin' || item.role === 'user')) {
          isUser = true;
        }
      });
      return isUser;
    },
    accountTransactionsDailyList: function() {
      return this.transactionsDailyList(this.filterByAccount(this.transactions, this.selectedAccountId));
    }
  },
  methods: {
    setMenuRef: function(el, transactionId) {
      if (el) {
        this.menuRefs.set(transactionId, el);
      }
    },
    handleTransactionClick: function(transaction) {
      if (this.$q.screen.gt.md) {
        const menu = this.menuRefs.get(transaction.id);
        if (menu) {
          menu.show();
        }
        return;
      }
      this.openPreviewTransactionModal(transaction.id);
    },
    openAccountSettingsModal: function() {
      useAccountModalStore().openAccountModal(this.account);
    },
    transactionDisplayAmount: function(transaction) {
      if (transaction.type === 'transfer') {
        if (transaction.accountId === this.selectedAccountId) {
          return this.transactionFormat('expense', transaction.amount, transaction.account.currency.id);
        } else {
          return this.transactionFormat('income', transaction.amountRecipient, transaction.accountRecipient.currency.id);
        }
      }
      return this.transactionFormat(transaction.type, transaction.amount, transaction.account.currency.id);
    },
    isIncome: function(transaction) {
      if (transaction.type === 'transfer') {
        return transaction.accountId !== this.selectedAccountId;
      }
      return transaction.type !== 'expense';
    },
    isTransfer: function(transaction) {
      return transaction.type === 'transfer';
    },
    transactionTitleInfo: function(transaction) {
      if (transaction.type === 'transfer') {
        return {
          text: transaction.accountId !== this.selectedAccountId
            ? this.$t('pages.account.transaction_list.item.transfer_from') + ' ' + (transaction.account?.name || this.$t('elements.account.name_hidden'))
            : this.$t('pages.account.transaction_list.item.transfer_to') + ' ' + (transaction.accountRecipient?.name || this.$t('elements.account.name_hidden')),
          source: 'transfer'
        };
      }

      const categoryName = transaction.category?.name || '';
      if (categoryName) {
        return { text: categoryName, source: 'category' };
      }

      const description = transaction.description || '';
      if (description) {
        return { text: description, source: 'description' };
      }

      const tagName = transaction.tag?.name || '';
      if (tagName) {
        return { text: tagName, source: 'tag' };
      }

      const payeeName = transaction.payee?.name || '';
      if (payeeName) {
        return { text: payeeName, source: 'payee' };
      }

      return { text: '', source: 'none' };
    },
    openDeleteTransactionModal: function(transactionId) {
      this.closeModal();
      this.deleteTransactionModal.transaction = _.cloneDeep(_.find(this.transactions, { id: transactionId }));
      this.deleteTransactionModal.isOpened = true;
    },
    openCreateTransactionModal: function() {
      useTransactionModalStore().openTransactionModal({ type: 'expense' });
    },
    openUpdateTransactionModal: function(transactionId) {
      this.closeModal();
      const transaction = _.cloneDeep(_.find(this.transactionsOrdered, { id: transactionId }));
      useTransactionModalStore().openTransactionModal(transaction);
    },
    deleteTransaction: function(transactionId) {
      useTransactionsStore().deleteTransaction(transactionId);
      this.closeModal();
    },
    closeModal: function() {
      this.previewTransactionModal.transaction = null;
      this.previewTransactionModal.account = null;
      this.previewTransactionModal.accountRecipient = null;
      this.previewTransactionModal.isOpened = false;
      this.deleteTransactionModal.transaction = null;
      this.deleteTransactionModal.isOpened = false;
    }
  }
});
</script>
