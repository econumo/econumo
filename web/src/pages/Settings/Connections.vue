<template>
  <q-page class="settings-connections">
    <div class="settings-connections-wrapper">
      <!-- toolbar for mobile -->
      <div class="settings-toolbar-mobile">
        <div>
          <q-btn class="settings-toolbar-mobile-button" flat icon="arrow_back" @click="navigateTo('settings', true)" />
        </div>
        <div>
          <h4 class="settings-toolbar-mobile-title">{{ $t('modules.connections.pages.settings.header') }}</h4>
        </div>
        <div class="settings-toolbar-mobile-container">
          <template v-if="econumoPackage.includesConnections">
            <q-btn class="settings-toolbar-mobile-button" flat icon="add_circle_outline" @click="openAcceptInviteModal()" />
          </template>
          <template v-else>
            &nbsp;
          </template>
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
      <h4 class="settings-label-header settings-connections-header">{{ $t('modules.connections.pages.settings.header') }}</h4>
      <div class="settings-connections-controls" v-if="econumoPackage.includesConnections">
        <q-btn class="econumo-btn -small -magenta settings-connections-controls-btn" flat :label="$t('modules.connections.pages.settings.generate_invite')" @click="generateInvite()"/>
        <q-btn class="econumo-btn -small -grey settings-connections-controls-btn" flat :label="$t('modules.connections.pages.settings.accept_invite')" @click="openAcceptInviteModal()"/>
      </div>
      <div class="settings-connections-empty" v-if="!connections.length">
        {{ $t('blocks.list.list_empty') }}
      </div>
      <div v-if="connections.length > 0">
        <q-list class="settings-connections-list">
          <q-item class="settings-connections-list-item" clickable v-for="connection in connections" v-bind:key="connection.user.id" @click="handleItemClick(connection.user.id)">
            <div class="settings-connections-list-item-section">
              <q-item-section class="settings-connections-list-item-avatar" avatar>
                <img class="settings-connections-list-item-avatar-img" :src="connection.user.avatar">
              </q-item-section>
              <div class="settings-connections-list-item-content">
                <q-item-section class="settings-connections-list-item-name">{{ connection.user.name }}</q-item-section>
              </div>
            </div>
            <q-item-section v-if="$q.screen.gt.md" class="settings-connections-list-item-more" side>
              <q-btn square flat icon="more_vert" class="account-transactions-item-check-button settings-connections-list-item-more-btn" @click.stop>
                <q-menu cover auto-close class="account-transactions-item-check-button-menu" :ref="(el) => setMenuRef(el, connection.user.id)">
                  <q-list class="account-transactions-item-check-button-list">
                    <q-item clickable @click="openPreviewConnectionModal(connection.user.id)" class="account-transactions-item-check-button-item">
                      <q-item-section class="account-transactions-item-check-button-section">{{ $t('elements.button.view.label') }}</q-item-section>
                    </q-item>
                    <q-item clickable @click="openDeleteConnectionModal(connection.user.id)" class="account-transactions-item-check-button-item" v-if="econumoPackage.includesConnections">
                      <q-item-section class="account-transactions-item-check-button-section -delete">{{ $t('elements.button.delete.label') }}</q-item-section>
                    </q-item>
                  </q-list>
                </q-menu>
              </q-btn>
            </q-item-section>
            <q-item-section v-if="!$q.screen.gt.md" class="settings-connections-list-item-chevron" side>
              <q-icon name="chevron_right" size="24px" color="grey-6" />
            </q-item-section>
          </q-item>
        </q-list>
      </div>
      <div class="settings-connections-generate" v-if="econumoPackage.includesConnections">
        <q-btn class="econumo-btn -medium -magenta settings-connections-generate-btn" flat :label="$t('modules.connections.pages.settings.generate_invite')" @click="generateInvite()"/>
      </div>
      <div v-if="!econumoPackage.includesConnections">
        <div class="settings-connections-info">
          <div v-html="$t('modules.connections.pages.settings.info.header')"></div>
          <ol>
            <li><b>{{ $t('modules.connections.pages.settings.info.list.all_users_connected') }}</b>: {{ $t('modules.connections.pages.settings.info.list.all_users_connected_description') }}</li>
            <li><b>{{ $t('modules.connections.pages.settings.info.list.no_connections') }}</b>: {{ $t('modules.connections.pages.settings.info.list.no_connections_description') }}</li>
          </ol>
          <a :href="$t('modules.connections.pages.settings.info.link_url')" target="_blank">{{ $t('modules.connections.pages.settings.info.link') }}</a>.
        </div>
      </div>

      <teleport to="body">
        <confirmation-dialog-modal :icon="false"
                                   v-if="deleteConnectionModal.isOpened.value"
                                   :question-label="$t('modules.connections.modals.delete_connection.question', {name: deleteConnectionModal.data.value?.user.name})"
                                   :action-label="$t('elements.button.delete.label')"
                                   :cancel-label="$t('elements.button.cancel.label')"
                                   v-on:cancel="closeModal"
                                   v-on:proceed="deleteConnection(deleteConnectionModal.data.value?.user.id)"
        />

        <generate-invite-modal
          v-model:isOpened="generateInviteModal.isOpened.value"
          :code="generateInviteModal.data.value?.code || ''"
          @hide="deleteInvite"
        />

        <accept-invite-modal
          v-model:isOpened="acceptInviteModal.isOpened.value"
          @hide="closeModal"
          @accept="acceptInvite"
        />

        <preview-connection-modal
          v-if="previewConnectionModal.data.value"
          v-model:isOpened="previewConnectionModal.isOpened.value"
          :connection="previewConnectionModal.data.value"
          :allow-changes="econumoPackage.includesConnections"
          @hide="closeModal"
          @delete="openDeleteConnectionModal"
          @decline-account="declineAccountAccess"
          @decline-budget="declineBudgetAccess"
          @allow-account="allowAccountAccess"
          @revoke-account="revokeAccountAccess"
          @share-budget="shareBudgetAccess"
          @revoke-budget="revokeBudgetAccess"
        />
      </teleport>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { useQuasar } from 'quasar';
import { storeToRefs } from 'pinia';
import _ from 'lodash';

import ConfirmationDialogModal from '../../components/ConfirmationDialogModal.vue';
import GenerateInviteModal from '../../components/Connections/GenerateInviteModal.vue';
import AcceptInviteModal from '../../components/Connections/AcceptInviteModal.vue';
import PreviewConnectionModal from '../../components/Connections/PreviewConnectionModal.vue';

import { useConnectionsStore, type Connection } from 'stores/connections';
import { useAccountsStore } from 'stores/accounts';
import { useBudgetsStore } from 'stores/budgets';
import { useActiveAreaStore } from 'stores/active-area';

import { useGenerateInviteModalState } from '../../composables/connections/useGenerateInviteModalState';
import { useAcceptInviteModalState } from '../../composables/connections/useAcceptInviteModalState';
import { usePreviewConnectionModalState } from '../../composables/connections/usePreviewConnectionModalState';
import { useDeleteConnectionModalState } from '../../composables/connections/useDeleteConnectionModalState';
import { Id } from '@shared/types';
import { RouterPage } from '../../router/constants';
import { econumoPackage } from '../../modules/package';
import config from '../../modules/config';

defineOptions({
  name: 'SettingsConnectionsPage'
});

const router = useRouter();
const $q = useQuasar();

const connectionsStore = useConnectionsStore();
const accountsStore = useAccountsStore();
const budgetsStore = useBudgetsStore();
const activeAreaStore = useActiveAreaStore();

const { isConnectionsLoaded, connections } = storeToRefs(connectionsStore);

// const econumoPackage = econumoPackage;
const generateInviteModal = useGenerateInviteModalState();
const acceptInviteModal = useAcceptInviteModalState();
const previewConnectionModal = usePreviewConnectionModalState();
const deleteConnectionModal = useDeleteConnectionModalState();

const reloadInterval = ref<number | null>(null);
const menuRefs = ref<Map<Id, any>>(new Map());

const startAutoReload = () => {
  reloadInterval.value = window.setInterval(() => {
    connectionsStore.fetchConnections();
  }, 5000);
};

const stopAutoReload = () => {
  if (reloadInterval.value) {
    clearInterval(reloadInterval.value);
    reloadInterval.value = null;
  }
};

const navigateTo = (name: string, replace = false) => {
  if (replace) {
    router.replace({ name });
  } else {
    router.push({ name });
  }
};

const setMenuRef = (el: any, userId: Id) => {
  if (el) {
    menuRefs.value.set(userId, el);
  }
};

const handleItemClick = (userId: Id) => {
  if ($q.screen.gt.md) {
    // Desktop: open menu
    const menu = menuRefs.value.get(userId);
    if (menu) {
      menu.show();
    }
  } else {
    // Mobile: open preview modal
    openPreviewConnectionModal(userId);
  }
};

const openAcceptInviteModal = () => {
  acceptInviteModal.open();
};

const openGenerateInviteModal = (invite: { code: string; expiredAt: string }) => {
  generateInviteModal.open(invite);
};


const openPreviewConnectionModal = (userId: Id) => {
  const connection = connections.value.find(c => c.user.id === userId);
  if (connection) {
    const connectionData: Connection = {
      user: {
        id: connection.user.id,
        name: connection.user.name,
        avatar: connection.user.avatar
      },
      sharedAccounts: connectionsStore.getSharedAccounts(userId),
      sharedBudgets: connectionsStore.getSharedBudgets(userId)
    };
    previewConnectionModal.open(connectionData);
  }
};

const openDeleteConnectionModal = (userId: Id) => {
  const connection = connections.value.find(c => c.user.id === userId);
  if (connection) {
    const connectionData: Connection = {
      user: {
        id: connection.user.id,
        name: connection.user.name,
        avatar: connection.user.avatar
      },
      sharedAccounts: connectionsStore.getSharedAccounts(userId),
      sharedBudgets: connectionsStore.getSharedBudgets(userId)
    };
    console.log(connectionData);
    deleteConnectionModal.open(connectionData);
  }
};

const closeModal = () => {
  generateInviteModal.close();
  acceptInviteModal.close();
  previewConnectionModal.close();
  deleteConnectionModal.close();
};

const generateInvite = async () => {
  const invite = await connectionsStore.generateConnectionInvite();
  openGenerateInviteModal(invite);
};

const deleteInvite = () => {
  closeModal();
};

const acceptInvite = (code: string) => {
  connectionsStore.acceptConnectionInvite(code);
};

const deleteConnection = (userId?: string) => {
  closeModal();
  if (userId) {
    connectionsStore.deleteConnection(userId);
  }
};

const refreshPreviewModalData = (userId: Id) => {
  const connection = connections.value.find(c => c.user.id === userId);
  if (connection && previewConnectionModal.data.value) {
    const connectionData: Connection = {
      user: {
        id: connection.user.id,
        name: connection.user.name,
        avatar: connection.user.avatar
      },
      sharedAccounts: connectionsStore.getSharedAccounts(userId),
      sharedBudgets: connectionsStore.getSharedBudgets(userId)
    };
    previewConnectionModal.data.value = connectionData;
  }
};

const declineAccountAccess = (accountId: Id) => {
  const userId = previewConnectionModal.data.value?.user.id;
  if (!userId) {
    return;
  }
  accountsStore.deleteAccount(accountId).then(() => {
    refreshPreviewModalData(userId);
  });
};

const declineBudgetAccess = (budgetId: Id) => {
  const userId = previewConnectionModal.data.value?.user.id;
  if (!userId) {
    return;
  }
  budgetsStore.declineAccess({ budgetId }).then(() => {
    connectionsStore.fetchConnections().then(() => {
      refreshPreviewModalData(userId);
    });
  });
};

const allowAccountAccess = (userId: Id, accountId: Id, role: string) => {
  connectionsStore.setAccountAccess({
    userId: userId,
    accountId: accountId,
    role: role
  }).then(() => {
    refreshPreviewModalData(userId);
  });
};

const revokeAccountAccess = (userId: Id, accountId: Id) => {
  connectionsStore.revokeAccountAccess({
    userId: userId,
    accountId: accountId
  }).then(() => {
    refreshPreviewModalData(userId);
  });
};

const shareBudgetAccess = (userId: Id, budgetId: Id, role: any) => {
  connectionsStore.setBudgetAccess({
    userId: userId,
    budgetId: budgetId,
    role: role
  }).then(() => {
    refreshPreviewModalData(userId);
  });
};

const revokeBudgetAccess = (userId: Id, budgetId: Id) => {
  connectionsStore.revokeBudgetAccess({
    userId: userId,
    budgetId: budgetId
  }).then(() => {
    refreshPreviewModalData(userId);
  });
};

onMounted(() => {
  if (router.currentRoute.value.name === RouterPage.SETTINGS_CONNECTIONS) {
    activeAreaStore.setWorkspaceActiveArea();
  }
  if (!isConnectionsLoaded.value) {
    connectionsStore.fetchConnections();
  }
  startAutoReload();
});

onUnmounted(() => {
  stopAutoReload();
});
</script>

<style scoped>
.settings-connections-list-item-section {
  align-items: center;
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
}

.settings-connections-list-item-content {
  min-width: 0;
}

.settings-connections-list-item-chevron {
  align-self: center;
}
</style>
