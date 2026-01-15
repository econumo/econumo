<template>
  <q-page class="settings-classification">
    <div class="settings-classification-wrapper">
      <!-- toolbar for mobile -->
      <div class="settings-toolbar-mobile settings-classification-toolbar-mobile">
        <div>
          <q-btn class="settings-toolbar-mobile-button" flat icon="arrow_back" @click="navigateTo('settings', true)" />
        </div>
        <div>
          <h4 class="settings-toolbar-mobile-title">{{ $t('modules.classifications.payees.pages.settings.header') }}</h4>
        </div>
        <div class="settings-toolbar-mobile-container">
          <q-btn class="settings-toolbar-mobile-button" flat icon="add_circle_outline" @click="openCreateModal()" />
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
      <h4 class="settings-label-header settings-classification-header">{{ $t('modules.classifications.payees.pages.settings.menu_item') }}</h4>
      <div class="settings-classification-create-class">
        <q-btn class="settings-classification-create-class-btn econumo-btn -small -magenta" flat :label="$t('modules.classifications.payees.pages.settings.create_payee')" @click="openCreateModal()"/>
      </div>

      <div class="settings-classification-sort" v-if="payees.length > 1">
        <q-btn class="settings-classification-sort-btn"
               icon="sort"
               :label="$t('blocks.list.order_list')"
               :no-caps=true
               :flat=true
               :ripple=false
               @click="openSortModal"/>
      </div>

      <div class="settings-classification-container">
        <div class="settings-classification-empty" v-if="!payees.length">
          {{ $t('blocks.list.list_empty') }}
        </div>
        <div v-if="payees.length > 0">
          <q-list class="settings-classification-list">
            <draggable v-model="payees" @start="drag=true" @end="drag=false" v-bind="options" item-key="id">
              <template #item="{element}">
                <q-item class="settings-classification-list-item" clickable @click="handleItemClick(element)">
                  <q-item-section side class="settings-classification-list-item-sortable sortable-control cursor-pointer" @click.stop>
                    <q-icon name="drag_indicator"/>
                  </q-item-section>
                  <q-item-section :class="!!element.isArchived ? 'settings-classification-list-item-text -archived' : 'settings-classification-list-item-text'">
                    <span class="settings-classification-list-item-name econumo-truncate" :title="element.name">{{ element.name }}</span>
                    <div class="settings-classification-list-item-description-archived" v-if="element.isArchived">{{ $t('modules.classifications.payees.pages.settings.archived_item') }}</div>
                  </q-item-section>
                  <q-item-section side @click.stop>
                    <q-toggle :model-value="!element.isArchived"
                              @click="updateArchivePayee(element.id, !!element.isArchived)"/>
                  </q-item-section>
                  <q-item-section side v-if="$q.screen.gt.md" class="cursor-pointer settings-classification-list-item-check-section">
                    <q-btn square flat icon="more_vert" class="account-transactions-item-check-button" @click.stop>
                      <q-menu cover auto-close class="account-transactions-item-check-button-menu" :ref="(el) => setMenuRef(el, element.id)">
                        <q-list class="account-transactions-item-check-button-list">
                          <q-item clickable @click="openEditModal(element.id)" class="account-transactions-item-check-button-item">
                            <q-item-section class="account-transactions-item-check-button-section">{{ $t('elements.button.edit.label') }}</q-item-section>
                          </q-item>
                          <q-item clickable @click="openDeleteModal(element.id)" class="account-transactions-item-check-button-item">
                            <q-item-section class="account-transactions-item-check-button-section -delete">{{ $t('elements.button.delete.label') }}</q-item-section>
                          </q-item>
                        </q-list>
                      </q-menu>
                    </q-btn>
                  </q-item-section>
                </q-item>
              </template>
            </draggable>
          </q-list>
        </div>
      </div>

      <teleport to="body">
        <context-menu-modal v-if="contextMenuModal.isOpened"
                            :header-label="contextMenuModal.payee.name"
                            :actions="[
                              {label: $t('elements.button.edit.label'), value: 'edit', context: contextMenuModal.payee.id},
                              {label: $t('elements.button.delete.label'), value: 'delete', context: contextMenuModal.payee.id},
                              {label: $t('elements.button.cancel.label'), value: 'cancel', context: contextMenuModal.payee.id}
                              ]"
                            v-on:cancel="closeContextMenuModal()"
                            v-on:proceed="openNextModal"
        />

        <sort-dialog-modal v-model="sortModal.isOpened" v-on:cancel="closeSortModal"
                           v-on:proceed="updateOrder"></sort-dialog-modal>

        <prompt-dialog-modal v-if="editModal.isOpened"
                             :header-label="$t('modules.classifications.payees.modals.edit.header')"
                             :input-value="editModal.payeeName"
                             :input-label="$t('modules.classifications.payees.forms.payee.name.label')"
                             :cancel-label="$t('elements.button.cancel.label')"
                             :submit-label="$t('elements.button.update.label')"
                             :validation="[
                              val => isNotEmpty(val) || $t('modules.classifications.payees.forms.payee.name.validation.required_field'),
                              val => isValidPayeeName(val) || $t('modules.classifications.payees.forms.payee.name.validation.invalid_name')
                             ]"
                             v-on:close="closeEditModal"
                             v-on:submit="updatePayee"
        />

        <confirmation-dialog-modal v-if="deleteModal.isOpened"
                                   :question-title="$t('modules.classifications.payees.modals.delete.title')"
                                   :question-label="deleteModal.payee?.name"
                                   :action-label="$t('elements.button.delete.label')"
                                   :cancel-label="$t('elements.button.cancel.label')"
                                   v-on:cancel="closeDeleteModal"
                                   v-on:proceed="deletePayee(deleteModal.payee.id)" />

        <prompt-dialog-modal v-if="createModal.isOpened"
                             :header-label="$t('modules.classifications.payees.modals.create.header')"
                             input-value=""
                             :input-label="$t('modules.classifications.payees.forms.payee.name.label')"
                             :cancel-label="$t('elements.button.cancel.label')"
                             :submit-label="$t('elements.button.create.label')"
                             :validation="[
                              val => isNotEmpty(val) || $t('modules.classifications.payees.forms.payee.name.validation.required_field'),
                              val => isValidPayeeName(val) || $t('modules.classifications.payees.forms.payee.name.validation.invalid_name')
                             ]"
                             v-on:close="closeCreateModal"
                             v-on:submit="createPayee"
        />
      </teleport>
    </div>
  </q-page>
</template>

<script>
import {defineComponent} from 'vue';
import {navigationMixin} from '../../mixins/navigationMixin';
import {useValidation} from '../../composables/useValidation';
import _ from 'lodash';
import draggable from 'vuedraggable';
import SortDialogModal from 'components/SortDialogModal';
import ConfirmationDialogModal from 'components/ConfirmationDialogModal';
import PromptDialogModal from 'components/PromptDialogModal';
import ContextMenuModal from 'components/ContextMenuModal';
import { mapState } from 'pinia'
import {useUsersStore} from 'stores/users';
import {useActiveAreaStore} from 'stores/active-area';
import {usePayeesStore} from 'stores/payees';

export default defineComponent({
  name: 'SettingsPayeesPage',
  mixins: [navigationMixin],
  components: {draggable, SortDialogModal, ContextMenuModal, PromptDialogModal, ConfirmationDialogModal},
  setup() {
    const validation = useValidation();
    return { ...validation };
  },
  data() {
    return {
      payeesCopy: null,
      menuRefs: new Map(),
      contextMenuModal: {
        isOpened: false,
        payee: null
      },
      editModal: {
        isOpened: false,
        payeeId: null,
        payeeName: null,
      },
      createModal: {
        isOpened: false
      },
      sortModal: {
        isOpened: false
      },
      deleteModal: {
        isOpened: false,
        payee: null
      },
      modal: {
        isOpened: false,
        payeeId: null,
        payeeName: null,
        payeeIsArchived: null
      }
    }
  },
  created() {
    if (this.$router.currentRoute.value.name === 'settingsPayees') {
      useActiveAreaStore().setWorkspaceActiveArea();
    }
  },
  computed: {
    ...mapState(useUsersStore, ['userId']),
    ...mapState(usePayeesStore, ['ownPayees']),
    payees: {
      get() {
        return this.payeesCopy || this.ownPayees
      },
      set(items) {
        this.payeesCopy = items;
        let payeesIds = [];
        items.forEach((item) => {
          payeesIds.push(item.id);
        })
        usePayeesStore().orderPayeeList(payeesIds).finally(() => {
          this.payeesCopy = null;
        })
      }
    },
    options() {
      return {
        animation: 200,
        group: 'description',
        disabled: false,
        ghostClass: 'ghost',
        handle: '.sortable-control'
      }
    }
  },
  methods: {
    setMenuRef: function(el, payeeId) {
      if (el) {
        this.menuRefs.set(payeeId, el);
      }
    },
    handleItemClick: function(payee) {
      if (this.$q.screen.gt.md) {
        const menu = this.menuRefs.get(payee.id);
        if (menu) {
          menu.show();
        }
      } else {
        this.openContextMenuModal(payee.id);
      }
    },
    openContextMenuModal: function(payeeId) {
      this.contextMenuModal.payee = _.find(this.ownPayees, {id: payeeId});
      this.contextMenuModal.isOpened = true;
    },
    closeContextMenuModal: function() {
      this.contextMenuModal.payee = null;
      this.contextMenuModal.isOpened = false;
    },
    openNextModal: function(value, payeeId) {
      this.closeContextMenuModal();
      if (value === 'cancel') {
        return;
      } else if (value === 'edit') {
        this.openEditModal(payeeId);
      } else if (value === 'delete') {
        this.openDeleteModal(payeeId);
      }
    },
    openSortModal: function() {
      this.sortModal.isOpened = true;
    },
    closeSortModal: function() {
      this.sortModal.isOpened = false;
    },
    updateOrder: function (type, direction) {
      usePayeesStore().changePayeesSortMode({type: type, direction: direction}).finally(() => {
        this.payeesCopy = null;
        this.closeSortModal();
      });
    },
    openEditModal: function (payeeId) {
      const payee = _.cloneDeep(_.find(this.ownPayees, {id: payeeId}));
      this.editModal.payeeId = payee.id;
      this.editModal.payeeName = payee.name;
      this.editModal.isOpened = true;
    },
    closeEditModal: function () {
      this.editModal.isOpened = false;
      this.editModal.payeeId = null;
      this.editModal.payeeName = null;
    },
    updatePayee: function(value, initialValue) {
      const payee = _.find(this.ownPayees, {name: initialValue});
      usePayeesStore().updatePayee({id: payee.id, name: value}).finally(() => {
        this.closeEditModal();
        this.payeesCopy = null;
      })
    },
    openDeleteModal: function (payeeId) {
      this.deleteModal.payee = _.cloneDeep(_.find(this.ownPayees, {id: payeeId}));
      this.deleteModal.isOpened = true;
    },
    closeDeleteModal: function () {
      this.deleteModal.isOpened = false;
      this.deleteModal.payee = null;
    },
    deletePayee: function(payeeId) {
      usePayeesStore().deletePayee(payeeId).finally(() => {
        this.closeDeleteModal();
        this.payeesCopy = null;
      })
    },
    openCreateModal: function () {
      this.createModal.isOpened = true;
    },
    closeCreateModal: function () {
      this.createModal.isOpened = false;
    },
    createPayee: function(value) {
      usePayeesStore().createPayee({name: value}).finally(() => {
        this.closeCreateModal();
        this.payeesCopy = null;
      })
    },
    updateArchivePayee: function (payeeId, value = false) {
      if (!value) {
        usePayeesStore().archivePayee(payeeId).finally(() => {
          this.payeesCopy = null;
        });
      } else {
        usePayeesStore().unarchivePayee(payeeId).finally(() => {
          this.payeesCopy = null;
        });
      }
    },
  }
})
</script>
