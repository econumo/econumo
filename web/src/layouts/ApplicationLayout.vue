<template>
  <div class="application">
    <q-layout class="application-layout">
      <!-- sidebar -->
      <component :is="$q.screen.lt.lg ? 'q-pull-to-refresh' : 'div'"
                 :class="'application-layout-sidebar ' + (activeArea === 'sidebar' ? ($q.screen.lt.lg ? 'col-grow' : '') : 'gt-md')"
                 @refresh="refresh">
        <div class="application-layout-logo">
          <object type="image/svg+xml" data="~assets/econumo.svg" width="145" height="15">
            <img src="~assets/econumo.svg" width="145" height="15" alt="Econumo" />
          </object>
          <div class="application-layout-logo-plan" v-if="econumoEdition">{{ econumoEdition }}</div>
        </div>

        <router-link class="application-layout-link" :to="{ name: 'settingsProfile'}">
          <div class="application-layout-user full-width"> <!-- avatar -->
            <div class="row no-wrap inline q-col-gutter-none no-padding">
              <div class="col q-col-gutter-none">
                <q-avatar class="application-layout-user-avatar">
                  <img :src="userAvatar + '?s=100'" class="q-col-gutter-none no-padding" />
                </q-avatar>
              </div>
              <div class="application-layout-user-info">
                <div class="application-layout-user-name" v-if="userName">
                  {{ userName }}
                </div>
                <div class="application-layout-user-login" v-if="userLogin">
                  {{ userLogin }}
                </div>
              </div>
            </div>
          </div>
        </router-link>

        <div class="full-width application-layout-account" v-if="isFullyLoaded"> <!-- account list -->

          <router-link :to="{name: 'onboarding'}" class="application-layout-menu-item" v-if="!userOnboardingCompleted">
            {{ $t('blocks.main.onboarding') }}
          </router-link>

          <router-link :to="{name: 'budget'}" class="application-layout-menu-item" @click="openBudget">
            {{ $t('blocks.main.budget') }}
          </router-link>

          <template v-if="!accountsTree.length && folders.length">
            <q-list class="application-layout-account-gap">
              <q-expansion-item
                class="application-layout-account-folder"
                expand-icon="navigate_next"
                expanded-icon="expand_more"
                :label="mainFolderName"
                :caption="userCurrencyId ? moneyFormat(0, userCurrencyId) : '0'"
                expand-icon-class="application-layout-account-folder-caption"
                :default-opened="true"
              >
                <q-list class="application-layout-account-list">
                  <q-item class="application-layout-account-list-item" clickable 
                          @click="openCreateAccountModal()">
                    <q-item-section class="application-layout-account-list-section-avatar" avatar>
                      <q-icon name="add_card" />
                    </q-item-section>
                    <div class="application-layout-account-list-name-block">
                      <q-item-section class="application-layout-account-list-section-name">
                        {{ $t('blocks.main.create_account') }}
                      </q-item-section>
                      <q-item-section class="application-layout-account-list-section-balance" side>
                        {{ userCurrencyId ? moneyFormat(0, userCurrencyId) : '0' }}
                      </q-item-section>
                    </div>
                  </q-item>
                </q-list>
              </q-expansion-item>
            </q-list>
          </template>

          <q-list class="application-layout-account-gap" v-if="isFullyLoaded">
            <q-expansion-item
              class="application-layout-account-folder"
              expand-icon="navigate_next"
              expanded-icon="expand_more"
              v-for="item in accountsTree" v-bind:key="item.folder.id"
              expand-icon-class="application-layout-account-folder-caption"
              :default-opened="item.folder.opened"
              @show="openAccountFolder(item.folder.id)"
              @hide="closeAccountFolder(item.folder.id)"
            >
              <template v-slot:header>
                <q-item-section>
                  <q-item-label class="application-layout-account-folder-name econumo-truncate" :title="item.folder.name">
                    {{ item.folder.name }}
                  </q-item-label>
                  <q-item-label caption>{{ moneyFormat(item.amount, item.currency.id) }}</q-item-label>
                </q-item-section>
              </template>
              <q-list class="application-layout-account-list">
                <q-item class="application-layout-account-list-item" clickable v-for="account in item.accounts"
                        v-bind:key="account.id" @click="selectAccount(account.id)"
                        :active="account.id === selectedAccountId">
                  <q-item-section class="application-layout-account-list-section-avatar" avatar>
                    <q-icon :name="account.icon" />
                  </q-item-section>
                  <div class="application-layout-account-list-name-block">
                    <q-item-section class="application-layout-account-list-section-name econumo-truncate" :title="account.name">{{ account.name }}
                    </q-item-section>
                    <q-item-section class="application-layout-account-list-section-balance" side>
                      {{ moneyFormat(account.balance, account.currency.id) }}
                    </q-item-section>
                  </div>
                  <q-item-section side v-if="account.sharedAccess.length > 0"
                                  class="application-layout-account-list-section-people">
                    <q-icon name="people" />
                  </q-item-section>
                  <q-item-section side v-if="account.sharedAccess.length > 0"
                                  class="application-layout-account-list-section-shared">
                    <q-avatar class="application-layout-account-list-section-shared-avatar">
                      <img :src="account.owner.avatar + '?s=30'" />
                    </q-avatar>
                    <q-avatar class="application-layout-account-list-section-shared-avatar"
                              v-for="access in account.sharedAccess" v-bind:key="access.user.id">
                      <img :src="access.user.avatar + '?s=30'" />
                    </q-avatar>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-expansion-item>
          </q-list>
        </div>

        <div class="application-layout-footer"> <!-- sidebar footer -->
          <div class="application-layout-footer-social">
            <div class="application-layout-footer-logo">
              <img src="~assets/econumo-gray.svg" width="125" height="20" alt="Econumo" />
              <div class="application-layout-footer-logo-edition" v-if="econumoEdition">{{ econumoEdition }}</div>
            </div>
            <div class="application-layout-footer-social-block">
              <a href="#" @click.prevent="sync" class="application-layout-footer-social-ico -sync"></a>
            </div>
          </div>
          <div class="application-layout-footer-links">
            <div class="application-layout-footer-links-item -settings" @click="openSettings">
              {{ $t('pages.settings.settings.menu_item') }}
            </div>
            <a target="_blank"
                :href="websiteUrl"
                class="application-layout-footer-links-item -website"
                :aria-label="$t('blocks.help.label')"
               v-if="websiteUrl"
              >{{ $t('blocks.website.label') }}</a>
          </div>
        </div>
      </component>

      <!-- workspace -->
      <div :class="'application-layout-workspace ' + (activeArea === 'workspace' ? '' : 'gt-md')">
        <q-page-container>
          <router-view />
        </q-page-container>
      </div>
    </q-layout>

    <teleport to="body">
      <transaction-modal></transaction-modal>
      <account-modal v-if="isFullyLoaded && !accountsTree.length"></account-modal>
      <loading-modal v-if="!isFullyLoaded" :header-label="$t('modules.app.modal.loading.data_loading')" />

      <q-dialog :model-value="!!transactionModalSwitchAccount" seamless position="bottom" class="switch-account-modal" no-backdrop-dismiss>
        <q-card>
          <q-card-section class="row items-center no-wrap">
            <div class="switch-account-modal-text" @click="transactionModalSwitchAccount && selectAccount(transactionModalSwitchAccount)">
              <div class="text-grey">{{ $t('elements.switch_to_account') }}
                <b>{{ transactionModalSwitchAccount ? getAccountName(transactionModalSwitchAccount) : '' }}</b></div>
            </div>
            <q-space />
            <q-btn flat round icon="close" @click="closeModalSwitchAccount()" />
          </q-card-section>
        </q-card>
      </q-dialog>
    </teleport>
  </div>
</template>

<script setup lang="ts">
import TransactionModal from '../components/TransactionModal.vue';
import AccountModal from '../components/AccountModal.vue';
import LoadingModal from '../components/LoadingModal.vue';
import _ from 'lodash';
import { useUsersStore } from 'stores/users';
import { useActiveAreaStore } from 'stores/active-area';
import { useAccountsStore } from 'stores/accounts';
import { useAccountFoldersStore } from 'stores/account-folders';
import { useSyncStore } from 'stores/sync';
import { useTransactionModalStore } from 'stores/transaction-modal';
import { useAccountModalStore } from 'stores/account-modal';
import { getWebsiteUrl } from '../modules/config';
import { econumoPackage } from '../modules/package';
import { useMoney } from '../composables/useMoney';
import { computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useQuasar } from 'quasar';
import type { AccountDto } from '@shared/dto/account.dto';
import type { CurrencyDto } from '@shared/dto/currency.dto';
import { AccountFolderVisibility } from '@shared/dto/account-folder.dto';
import { useCurrency } from '../composables/useCurrency';
import { ref } from 'vue';
const router = useRouter();
const $q = useQuasar();
const { moneyFormat } = useMoney();
const { exchange } = useCurrency();
const usersStore = useUsersStore();
const activeAreaStore = useActiveAreaStore();
const accountsStore = useAccountsStore();
const accountFoldersStore = useAccountFoldersStore();
const syncStore = useSyncStore();
const transactionModalStore = useTransactionModalStore();
const accountModalStore = useAccountModalStore();
const seamless = ref(true);

onMounted(() => {
  syncStore.fetchUpdates();
  document.addEventListener('visibilitychange', () => {
    syncStore.fetchUpdates();
  });
});

const userAvatar = computed(() => usersStore.userAvatar);
const userName = computed(() => usersStore.userName);
const userLogin = computed(() => usersStore.userLogin);
const userCurrencyId = computed(() => usersStore.userCurrencyId);
const userOnboardingCompleted = computed(() => usersStore.userOnboardingCompleted);
const selectedAccountId = computed(() => accountsStore.selectedAccountId);
const activeArea = computed(() => activeAreaStore.activeArea);
const transactionModalSwitchAccount = computed(() => {
  const accountId = transactionModalStore.transactionModalSwitchAccount;
  return typeof accountId === 'string' ? accountId : null;
});
const isFullyLoaded = computed(() => syncStore.isFullyLoaded);
const accounts = computed(() => accountsStore.accountsOrdered);
const folders = computed(() => accountFoldersStore.accountFoldersOrdered);

const websiteUrl = computed(() => getWebsiteUrl());
const econumoEdition = computed(() => econumoPackage.label);
const mainFolderName = computed(() => folders.value.find(folder => folder.isVisible === AccountFolderVisibility.Visible)?.name || 'elements.folders.default_folder');

interface AccountTreeItem {
  folder: {
    id: string;
    name: string;
    position: number;
    opened: boolean;
    isVisible: AccountFolderVisibility;
  };
  accounts: AccountDto[];
  amount: number;
  currency: CurrencyDto | null;
  amountAccountCurrency: number;
  accountCurrency: CurrencyDto | null | false;
}

const accountsTree = computed(() => {
  const result: AccountTreeItem[] = [];
  const accountsWithoutFolder: AccountDto[] = [];
  let accountsAmountWithoutFolder = 0;
  const userCurrency = usersStore.userCurrency;

  folders.value.forEach(folder => {
    if (!folder.isVisible) {
      return;
    }

    const item: AccountTreeItem = {
      folder: {
        ...folder,
        opened: true,
        isVisible: AccountFolderVisibility.Visible
      },
      accounts: [],
      amount: 0,
      currency: userCurrency,
      amountAccountCurrency: 0,
      accountCurrency: null
    };

    accounts.value.forEach(account => {
      if (account.folderId === folder.id) {
        item.accounts.push(account);
        item.amount += convert(account.balance, account.currency.id);
        if (item.accountCurrency === null) {
          item.accountCurrency = account.currency;
        }
        if (item.accountCurrency === false) {
          return;
        }
        if (item.accountCurrency.id === account.currency.id) {
          item.amountAccountCurrency += account.balance;
        } else {
          item.accountCurrency = false;
          item.amountAccountCurrency = 0;
        }
      }
    });

    if (item.accountCurrency !== false) {
      item.amount = item.amountAccountCurrency;
      if (item.accountCurrency) {
        item.currency = item.accountCurrency;
      }
    }
    if (item.accounts.length > 0) {
      result.push(item);
    }
  });

  accounts.value.forEach(account => {
    if (!account.folderId) {
      accountsWithoutFolder.push(account);
      accountsAmountWithoutFolder += convert(account.balance, account.currency.id);
    }
  });

  if (accountsWithoutFolder.length) {
    result.push({
      folder: {
        id: '0',
        name: 'elements.folders.default_folder',
        position: folders.value.length - 1,
        opened: true,
        isVisible: AccountFolderVisibility.Visible
      },
      accounts: accountsWithoutFolder,
      amount: accountsAmountWithoutFolder,
      currency: userCurrency,
      amountAccountCurrency: 0,
      accountCurrency: null
    });
  }

  return result;
});

const openAccountFolder = (id: string) => accountFoldersStore.openAccountFolder(id);
const closeAccountFolder = (id: string) => accountFoldersStore.closeAccountFolder(id);

function openBudget() {
  openPage('budget', 'budget');
}

function openPage(desktopPage: string, mobilePage: string) {
  if ($q.screen.gt.md) {
    router.push({ name: desktopPage });
  } else {
    router.push({ name: mobilePage });
  }

  accountsStore.selectAccount(null);
  activeAreaStore.setWorkspaceActiveArea();
}

function selectAccount(accountId: string | null) {
  if (accountId && typeof accountId === 'string') {
    accountsStore.selectAccount(accountId);
    router.push({ name: 'account', params: { id: accountId } });
  } else {
    accountsStore.selectAccount(null);
  }
  transactionModalStore.changeTransactionModalSwitchAccount(null);
}

function openSettings() {
  openPage('settings', 'settings');
}

function openAnalytics() {
  openPage('analytics', 'analytics');
}

function sync() {
  syncStore.fetchAll();
}

function refresh(done: () => void) {
  syncStore.fetchAll().finally(() => {
    done();
  });
}

function openCreateAccountModal() {
  accountModalStore.openAccountModal({ folderId: folders.value[0].id });
}

function convert(amount: number, originCurrencyId: string) {
  if (!userCurrencyId.value) return amount;
  return exchange(originCurrencyId, userCurrencyId.value, amount);
}

function getAccountName(accountId: string) {
  if (!accountId) return '';
  const account = _.find(accounts.value, { id: accountId });
  return account ? account.name : '';
}

function closeModalSwitchAccount() {
  transactionModalStore.changeTransactionModalSwitchAccount(null);
}
</script>
