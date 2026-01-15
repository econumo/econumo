<template>
  <q-page class="settings-classification">
    <div class="settings-classification-wrapper">
      <!-- toolbar for mobile -->
      <div class="settings-toolbar-mobile settings-classification-toolbar-mobile">
        <div>
          <q-btn class="settings-toolbar-mobile-button" flat icon="arrow_back" @click="navigateTo('settings', true)" />
        </div>
        <div>
          <h4 class="settings-toolbar-mobile-title">{{ $t('modules.classifications.categories.pages.settings.header') }}</h4>
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
      <h4 class="settings-label-header settings-classification-header">{{ $t('modules.classifications.categories.pages.settings.header') }}</h4>

      <div class="settings-classification-create-class">
        <q-btn class="settings-classification-create-class-btn econumo-btn -small -magenta" flat :label="$t('modules.classifications.categories.pages.settings.create_category')" @click="openCreateModal()"/>
      </div>

      <div class="settings-classification-sort" v-if="categories.length > 1">
        <q-btn class="settings-classification-sort-btn"
               icon="sort"
               :label="$t('blocks.list.order_list')"
               :no-caps=true
               :flat=true
               :ripple=false
               @click="openSortDialog"/>
      </div>

      <div class="settings-classification-container">
        <div class="settings-classification-empty" v-if="!categories.length">
          {{ $t('blocks.list.list_empty') }}
        </div>
        <div v-if="categories.length > 0">
          <q-list class="settings-classification-list">
            <draggable v-model="categories" @start="drag=true" @end="drag=false" v-bind="options" item-key="id">
              <template #item="{element}">
                <q-item class="settings-classification-list-item" clickable @click="handleItemClick(element)">
                  <q-item-section side class="settings-classification-list-item-sortable sortable-control cursor-pointer" @click.stop>
                    <q-icon name="drag_indicator" />
                  </q-item-section>
                  <q-item-section avatar class="settings-classification-list-item-avatar">
                    <q-icon :name="element.icon" />
                  </q-item-section>
                  <q-item-section :class="!!element.isArchived ? 'settings-classification-list-item-text -archived' : 'settings-classification-list-item-text'">
                    <span class="settings-classification-list-item-name econumo-truncate" :title="element.name">{{ element.name }}</span>
                    <div class="settings-classification-list-item-description-archived" v-if="element.isArchived">{{ $t('modules.classifications.categories.pages.settings.archived_item') }}</div>
                  </q-item-section>
                  <q-item-section side @click.stop>
                    <q-toggle :model-value="!element.isArchived" @click="updateArchiveCategory(element.id, !!element.isArchived)"/>
                  </q-item-section>
                  <q-item-section side v-if="$q.screen.gt.md" class="cursor-pointer settings-classification-list-item-check-section">
                    <q-btn square flat icon="more_vert" class="account-transactions-item-check-button" @click.stop>
                      <q-menu cover auto-close class="account-transactions-item-check-button-menu" :ref="(el) => setMenuRef(el, element.id)">
                        <q-list class="account-transactions-item-check-button-list">
                          <q-item clickable @click="openEditModal(element)" class="account-transactions-item-check-button-item">
                            <q-item-section class="account-transactions-item-check-button-section">{{ $t('elements.button.edit.label') }}</q-item-section>
                          </q-item>
                          <!-- <q-item clickable @click="openReplaceModal(element.id)" class="account-transactions-item-check-button-item">
                            <q-item-section class="account-transactions-item-check-button-section">{{ $t('elements.button.replace.label') }}</q-item-section>
                          </q-item> -->
                          <q-item clickable @click="openDeleteModal(element)" class="account-transactions-item-check-button-item">
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
        <context-menu-modal v-if="isContextMenuOpened"
                            :header-label="contextMenuCategory.name"
                            :actions="contextMenuActions"
                            v-on:cancel="closeContextMenu"
                            v-on:proceed="openNextModal"
        />

        <sort-dialog-modal v-model="isSortDialogOpened" v-on:cancel="closeSortDialog"
                           v-on:proceed="(type: 'name' | 'type', direction: 'asc' | 'desc') => submitSortDialog(type, direction).finally(() => { categoriesCopy = null; })"></sort-dialog-modal>

        <categories-edit-modal />

        <categories-replace-modal :categories="categories" @update:categoriesCopy="categoriesCopy = $event" />

        <confirmation-dialog-modal v-if="isDeleteModalOpened"
                                   :question-title="$t('modules.classifications.categories.modals.delete.title')"
                                   :question-label="deleteCategory.name"
                                   :action-label="$t('elements.button.delete.label')"
                                   :cancel-label="$t('elements.button.cancel.label')"
                                   v-on:cancel="closeDeleteModal"
                                   v-on:proceed="submitDeleteCategory().finally(() => { categoriesCopy = null; })" />

        <categories-create-modal />
      </teleport>
    </div>
  </q-page>
</template>

<script lang="ts">
import {defineComponent} from 'vue';
import {useQuasar} from 'quasar';
import {navigationMixin} from '../../mixins/navigationMixin';
import {useValidation} from '../../composables/useValidation';
import {useCreateCategory} from '../../composables/categories/useCreateCategory';
import {useEditCategory} from '../../composables/categories/useEditCategory';
import {useDeleteCategory} from '../../composables/categories/useDeleteCategory';
import {useReplaceCategory} from '../../composables/categories/useReplaceCategory';
import {useContextMenu} from '../../composables/categories/useContextMenu';
import {useSortDialog} from '../../composables/categories/useSortDialog';
import _ from 'lodash';
import draggable from 'vuedraggable';
import ConfirmationDialogModal from '../../components/ConfirmationDialogModal.vue';
import ContextMenuModal from '../../components/ContextMenuModal.vue';
import SortDialogModal from '../../components/SortDialogModal.vue';
import CategoriesEditModal from '../../components/Categories/EditModal.vue';
import CategoriesReplaceModal from '../../components/Categories/ReplaceModal.vue';
import CategoriesCreateModal from '../../components/Categories/CreateModal.vue';
import { mapState } from 'pinia';
import {useUsersStore} from 'stores/users';
import {useCategoriesStore} from 'stores/categories';
import {useActiveAreaStore} from 'stores/active-area';

interface Category {
  id: string;
  name: string;
  type: string;
  icon: string;
  isArchived?: boolean;
  ownerUserId: string;
}

interface CategoryOption {
  label: string;
  value: string;
  icon: string;
}

interface ContextMenuAction {
  label: string;
  value: string;
  context: string;
  isHidden: boolean;
}

export default defineComponent({
  name: 'SettingsCategoriesPage',
  mixins: [navigationMixin],
  components: {
    draggable, 
    ContextMenuModal, 
    ConfirmationDialogModal, 
    SortDialogModal, 
    CategoriesEditModal,
    CategoriesReplaceModal,
    CategoriesCreateModal
  },
  setup() {
    const $q = useQuasar();
    const validation = useValidation();
    const { openCreateModal } = useCreateCategory();
    const { openEditModal } = useEditCategory();
    const deleteCategory = useDeleteCategory();
    const { openReplaceModal } = useReplaceCategory();
    const contextMenu = useContextMenu();
    const sortDialog = useSortDialog();
    return { 
      $q,
      ...validation, 
      openCreateModal,
      openEditModal,
      ...deleteCategory, 
      openReplaceModal,
      ...contextMenu, 
      ...sortDialog 
    };
  },
  data() {
    return {
      categoriesCopy: null as Category[] | null,
      selectedCategoryId: null as string | null,
      drag: false,
      menuRefs: new Map()
    }
  },
  created() {
    if ((this.$router as any).currentRoute.value.name === 'settingsCategories') {
      useActiveAreaStore().setWorkspaceActiveArea();
    }
  },
  computed: {
    ...mapState(useUsersStore, ['userId']),
    ...mapState(useCategoriesStore, ['ownCategories']),
    categories: {
      get(): Category[] {
        return this.categoriesCopy || (_.filter(this.ownCategories, {ownerUserId: this.userId}) as unknown as Category[])
      },
      set(items: Category[]) {
        this.categoriesCopy = items;
        const categoriesIds: string[] = [];
        items.forEach((item) => {
          categoriesIds.push(item.id);
        })
        useCategoriesStore().orderCategoryList(categoriesIds).finally(() => {
          this.categoriesCopy = null;
        });
      }
    },
    selectedCategory(): Category | undefined {
      return _.find(this.ownCategories, {id: this.selectedCategoryId}) as unknown as Category | undefined
    },
    options() {
      return {
        animation: 200,
        group: 'description',
        disabled: false,
        ghostClass: 'ghost',
        handle: '.sortable-control'
      }
    },
    categoriesOptions(): CategoryOption[] {
      const result: CategoryOption[] = [];
      if (this.selectedCategory) {
        this.categories.forEach(item => {
          if (this.selectedCategory?.type === item.type) {
            result.push({
              label: item.name,
              value: item.id,
              icon: item.icon
            })
          }
        })
      }
      return result;
    },
    contextMenuActions(): ContextMenuAction[] {
      return [
        {label: (this.$t as any)('elements.button.edit.label'), value: 'edit', context: this.contextMenuCategory.id, isHidden: false},
        // {label: (this.$t as any)('elements.button.replace.label'), value: 'replace', context: this.contextMenuCategory.id},
        {label: (this.$t as any)('elements.button.delete.label'), value: 'delete', context: this.contextMenuCategory.id, isHidden: false},
        {label: (this.$t as any)('elements.button.cancel.label'), value: 'cancel', context: this.contextMenuCategory.id, isHidden: false}
      ];
    }
  },
  methods: {
    setMenuRef(el: any, categoryId: string): void {
      if (el) {
        this.menuRefs.set(categoryId, el);
      }
    },
    handleItemClick(category: Category): void {
      if (this.$q.screen.gt.md) {
        // Desktop: open menu
        const menu = this.menuRefs.get(category.id);
        if (menu) {
          menu.show();
        }
      } else {
        // Mobile: open context menu modal
        this.openContextMenu(category);
      }
    },
    openNextModal(value: string, categoryId: string): void {
      this.closeContextMenu();
      if (value === 'cancel') {
        // Just close the modal, which is already done above
        return;
      } else if (value === 'edit') {
        const category = _.find(this.categories, {id: categoryId});
        if (category) this.openEditModal(category);
      } else if (value === 'replace') {
        this.openReplaceModal(categoryId);
      } else if (value === 'delete') {
        const category = _.find(this.categories, {id: categoryId});
        if (category) this.openDeleteModal(category);
      }
    },
    updateArchiveCategory(categoryId: string, value = false): void {
      if (!value) {
        useCategoriesStore().archiveCategory(categoryId).finally(() => {
          this.categoriesCopy = null;
        });
      } else {
        useCategoriesStore().unarchiveCategory(categoryId).finally(() => {
          this.categoriesCopy = null;
        });
      }
    }
  }
});
</script>
