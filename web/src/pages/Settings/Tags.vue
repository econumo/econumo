<template>
  <q-page class="settings-classification">
    <div class="settings-classification-wrapper">
      <!-- toolbar for mobile -->
      <div class="settings-toolbar-mobile settings-classification-toolbar-mobile">
        <div>
          <q-btn class="settings-toolbar-mobile-button" flat icon="arrow_back" @click="navigateTo('settings', true)" />
        </div>
        <div>
          <h4 class="settings-toolbar-mobile-title">{{ $t('modules.classifications.tags.pages.settings.header') }}</h4>
        </div>
        <div class="settings-toolbar-mobile-container">
          <q-btn class="settings-toolbar-mobile-button" flat icon="add_circle_outline" @click="openCreateTagModal()" />
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
      <h4 class="settings-label-header settings-classification-header">{{ $t('modules.classifications.tags.pages.settings.header') }}</h4>
      <div class="settings-classification-create-class">
        <q-btn class="settings-classification-create-class-btn econumo-btn -small -magenta" flat :label="$t('modules.classifications.tags.pages.settings.create_tag')" @click="openCreateTagModal()"/>
      </div>

      <div class="settings-classification-sort" v-if="tags.length > 1">
        <q-btn class="settings-classification-sort-btn"
               icon="sort"
               :label="$t('blocks.list.order_list')"
               :no-caps=true
               :flat=true
               :ripple=false
               @click="openSortModal"/>
      </div>

      <div class="settings-classification-container">
        <div class="settings-classification-empty" v-if="!tags.length">
          {{ $t('blocks.list.list_empty') }}
        </div>
        <div v-if="tags.length > 0">
          <q-list class="settings-classification-list">
            <draggable v-model="tags" @start="drag=true" @end="drag=false" v-bind="options" item-key="id">
              <template #item="{element}">
                <q-item class="settings-classification-list-item" clickable @click="handleItemClick(element)">
                  <q-item-section side class="settings-classification-list-item-sortable sortable-control cursor-pointer" @click.stop>
                    <q-icon name="drag_indicator"/>
                  </q-item-section>
                  <q-item-section :class="!!element.isArchived ? 'settings-classification-list-item-text -archived' : 'settings-classification-list-item-text'">
                    <span class="settings-classification-list-item-name econumo-truncate" :title="element.name">{{ element.name }}</span>
                    <div class="settings-classification-list-item-description-archived" v-if="element.isArchived">{{ $t('modules.classifications.tags.pages.settings.archived_item') }}</div>
                  </q-item-section>
                  <q-item-section side @click.stop>
                    <q-toggle :model-value="!element.isArchived"
                              @click="updateArchiveTag(element.id, !!element.isArchived)"/>
                  </q-item-section>
                  <q-item-section side v-if="$q.screen.gt.md" class="cursor-pointer settings-classification-list-item-check-section"> <!-- gt.md - desktop -->
                    <q-btn square flat icon="more_vert" class="account-transactions-item-check-button" @click.stop>
                      <q-menu cover auto-close class="account-transactions-item-check-button-menu" :ref="(el) => setMenuRef(el, element.id)">
                        <q-list class="account-transactions-item-check-button-list">
                          <q-item clickable @click="openEditTagModal(element.id)" class="account-transactions-item-check-button-item">
                            <q-item-section class="account-transactions-item-check-button-section">{{ $t('elements.button.edit.label') }}</q-item-section>
                          </q-item>
                          <q-item clickable @click="openDeleteTagModal(element.id)" class="account-transactions-item-check-button-item">
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
                            :header-label="contextMenuModal.tag.name"
                            :actions="[
                              {label: $t('elements.button.edit.label'), value: 'edit', context: contextMenuModal.tag.id},
                              {label: $t('elements.button.delete.label'), value: 'delete', context: contextMenuModal.tag.id},
                              {label: $t('elements.button.cancel.label'), value: 'cancel', context: contextMenuModal.tag.id}
                              ]"
                            v-on:cancel="closeContextMenuModal()"
                            v-on:proceed="openNextModal"
        />

        <sort-dialog-modal v-model="sortModal.isOpened" v-on:cancel="closeSortModal"
                           v-on:proceed="updateOrder"></sort-dialog-modal>

        <prompt-dialog-modal v-if="editTagModal.isOpened"
                             :header-label="$t('modules.classifications.tags.modals.edit.header')"
                             :input-value="editTagModal.tagName"
                             :input-label="$t('modules.classifications.tags.forms.tag.name.label')"
                             :cancel-label="$t('elements.button.cancel.label')"
                             :submit-label="$t('elements.button.update.label')"
                             :validation="[
                                val => isNotEmpty(val) || $t('modules.classifications.tags.forms.tag.name.validation.required_field'),
                                val => isValidTagName(val) || $t('modules.classifications.tags.forms.tag.name.validation.invalid_name')
                             ]"
                             v-on:close="closeEditTagModal"
                             v-on:submit="updateTag"
        />

        <confirmation-dialog-modal v-if="deleteModal.isOpened"
                                   :question-title="$t('modules.classifications.tags.modals.delete.title')"
                                   :question-label="deleteModal.tag?.name"
                                   :action-label="$t('elements.button.delete.label')"
                                   :cancel-label="$t('elements.button.cancel.label')"
                                   v-on:cancel="closeDeleteTagModal"
                                   v-on:proceed="deleteTag(deleteModal.tag.id)" />


        <prompt-dialog-modal v-if="createTagModal.isOpened"
                             :header-label="$t('modules.classifications.tags.modals.create.header')"
                             input-value=""
                             :input-label="$t('modules.classifications.tags.forms.tag.name.label')"
                             :cancel-label="$t('elements.button.cancel.label')"
                             :submit-label="$t('elements.button.create.label')"
                             :validation="[
                              val => isNotEmpty(val) || $t('modules.classifications.tags.forms.tag.name.validation.required_field'),
                              val => isValidTagName(val) || $t('modules.classifications.tags.forms.tag.name.validation.invalid_name')
                             ]"
                             v-on:close="closeCreateTagModal"
                             v-on:submit="createTag"
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
import {useTagsStore} from 'stores/tags';

export default defineComponent({
  name: 'SettingsTagsPage',
  mixins: [navigationMixin],
  components: {SortDialogModal, ConfirmationDialogModal, PromptDialogModal, draggable, ContextMenuModal},
  setup() {
    const validation = useValidation();
    return { ...validation };
  },
  data() {
    return {
      tagsCopy: null,
      menuRefs: new Map(),
      contextMenuModal: {
        isOpened: false,
        tag: null
      },
      editTagModal: {
        isOpened: false,
        tagId: null,
        tagName: null,
      },
      createTagModal: {
        isOpened: false
      },
      sortModal: {
        isOpened: false
      },
      deleteModal: {
        isOpened: false,
        tag: null
      }
    }
  },
  created() {
    if (this.$router.currentRoute.value.name === 'settingsTags') {
      useActiveAreaStore().setWorkspaceActiveArea();
    }
  },
  computed: {
    ...mapState(useUsersStore, ['userId']),
    ...mapState(useTagsStore, ['ownTags']),
    tags: {
      get() {
        return this.tagsCopy || this.ownTags
      },
      set(items) {
        this.tagsCopy = items;
        let tagsIds = [];
        items.forEach((item) => {
          tagsIds.push(item.id);
        })
        useTagsStore().orderTagList(tagsIds).finally(() => {
          this.tagsCopy = null;
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
    setMenuRef: function(el, tagId) {
      if (el) {
        this.menuRefs.set(tagId, el);
      }
    },
    handleItemClick: function(tag) {
      if (this.$q.screen.gt.md) {
        const menu = this.menuRefs.get(tag.id);
        if (menu) {
          menu.show();
        }
      } else {
        this.openContextMenuModal(tag.id);
      }
    },
    openContextMenuModal: function(tagId) {
      this.contextMenuModal.tag = _.find(this.ownTags, {id: tagId});
      this.contextMenuModal.isOpened = true;
    },
    closeContextMenuModal: function() {
      this.contextMenuModal.tag = null;
      this.contextMenuModal.isOpened = false;
    },
    openNextModal: function(value, tagId) {
      this.closeContextMenuModal();
      if (value === 'cancel') {
        return;
      } else if (value === 'edit') {
        this.openEditTagModal(tagId);
      } else if (value === 'delete') {
        this.openDeleteTagModal(tagId);
      }
    },
    openSortModal: function() {
      this.sortModal.isOpened = true;
    },
    closeSortModal: function() {
      this.sortModal.isOpened = false;
    },
    updateOrder: function (type, direction) {
      useTagsStore().changeTagsSortMode({type: type, direction: direction}).finally(() => {
        this.tagsCopy = null;
        this.closeSortModal();
      });
    },
    openEditTagModal: function (tagId) {
      const tag = _.find(this.ownTags, {id: tagId});
      this.editTagModal.tagId = tag.id;
      this.editTagModal.tagName = tag.name;
      this.editTagModal.isOpened = true;
    },
    closeEditTagModal: function () {
      this.editTagModal.isOpened = false;
      this.editTagModal.tagId = null;
      this.editTagModal.tagName = null;
    },
    updateTag: function (value, initialValue) {
      const tag = _.find(this.tags, {name: initialValue});
      useTagsStore().updateTag({id: tag.id, name: value}).finally(() => {
        this.closeEditTagModal();
        this.tagsCopy = null;
      })
    },
    openDeleteTagModal: function (tagId) {
      this.deleteModal.tag = _.cloneDeep(_.find(this.tags, {id: tagId}));
      this.deleteModal.isOpened = true;
    },
    closeDeleteTagModal: function () {
      this.deleteModal.isOpened = false;
      this.deleteModal.tag = null;
    },
    deleteTag: function (tagId) {
      useTagsStore().deleteTag(tagId).finally(() => {
        this.closeDeleteTagModal();
        this.tagsCopy = null;
      })
    },
    openCreateTagModal: function () {
      this.createTagModal.isOpened = true;
    },
    closeCreateTagModal: function () {
      this.createTagModal.isOpened = false;
    },
    createTag: function (value) {
      useTagsStore().createTag({name: value}).finally(() => {
        this.closeCreateTagModal();
        this.tagsCopy = null;
      })
    },
    updateArchiveTag: function (tagId, value = false) {
      if (!value) {
        useTagsStore().archiveTag(tagId).finally(() => {
          this.tagsCopy = null;
        });
      } else {
        useTagsStore().unarchiveTag(tagId).finally(() => {
          this.tagsCopy = null;
        });
      }
    },
  }
})
</script>
