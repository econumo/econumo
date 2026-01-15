<template>
  <q-page class="settings-accounts">
    <div class="settings-accounts-wrapper">
      <!-- toolbar for mobile -->
      <div class="settings-toolbar-mobile">
        <div>
          <q-btn class="settings-toolbar-mobile-button" flat icon="arrow_back" @click="navigateTo('settings', true)" />
        </div>
        <div>
          <h4 class="settings-toolbar-mobile-title">{{ $t('pages.settings.accounts.header') }}</h4>
        </div>
        <div class="settings-toolbar-mobile-container">
          <q-btn class="settings-toolbar-mobile-button" flat icon="create_new_folder" @click="openCreateFolderModal()" />
        </div>
      </div>

      <!-- toolbar for desktop -->
      <div class="settings-toolbar-desktop">
        <div class="settings-breadcrumbs">
          <div class="settings-breadcrumbs-item" @click="navigateTo('settings', true)">
            {{ $t('pages.settings.settings.header_desktop') }}
          </div>
        </div>
      </div>

      <!-- main block -->
      <h4 class="settings-label-header settings-accounts-header">{{ $t('pages.settings.accounts.header') }}</h4>
      <div class="settings-accounts-create-folder">
        <q-btn class="settings-accounts-create-folder-btn econumo-btn -small -magenta " flat :label="$t('pages.settings.accounts.create_folder')" @click="openCreateFolderModal()"/>
      </div>

      <div class="settings-accounts-container">
        <div>
          <div class="settings-accounts-account" v-for="(tree, index) in accountsTree" v-bind:key="tree.id">
            <q-item class="settings-accounts-account-head cursor-pointer">
              <q-item-section class="settings-accounts-account-head-title">
                <span class="settings-accounts-account-head-name">
                  <q-icon name="visibility_off" v-if="!tree.folder.isVisible" left @click="showFolder(tree.folder.id)"/>
                  <span class="econumo-truncate" :title="tree.folder.name">{{ tree.folder.name }}</span>
                </span>
              </q-item-section>
              <div class="settings-accounts-account-head-controls">
                <q-item-section class="settings-accounts-account-head-control">
                  <q-btn class="settings-accounts-account-head-control-btn" icon="control_point" @click="openCreateAccountModal(tree.folder.id)"/>
                </q-item-section>
                <q-item-section class="settings-accounts-account-head-control">
                  <q-btn flat icon="more_vert" class="settings-accounts-account-head-control-btn">
                    <q-menu cover auto-close class="account-transactions-item-check-button-menu">
                      <q-list class="account-transactions-item-check-button-list">
                        <q-item clickable @click="orderFolder(tree.folder.id, -1)" v-if="index > 0"
                                class="account-transactions-item-check-button-item">
                          <q-item-section class="account-transactions-item-check-button-section">
                            {{ $t('elements.button.up.label') }}
                          </q-item-section>
                        </q-item>
                        <q-item clickable @click="orderFolder(tree.folder.id, 1)" v-if="index < accountsTree.length - 1"
                                class="account-transactions-item-check-button-item">
                          <q-item-section class="account-transactions-item-check-button-section">
                            {{ $t('elements.button.down.label') }}
                          </q-item-section>
                        </q-item>
                        <q-item clickable @click="openUpdateFolderModal(tree.folder.id)"
                                class="account-transactions-item-check-button-item">
                          <q-item-section class="account-transactions-item-check-button-section">
                            {{ $t('elements.button.edit.label') }}
                          </q-item-section>
                        </q-item>
                        <q-item clickable @click="hideFolder(tree.folder.id)" v-if="tree.folder.isVisible"
                                class="account-transactions-item-check-button-item">
                          <q-item-section class="account-transactions-item-check-button-section">
                            {{ $t('elements.button.hide.label') }}
                          </q-item-section>
                        </q-item>
                        <q-item clickable @click="showFolder(tree.folder.id)" v-if="!tree.folder.isVisible"
                                class="account-transactions-item-check-button-item">
                          <q-item-section class="account-transactions-item-check-button-section">
                            {{ $t('elements.button.show.label') }}
                          </q-item-section>
                        </q-item>
                        <q-item clickable @click="openDeleteFolderModal(tree.folder.id)" v-if="index > 0"
                                class="account-transactions-item-check-button-item">
                          <q-item-section class="account-transactions-item-check-button-section -delete">
                            {{ $t('elements.button.delete.label') }}
                          </q-item-section>
                        </q-item>
                      </q-list>
                    </q-menu>
                  </q-btn>
                </q-item-section>
              </div>
            </q-item>
            <q-list class="settings-accounts-account-list" v-if="accounts.length > 0">
              <draggable :list="tree.accounts" v-bind="draggableAccountOptions" item-key="id"
                         :component-data="{'id': tree.id}" @end="orderAccounts">
                <template #item="{element}">
                  <q-item class="settings-accounts-account-list-item" clickable :id="element.id" @click="handleItemClick(element.id)">
                    <q-item-section side class="settings-accounts-account-list-item-draggable sortable-control" @click.stop>
                      <q-icon class="settings-accounts-account-list-item-draggable-icon" name="drag_indicator"/>
                    </q-item-section>
                      <div class="settings-accounts-account-list-item-wrapper">
                        <q-item-section class="settings-accounts-account-list-item-avatar" avatar>
                          <q-icon class="settings-accounts-account-list-item-avatar-icon" :name="element.icon"/>
                        </q-item-section>
                        <div class="settings-accounts-account-list-item-info">
                          <q-item-section class="settings-accounts-account-list-item-name">
                            <span class="settings-accounts-account-list-item-name-text econumo-truncate" :title="element.name">{{ element.name }}</span>
                            <q-icon v-if="element.sharedAccess.length > 0 && econumoPackage.includesSharedAccess" class="settings-accounts-account-list-item-shared-icon" name="link"/>
                          </q-item-section>
                          <div class="settings-accounts-account-list-item-info-container">
                            <q-item-section class="settings-accounts-account-list-item-balance">{{ moneyFormat(element.balance, element.currency.id, true, false) }}</q-item-section>
                          </div>
                        </div>
                        <q-item-section class="settings-accounts-account-list-item-shared" v-if="element.sharedAccess.length > 0 && econumoPackage.includesSharedAccess">
                          <q-avatar class="settings-accounts-account-list-item-shared-avatar">
                            <img :src="avatarUrl(element.owner.avatar, 30)" width="30" height="30"/>
                          </q-avatar>
                          <q-avatar class="settings-accounts-account-list-item-shared-avatar" v-for="access in element.sharedAccess"
                                    v-bind:key="access.user.id">
                            <img :src="avatarUrl(access.user.avatar, 30)" width="30" height="30"/>
                          </q-avatar>
                        </q-item-section>
                        <q-item-section class="settings-accounts-account-list-item-more" side v-if="$q.screen.gt.md">
                          <q-btn color="grey-7" round flat icon="more_vert" class="gt-md settings-accounts-account-list-item-more-btn" @click.stop>
                            <q-menu cover auto-close :ref="(el) => setMenuRef(el, element.id)">
                              <q-list>
                                <q-item clickable @click="openPreviewAccountModal(element.id)" class="account-transactions-item-check-button-item">
                                  <q-item-section class="account-transactions-item-check-button-section">{{ $t('elements.button.view.label') }}</q-item-section>
                                </q-item>
                                <q-item clickable @click="openUpdateAccountModal(element.id)" v-if="hasAdminAccess(element)" class="account-transactions-item-check-button-item">
                                  <q-item-section class="account-transactions-item-check-button-section">{{ $t('elements.button.edit.label') }}</q-item-section>
                                </q-item>
                                <q-item clickable @click="openAccountAccessModal(element.id)" v-if="hasAdminAccess(element) && econumoPackage.includesSharedAccess" class="account-transactions-item-check-button-item">
                                  <q-item-section class="account-transactions-item-check-button-section">{{ $t('pages.settings.accounts.list_actions.access') }}</q-item-section>
                                </q-item>
                                <q-item clickable @click="openDeleteAccountModal(element.id)" class="account-transactions-item-check-button-item">
                                  <q-item-section class="account-transactions-item-check-button-section -delete">{{ $t('elements.button.delete.label') }}</q-item-section>
                                </q-item>
                              </q-list>
                            </q-menu>
                          </q-btn>
                        </q-item-section>
                      </div>
                  </q-item>
                </template>
              </draggable>
            </q-list>
          </div>
        </div>
        <div class="settings-accounts-empty" v-if="!accounts.length">
          {{ $t('blocks.list.list_empty') }}
          <span v-if="folders.length"><a class="text-decoration-underline text-purple cursor-pointer" @click="openCreateAccountModal(accountsTree[0].folder.id)">{{ $t('pages.settings.accounts.list_empty_create') }}</a> {{ $t('pages.settings.accounts.list_empty_new_account') }}</span>
        </div>
      </div>

      <teleport to="body">
        <account-modal></account-modal>

        <confirmation-dialog-modal v-if="deleteFolderModal.isOpened"
                                   :question-title="$t('pages.settings.accounts.delete_folder_modal.title')"
                                   :question-label="deleteFolderModal.folder.name"
                                   :action-label="$t('elements.button.delete.label')"
                                   :cancel-label="$t('elements.button.cancel.label')"
                                   v-on:cancel="closeModals"
                                   v-on:proceed="deleteFolder(deleteFolderModal.folder.id)"
        />

        <confirmation-dialog-modal v-if="deleteAccountModal.isOpened"
                                   :question-label="$t('pages.settings.accounts.delete_account_modal.question', {account: deleteAccountModal.account.name})"
                                   :action-label="$t('elements.button.delete.label')"
                                   :cancel-label="$t('elements.button.cancel.label')"
                                   v-on:cancel="closeModals"
                                   v-on:proceed="deleteAccount(deleteAccountModal.account.id)"
        />

        <prompt-dialog-modal v-if="createFolderModal.isOpened"
                             :header-label="$t('pages.settings.accounts.create_folder_modal.header')"
                             input-value=""
                             :input-label="$t('elements.form.account.folder.label')"
                             :cancel-label="$t('elements.button.cancel.label')"
                             :submit-label="$t('elements.button.create.label')"
                             :validation="[
                               val => isNotEmpty(val) || $t('elements.form.account.folder.validation.empty_name'),
                               val => isValidFolderName(val) || $t('elements.form.account.folder.validation.error_name_length'),
                             ]"
                             v-on:cancel="closeModals"
                             v-on:close="closeModals"
                             v-on:submit="createFolder"
        />

        <prompt-dialog-modal v-if="updateFolderModal.isOpened"
                             :header-label="$t('pages.settings.accounts.update_folder_modal.header')"
                             :input-value="updateFolderModal.folderName"
                             :input-label="$t('elements.form.account.folder.label')"
                             :cancel-label="$t('elements.button.cancel.label')"
                             :submit-label="$t('elements.button.update.label')"
                             :validation="[
                               val => isNotEmpty(val) || $t('elements.form.account.folder.validation.empty_name'),
                               val => isValidFolderName(val) || $t('elements.form.account.folder.validation.error_name_length'),
                             ]"
                             v-on:close="closeModals"
                             v-on:submit="renameFolder"
        />

        <q-dialog class="preview-modal" v-model="previewAccountModal.isOpened" :position="$q.screen.gt.md ? 'standard' : 'bottom'">
          <q-card class="preview-modal-card">

<!--            Плашка "Доступ"-->
<!--            &lt;!&ndash; toolbar for desktop &ndash;&gt;-->
<!--            <div class="settings-toolbar-desktop preview-modal-breadcrumbs">-->
<!--              <div class="settings-breadcrumbs">-->
<!--                <div class="settings-breadcrumbs-item" @click="navigateTo('settings', true)">-->
<!--                  {{ $t('pages.settings.settings.header_desktop') }}-->
<!--                </div>-->
<!--                <div class="settings-breadcrumbs-arrow">-->
<!--                  <img src="~assets/breadcrumbs.svg" />-->
<!--                </div>-->
<!--                <div class="settings-breadcrumbs-item" v-close-popup>-->
<!--                  {{ $t('pages.settings.accounts.header') }}-->
<!--                </div>-->
<!--              </div>-->
<!--            </div>-->

            <q-card-section class="preview-modal-section">
              <div class="preview-modal-head">
                <div class="preview-modal-head-title">{{ $t('pages.settings.accounts.preview_account_modal.header') }}</div>
                <div class="preview-modal-head-shared" v-if="previewAccountModal.account.sharedAccess.length > 0 && econumoPackage.includesSharedAccess">
                  <q-icon class="preview-modal-head-shared-icon" name="link"/>
                  <div class="preview-modal-head-shared-label">{{ $t('pages.settings.accounts.preview_account_modal.shared_access') }}</div>
                </div>
              </div>

              <div class="preview-modal-account">
                <q-item class="preview-modal-account-item">
                  <q-item-section class="preview-modal-account-item-avatar" avatar>
                    <q-icon class="preview-modal-account-item-avatar-icon" :name="previewAccountModal.account.icon"/>
                  </q-item-section>
                  <div class="preview-modal-account-item-box">
                    <q-item-section class="preview-modal-account-item-box-name">
                      {{ previewAccountModal.account.name }}
                    </q-item-section>
                    <q-item-section class="preview-modal-account-item-box-balance" side>
                      {{ moneyFormat(previewAccountModal.account.balance, previewAccountModal.account.currency.id, true, false) }}
                    </q-item-section>
                  </div>
                  <q-item-section side v-if="previewAccountModal.account.sharedAccess.length > 0 && econumoPackage.includesSharedAccess"
                                  class="settings-accounts-account-list-item-shared">
                    <q-avatar class="settings-accounts-account-list-item-shared-avatar">
                      <img :src="avatarUrl(previewAccountModal.account.owner.avatar, 30)" width="30" height="30"/>
                    </q-avatar>
                    <q-avatar class="settings-accounts-account-list-item-shared-avatar"
                              v-for="access in previewAccountModal.account.sharedAccess" v-bind:key="access.user.id">
                      <img :src="avatarUrl(access.user.avatar, 30)" width="30" height="30"/>
                    </q-avatar>
                  </q-item-section>
                </q-item>
              </div>
            </q-card-section>

            <q-card-section class="preview-modal-section -info">
              <div class="preview-modal-account-info">
                <div class="preview-modal-account-info-item">
                  <div class="preview-modal-account-info-item-label">{{ $t('elements.form.account.name.label') }}</div>
                  <div class="preview-modal-account-info-item-content">{{ previewAccountModal.account.name }}</div>
                </div>
                <div class="preview-modal-account-info-item">
                  <div class="preview-modal-account-info-item-label">{{ $t('elements.form.account.balance.label') }}</div>
                  <div class="preview-modal-account-info-item-content">{{ previewAccountModal.account.balance }}</div>
                </div>
                <div class="preview-modal-account-info-item">
                  <div class="preview-modal-account-info-item-label">{{ $t('elements.form.account.currency.label') }}</div>
                  <div class="preview-modal-account-info-item-content">{{ previewAccountModal.account.currency.name }}</div>
                </div>
                <div class="preview-modal-account-info-access" v-if="econumoPackage.includesSharedAccess">
                  <div class="preview-modal-account-info-item">
                    <div class="preview-modal-account-info-item-label -access">{{ $t('pages.settings.accounts.preview_account_modal.access.label') }}</div>
                  </div>
                  <div v-if="previewAccountModal.account.owner.id !== userId" class="q-mb-sm">
                    <div class="preview-modal-account-info-access-item cursor-pointer" @click="openAccountAccessLevelModal(previewAccountModal.account.id, previewAccountModal.account.owner.id, 'owner')">
                      <q-avatar class="preview-modal-account-info-access-item-avatar">
                        <img :src="avatarUrl(previewAccountModal.account.owner.avatar, 100)" width="100" height="100"/>
                      </q-avatar>
                      <div class="preview-modal-account-info-access-item-user">
                        <div class="preview-modal-account-info-access-item-user-name">
                          {{ previewAccountModal.account.owner.name }}
                        </div>
                        <div class="preview-modal-account-info-access-item-user-role">
                          {{ $t('modules.connections.accounts.roles.owner') }}
                        </div>
                      </div>
                    </div>
                  </div>
                  <div v-if="!previewAccountModal.account.sharedAccess.length" class="preview-modal-account-info-access-empty">
                    {{ $t('pages.settings.accounts.preview_account_modal.access.no_shared_access') }}
                  </div>
                  <div v-else v-for="sharedAccess in previewAccountModal.account.sharedAccess"
                       v-bind:key="sharedAccess.user.id" class="q-mb-sm">
                    <div class="preview-modal-account-info-access-item cursor-pointer" @click="openAccountAccessLevelModal(previewAccountModal.account.id, sharedAccess.user.id, sharedAccess.role)">
                      <q-avatar class="preview-modal-account-info-access-item-avatar">
                        <img :src="avatarUrl(sharedAccess.user.avatar, 100)" class="preview-modal-account-info-access-item-avatar-img" width="100" height="100"/>
                      </q-avatar>
                      <div class="preview-modal-account-info-access-item-user">
                        <div class="preview-modal-account-info-access-item-user-name">
                          {{ sharedAccess.user.name }}
                        </div>
                        <div class="preview-modal-account-info-access-item-user-role">
                          {{ $t('modules.connections.accounts.roles.' + sharedAccess.role) }}
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div v-if="hasAdminAccess(previewAccountModal.account) && econumoPackage.includesSharedAccess">
                  <div class="preview-modal-account-info-access-btn" @click="openAccountAccessModal(previewAccountModal.account.id)">{{ $t('pages.settings.accounts.preview_account_modal.action.shared_access') }}</div>
                </div>
              </div>
            </q-card-section>

            <q-card-actions class="preview-modal-account-info-access-actions">
              <q-btn class="econumo-btn -large -grey preview-modal-account-info-access-actions-btn" flat icon="delete" :title="$t('elements.button.delete.label')"
                     @click="openDeleteAccountModal(previewAccountModal.account.id)"/>
              <q-btn class="econumo-btn -large -grey preview-modal-account-info-access-actions-btn -wide" flat :label="$t('elements.button.edit.label')"
                     @click="openUpdateAccountModal(previewAccountModal.account.id)" v-if="hasAdminAccess(previewAccountModal.account)"/>
              <q-space v-if="!hasAdminAccess(previewAccountModal.account)" />
              <q-btn class="econumo-btn -large -grey preview-modal-account-info-access-actions-btn" flat icon="expand_more" :title="$t('elements.button.cancel.label')"
                     v-close-popup/>
            </q-card-actions>
          </q-card>
        </q-dialog>

        <access-dialog-modal v-if="accountAccessModal.isOpened"
                             :item="accountAccessModal.account"
                             :user-id="userId"
                             :connections="connections"
                             :item-owner-user-id="accountAccessModal.account.owner.id"
                             v-on:cancel="closeModals"
                             v-on:open-access-level-dialog="openAccountAccessLevelModal" />

        <access-level-dialog-modal v-if="accountAccessLevelModal.isOpened"
                                     :user="accountAccessLevelModal.user"
                                     :item-id="accountAccessLevelModal.accountId"
                                     :role="accountAccessLevelModal.role"
                                     v-on:cancel="closeAccessLevelModal"
                                     v-on:allow="updateAccountAccess"
                                     v-on:revoke="revokeAccountAccess"
        />
      </teleport>
    </div>
  </q-page>
</template>

<script>
import {defineComponent} from 'vue';
import {navigationMixin} from '../../mixins/navigationMixin';
import {useValidation} from '../../composables/useValidation';
import {useMoney} from '../../composables/useMoney';
import {useAccount} from '../../composables/useAccount';
import _ from 'lodash';
import draggable from 'vuedraggable';
import AccountModal from '../../components/AccountModal';
import ConfirmationDialogModal from '../../components/ConfirmationDialogModal';
import PromptDialogModal from 'components/PromptDialogModal';
import AccessDialogModal from 'components/AccessDialogModal';
import AccessLevelDialogModal from 'components/AccessLevelDialogModal';
import { mapState } from 'pinia'
import {useUsersStore} from '../../stores/users';
import {useConnectionsStore} from '../../stores/connections';
import {useAccountsStore} from '../../stores/accounts';
import {useAccountFoldersStore} from '../../stores/account-folders';
import {useAccountModalStore} from '../../stores/account-modal';
import {useActiveAreaStore} from '../../stores/active-area';
import { econumoPackage } from '../../modules/package';
import { useAvatar } from '../../composables/useAvatar';

export default defineComponent({
  name: 'SettingsAccountsPage',
  mixins: [navigationMixin],
  components: {AccessDialogModal, AccessLevelDialogModal, PromptDialogModal, draggable, AccountModal, ConfirmationDialogModal},
  setup() {
    const { avatarUrl } = useAvatar();
    const validation = useValidation();
    const { moneyFormat } = useMoney();
    const { accountName } = useAccount();

    return {
      avatarUrl,
      ...validation,
      moneyFormat,
      accountName
    };
  },
  data() {
    return {
      accountsCopy: null,
      accountFoldersCopy: null,
      menuRefs: new Map(),
      deleteFolderModal: {
        isOpened: false,
        folder: null,
      },
      createFolderModal: {
        isOpened: false,
      },
      updateFolderModal: {
        isOpened: false,
        folderId: null,
        folderName: null,
      },
      previewAccountModal: {
        isOpened: false,
        account: null,
      },
      deleteAccountModal: {
        isOpened: false,
        account: null,
      },
      accountAccessModal: {
        isOpened: false,
        account: null,
      },
      accountAccessLevelModal: {
        isOpened: false,
        user: null,
        accountId: null,
        role: null
      }
    }
  },
  created() {
    if (this.$router.currentRoute.value.name === 'settingsAccounts') {
      useActiveAreaStore().setWorkspaceActiveArea();
    }
  },
  computed: {
    ...mapState(useUsersStore, ['userId']),
    ...mapState(useConnectionsStore, ['connections']),
    ...mapState(useAccountFoldersStore, ['accountFoldersOrdered']),
    ...mapState(useAccountsStore, ['accountsOrdered']),
    ...mapState(useAccountsStore, {'accountAll': 'accounts'}),
    accounts: {
      get() {
        return this.accountsCopy || this.accountsOrdered
      },
      set(items) {
        this.accountsCopy = items;
        useAccountsStore().orderAccountList(items).finally(() => {
          this.accountsCopy = null;
        });
      }
    },
    folders: function () {
      return this.accountFoldersCopy || this.accountFoldersOrdered;
    },
    draggableAccountOptions() {
      return {
        animation: 200,
        group: 'accounts',
        disabled: false,
        ghostClass: 'ghost',
        handle: '.sortable-control'
      }
    },
    accountsTree: function () {
      let result = [];
      this.folders.forEach((item) => {
        result.push({
          id: item.id,
          folder: item,
          accounts: _.orderBy(_.filter(this.accounts, {folderId: item.id}), ['position'], ['asc'])
        });
      })
      return result;
    },
    econumoPackage: function() {
      return econumoPackage;
    }
  },
  methods: {
    setMenuRef: function(el, accountId) {
      if (el) {
        this.menuRefs.set(accountId, el);
      }
    },
    handleItemClick: function(accountId) {
      if (this.$q.screen.gt.md) {
        // Desktop: open menu
        const menu = this.menuRefs.get(accountId);
        if (menu) {
          menu.show();
        }
      } else {
        // Mobile: open preview modal
        this.openPreviewAccountModal(accountId);
      }
    },
    hasAdminAccess: function (account) {
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
    },
    createFolder: function (value) {
      this.closeModals();
      useAccountFoldersStore().createAccountFolder({name: value.toString()});
    },
    renameFolder: function (value, initialValue) {
      this.closeModals();
      const folder = _.find(this.accountFoldersOrdered, {name: initialValue});
      useAccountFoldersStore().updateAccountFolder({id: folder.id, name: value});
    },
    orderAccounts: function (e) {
      const newFolderId = e.to.id || null,
        accountId = e.item.id || null,
        newIndex = e.newIndex;

      let accountsIds = [],
        accounts = [];
      this.accountsTree.forEach((item) => {
        item.accounts.forEach((account, index) => {
          if (item.folder.id === newFolderId && index === newIndex) {
            accountsIds.push(accountId)
          }
          if (_.indexOf(accountsIds, account.id) === -1) {
            accountsIds.push(account.id)
          }
        })
      });
      let globalPosition = 0,
        newAccountPosition = 0;
      accountsIds.forEach((id) => {
        const account = _.find(this.accounts, {id: id});
        if (id === accountId) {
          account.folderId = newFolderId;
          newAccountPosition = globalPosition;
        }
        if (account) {
          account.position = globalPosition++;
          accounts.push(account);
        }
      })
      this.accounts = accounts;
    },
    orderFolder: function (folderId, changePosition) {
      let foldersOrdered = _.cloneDeep(this.accountFoldersOrdered);
      let changedFolder = _.find(foldersOrdered, {id: folderId});
      const changedPosition = changedFolder.position + changePosition;
      foldersOrdered.forEach((item, index) => {
        if (index === changedPosition) {
          item.position = changePosition < 0 ? (item.position + 1) : (item.position - 1);
        } else if (item.id === changedFolder.id) {
          item.position = changedPosition;
        }
      });
      this.accountFoldersCopy = _.orderBy(foldersOrdered, ['position'], ['asc']);
      let orderedIds = [];
      _.orderBy(foldersOrdered, ['position'], ['asc']).forEach((item) => {
        orderedIds.push(item.id)
      })
      useAccountFoldersStore().orderAccountFolderList(orderedIds).finally(() => {
        this.accountFoldersCopy = null;
      })
    },
    openCreateFolderModal: function () {
      this.createFolderModal.isOpened = true;
    },
    openUpdateFolderModal: function (folderId) {
      this.updateFolderModal.folderId = folderId;
      this.updateFolderModal.folderName = _.find(this.folders, {id: folderId}).name;
      this.updateFolderModal.isOpened = true;
    },
    hideFolder: function (folderId) {
      useAccountFoldersStore().hideAccountFolder({id: folderId});
    },
    showFolder: function (folderId) {
      useAccountFoldersStore().showAccountFolder({id: folderId});
    },
    openDeleteFolderModal: function (folderId) {
      this.deleteFolderModal.folder = _.cloneDeep(_.find(this.accountFoldersOrdered, {id: folderId}));
      this.deleteFolderModal.isOpened = true;
    },
    deleteFolder: function (folderId) {
      const replaceFolder = _.last(_.filter(this.accountFoldersOrdered, (item) => {
        return item.id !== folderId;
      }));
      useAccountFoldersStore().replaceAccountFolder({id: folderId, replaceId: replaceFolder.id}).finally(() => {
        this.closeModals();
      });
    },
    openPreviewAccountModal: function (accountId) {
      this.previewAccountModal.account = _.cloneDeep(_.find(this.accountAll, {id: accountId}));
      this.previewAccountModal.isOpened = true;
    },
    openAccountAccessModal: function (accountId) {
      this.accountAccessModal.account = _.cloneDeep(_.find(this.accountAll, {id: accountId}));
      this.accountAccessModal.isOpened = true;
    },
    openAccountAccessLevelModal: function (accountId, userId, userRole) {
      if (userRole === 'owner') {
        return;
      }
      const account = _.cloneDeep(_.find(this.accountAll, {id: accountId}));
      const sharedAccess = _.find(account.sharedAccess, {user: {id: userId}});
      this.accountAccessLevelModal.accountId = account.id;
      let tmpUser = _.find(this.connections, {user: {id: userId}});
      if (tmpUser) {
        this.accountAccessLevelModal.user = tmpUser.user;
      } else {
        this.accountAccessLevelModal.user = sharedAccess?.user || null;
      }
      this.accountAccessLevelModal.role = sharedAccess?.role || null;
      this.accountAccessLevelModal.isOpened = true;
      console.log(this.accountAccessLevelModal, accountId, userId)
    },
    updateAccountAccess: function(userId, accountId, role) {
      this.closeModals();
      useConnectionsStore().setAccountAccess({
        userId: userId,
        accountId: accountId,
        role: role
      }).finally(() => {
        this.accountsCopy = null;
      });
    },
    revokeAccountAccess: function(userId, accountId) {
      this.closeModals();
      useConnectionsStore().revokeAccountAccess({
        userId: userId,
        accountId: accountId
      }).finally(() => {
        this.accountsCopy = null;
      });
    },
    openCreateAccountModal: function (folderId) {
      useAccountModalStore().openAccountModal({folderId: folderId});
    },
    openUpdateAccountModal: function (accountId) {
      this.previewAccountModal.account = null;
      this.previewAccountModal.isOpened = false;
      useAccountModalStore().openAccountModal(_.cloneDeep(_.find(this.accountAll, {id: accountId})));
    },
    openDeleteAccountModal: function (accountId) {
      this.deleteAccountModal.account = _.cloneDeep(_.find(this.accountAll, {id: accountId}));
      this.deleteAccountModal.isOpened = true;
    },
    deleteAccount: function (accountId) {
      useAccountsStore().deleteAccount(accountId).finally(() => {
        this.closeModals();
      });
    },
    closeAccessLevelModal: function () {
      this.accountAccessLevelModal.isOpened = false;
      this.accountAccessLevelModal.accountId = null;
      this.accountAccessLevelModal.user = null;
      this.accountAccessLevelModal.role = null;
    },
    closeModals: function () {
      this.previewAccountModal.isOpened = false;
      this.previewAccountModal.account = null;
      this.deleteAccountModal.isOpened = false;
      this.deleteAccountModal.account = null;
      this.deleteFolderModal.isOpened = false;
      this.deleteFolderModal.folder = null;
      this.updateFolderModal.isOpened = false;
      this.updateFolderModal.folderId = null;
      this.updateFolderModal.folderName = null;
      this.createFolderModal.isOpened = false;
      this.accountAccessModal.isOpened = false;
      this.accountAccessModal.account = null;
      this.accountAccessLevelModal.isOpened = false;
      this.accountAccessLevelModal.accountId = null;
      this.accountAccessLevelModal.user = null;
      this.accountAccessLevelModal.level = null;
      useAccountModalStore().closeAccountModal();
    },
    availableConnections: function (account) {
      return _.filter(this.connections, (connection) => {
        if (connection.user.id === account.owner.id) {
          return false;
        }
        return !_.find(connection.sharedAccounts, {id: account.id}) || !_.find(account.sharedAccess, {user: {id: connection.user.id}});
      })
    }
  }
})
</script>
