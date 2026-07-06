import { defineStore } from 'pinia';
import AccountsAPIv1 from '../modules/api/v1/account';
import { date } from 'quasar';
import { getChangedElements } from '../modules/helpers';
import { METRICS, trackEvent } from '../modules/metrics';
import _ from 'lodash';
import { useActiveAreaStore } from './active-area';
import { useUsersStore } from './users';
import { useTransactionsStore } from './transactions';
import { useBudgetsStore } from './budgets';
import { useAccountFoldersStore } from './account-folders';
import { AccountListDto, AccountDto, AccountItemDto, AccountUpdateDto, AccountCreateDto } from 'modules/api/v1/dto/account.dto';
import { useLocalStorage } from '@vueuse/core';
import { RemovableRef } from '@vueuse/shared';
import { computed, ComputedRef, Ref, ref } from 'vue';
import { DateString, Id } from '../modules/types';
import { DATE_TIME_FORMAT } from '../modules/constants';
import { StorageKeys } from '../modules/storage';

export const useAccountsStore = defineStore('accounts', () => {
  const accounts = useLocalStorage(StorageKeys.ACCOUNTS, []) as RemovableRef<AccountDto[]>;
  const accountsLoadedAt = useLocalStorage(StorageKeys.ACCOUNTS_LOADED_AT, null) as RemovableRef<DateString | null>;
  const selectedAccountId: Ref<Id | null> = ref(null);
  const selectedAccountUserId: Ref<Id | null> = ref(null);

  const isAccountsLoaded: ComputedRef<boolean> = computed(() => !!accountsLoadedAt.value);
  const ownAccounts = computed(() => {
    const usersStore = useUsersStore();
    return _.orderBy(_.filter(_.cloneDeep(accounts.value), { owner: { id: usersStore.userId } }), 'position', 'asc');
  });
  const accountsOrdered = computed(() => _.orderBy(_.cloneDeep(accounts.value), 'position'));

  function fetchAccounts() {
    return AccountsAPIv1.getList((response: AccountListDto) => {
      updateAccounts(response.items);
    }, (error: any) => {
      console.log('Accounts error', error);
      return error;
    });
  }

  function updateAccounts(items: AccountDto[]) {
    accounts.value = items.map(account => ({
      ...account,
      balance: Number(account.balance)
    }));
    accountsLoadedAt.value = date.formatDate(new Date(), DATE_TIME_FORMAT);
  }

  function selectAccount(id: Id | null) {
    trackEvent(METRICS.ACCOUNT_SELECT);
    const account = _.find(accounts.value, { id: id }) as AccountDto | null;
    if (!account) {
      selectedAccountId.value = null;
      selectedAccountUserId.value = null;
    } else {
      selectedAccountId.value = id;
      selectedAccountUserId.value = account.owner.id;
    }

    const activeArea = useActiveAreaStore();
    if (id) {
      activeArea.setWorkspaceActiveArea();
    } else {
      activeArea.setSidebarActiveArea();
    }
  }

  function createAccount(form: AccountCreateDto) {
    trackEvent(METRICS.ACCOUNT_CREATE);
    const budgetsStore = useBudgetsStore();
    const transactionsStore = useTransactionsStore();
    return AccountsAPIv1.create(form, (response: AccountItemDto) => {
      addAccount(response.item);
      // A non-zero opening balance writes a correction transaction; add it so it
      // shows immediately instead of waiting for the next full sync.
      if (response.transaction) {
        transactionsStore.TRANSACTION_ADDED(response.transaction);
        budgetsStore.resetCachedBudget();
      }
      // A user's very first account makes the backend auto-create a default
      // folder. When the user had no folders, refetch so that folder appears
      // now instead of waiting for the next periodic sync.
      const foldersStore = useAccountFoldersStore();
      if (foldersStore.accountFolders.length === 0) {
        foldersStore.fetchAccountFolders();
      }
    }, (error: any) => {
      console.log('Accounts error', error);
      return error;
    });
  }

  function updateAccount(form: AccountUpdateDto) {
    trackEvent(METRICS.ACCOUNT_UPDATE);
    form.updatedAt = date.formatDate(new Date(), DATE_TIME_FORMAT);
    const budgetsStore = useBudgetsStore();
    const transactionsStore = useTransactionsStore();
    return AccountsAPIv1.update(form, (response: AccountItemDto) => {
      saveAccount(response.item);
      if (response.transaction) {
        transactionsStore.TRANSACTION_ADDED(response.transaction);
        budgetsStore.resetCachedBudget();
      }
    }, (error: any) => {
      console.log('Accounts error', error);
      return error;
    });
  }

  function deleteAccount(id: Id) {
    trackEvent(METRICS.ACCOUNT_DELETE);
    return AccountsAPIv1.delete(id, () => {
      _.remove(accounts.value, (item) => {
        return item.id === id;
      });
      const transactionsStore = useTransactionsStore();
      transactionsStore.TRANSACTIONS_ACCOUNT_DELETE(id);
    }, (error: any) => {
      console.log('Accounts error', error);
      return error;
    });
  }

  function orderAccountList(accountIds: Id[]) {
    trackEvent(METRICS.ACCOUNT_ORDER_LIST);
    const changes = getChangedElements(accounts.value, accountIds, ['position', 'folderId']);
    if (!changes.length) {
      return new Promise((resolve, reject) => {
        reject('No changes');
      });
    }
    return AccountsAPIv1.orderList(changes, (response: AccountListDto) => {
      updateAccounts(response.items);
      return response.items;
    }, (error: any) => {
      console.log('Accounts error', error);
      return error;
    });
  }

  function addAccount(account: AccountDto) {
    const copyAccounts = _.cloneDeep(accounts.value);
    _.remove(copyAccounts, (item) => {
      return item.id === account.id;
    });
    updateAccounts([...copyAccounts, account]);
  }

  function replaceFolder(options: { id: Id, replaceId: Id }) {
    accounts.value.forEach((item) => {
      if (item.folderId === options.id) {
        item.folderId = options.replaceId;
      }
    });
  }

  function accessDelete(options: { accountId: Id, userId: Id }) {
    const account = _.cloneDeep(_.find(accounts.value, { id: options.accountId })) as AccountDto | null;
    if (!account) {
      return;
    }
    _.remove(account.sharedAccess, { user: { id: options.userId } });
    saveAccount(account);
  }

  function accessUpdate(options: any) {
    const account = _.cloneDeep(_.find(accounts.value, { id: options.request.accountId })) as AccountDto | null;
    if (!account) {
      return;
    }
    const connection = _.find(options.connections, { user: { id: options.request.userId } });
    _.remove(account.sharedAccess, { user: { id: options.request.userId } });
    account.sharedAccess.push({
      role: options.request.role,
      user: connection.user
    });
    saveAccount(account);
  }

  function saveAccount(account: AccountDto) {
    _.remove(accounts.value, (item) => {
      return item.id === account.id;
    });
    accounts.value.push(account);
  }

  return {
    accounts,
    accountsLoadedAt,
    selectedAccountId,
    selectedAccountUserId,
    isAccountsLoaded,
    ownAccounts,
    accountsOrdered,
    fetchAccounts,
    selectAccount,
    createAccount,
    updateAccount,
    deleteAccount,
    orderAccountList,
    updateAccounts,
    replaceFolder,
    accessDelete,
    accessUpdate
  };
});
