<template>
  <q-dialog class="settings-classification-modal" v-model="isEditModalOpened" @hide="closeEditModal" no-backdrop-dismiss>
    <q-card class="settings-classification-modal-card">
      <q-form
        ref="categoryEditForm"
        @submit="submitEditForm($refs.categoryEditForm)"
        class="settings-classification-modal-form"
      >
        <q-card-section>
          <div class="settings-toolbar-mobile">
            <div>
              <q-btn class="settings-toolbar-mobile-button" flat icon="arrow_back" v-close-popup />
            </div>
            <div>
              <h4 class="settings-toolbar-mobile-title">{{ $t('modules.classifications.categories.modals.edit.header') }}</h4>
            </div>
            <div class="settings-toolbar-mobile-container"></div>
          </div>

          <div class="settings-classification-modal-header">{{ $t('modules.classifications.categories.modals.edit.header') }}</div>

          <div class="settings-classification-modal-toggle">
            <q-btn-toggle
              class="settings-classification-modal-toggle-btn"
              v-model="editCategory.type"
              disable
              readonly
              toggle-color="primary"
              :options="[
                {label: $t('modules.classifications.categories.forms.category.type.income'), value: 'income'},
                {label: $t('modules.classifications.categories.forms.category.type.expense'), value: 'expense'}
              ]"
            />
          </div>
          <div class="settings-classification-modal-control">
            <q-input
              outlined
              class="form-input full-width"
              v-model="editCategory.name"
              :label="$t('modules.classifications.categories.forms.category.name.label')"
              lazy-rules
              :rules="[
                (val: string) => isNotEmpty(val) || $t('modules.classifications.categories.forms.category.name.validation.required_field'),
                (val: string) => isValidCategoryName(val) || $t('modules.classifications.categories.forms.category.name.validation.invalid_name')
              ]"
              maxlength="64">
              <template v-slot:before>
                <div class="settings-classification-modal-control-icon">
                  <q-icon class="settings-classification-modal-control-icon-img" :name="editCategory.icon"/>
                </div>
              </template>
            </q-input>
          </div>
        </q-card-section>

        <q-card-section class="responsive-modal-section-icon">
          <responsive-modal-icons 
            class="responsive-modal-icons-container" 
            :header="$t('modules.classifications.categories.forms.category.icon.label')" 
            :icon="editCategory.icon" 
            @update-icon="value => editCategory.icon = value" 
          />
        </q-card-section>

        <q-card-actions class="settings-classification-modal-actions">
          <q-btn class="econumo-btn -medium -grey settings-classification-modal-actions-button" flat :label="$t('elements.button.cancel.label')" @click="closeEditModal"/>
          <q-btn class="econumo-btn -medium -magenta settings-classification-modal-actions-button" flat :label="$t('elements.button.update.label')" type="submit"/>
        </q-card-actions>
      </q-form>
    </q-card>
  </q-dialog>
</template>

<script lang="ts">
import { defineComponent } from 'vue';
import { useEditCategory } from '../../composables/categories/useEditCategory';
import { useValidation } from '../../composables/useValidation';
import ResponsiveModalIcons from '../ResponsiveModal/Icons.vue';

export default defineComponent({
  name: 'CategoriesEditModal',
  components: {
    ResponsiveModalIcons
  },
  setup() {
    const { isNotEmpty, isValidCategoryName } = useValidation();
    const { isEditModalOpened, editCategory, closeEditModal, submitEditForm } = useEditCategory();

    return {
      isNotEmpty,
      isValidCategoryName,
      isEditModalOpened,
      editCategory,
      closeEditModal,
      submitEditForm
    };
  }
});
</script> 