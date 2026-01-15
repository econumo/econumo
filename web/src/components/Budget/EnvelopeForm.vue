<template>
  <div class="budget-form-wrapper">
    <div class="responsive-modal-control">
      <q-input
        class="form-input full-width"
        outlined
        :placeholder="$t('modules.budget.form.budget_envelope.name.placeholder')"
        v-model="name"
        :label="$t('modules.budget.form.budget_envelope.name.label')"
        lazy-rules
        autofocus
        :rules="[
                  val => isNotEmpty(val) || $t('modules.budget.form.budget_envelope.name.validation.required_field'),
                  val => isValidBudgetEnvelopeName(val) || $t('modules.budget.form.budget_envelope.name.validation.invalid_name')
                ]"
        maxlength="64">
        <template v-slot:before>
          <div class="responsive-modal-control-icon">
            <q-icon class="responsive-modal-control-icon-img" :name="icon"/>
          </div>
        </template>
      </q-input>
    </div>
    <div class="responsive-modal-control">
      <currency-select
        v-model="currency"
        :outlined="true"
        :borderless="true"
        custom-class="form-select"
        :label="$t('modules.budget.form.budget_envelope.currency.label')"
        :rules="[val => !!val || $t('modules.budget.form.budget_envelope.currency.validation.required_field')]"
      />
    </div>
    <div class="responsive-modal-control">
      <q-select class="form-select budget-envelope-categories"
                outlined
                v-model="selectedCategories"
                use-input
                multiple
                emit-value
                map-options
                :options="categoriesOptions"
                :label="$t('modules.budget.form.budget_envelope.categories.label')"
                @filter="filterCategories"
                @filter-abort="filterCategoriesAbort">
        <template v-slot:option="scope">
          <q-item v-bind="scope.itemProps" clickable @click="handleSelection(!scope.selected, scope.opt)">
            <q-item-section avatar class="budget-envelope-modal-control-avatar">
              <q-avatar size="30px" v-if="props.access.length > 1">
                <img :src="avatarUrl(accessList[scope.opt.ownerUserId].user.avatar, 30)" :alt="accessList[scope.opt.ownerUserId].user.name" :title="accessList[scope.opt.ownerUserId].user.name" width="30" height="30" />
              </q-avatar>
              <q-avatar size="30px" :icon="scope.opt.icon" v-else />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ scope.opt.label }}</q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-toggle class="budget-envelope-categories-toggle" :model-value="scope.selected"
                        @click.stop
                        @update:model-value="(val) => handleSelection(val, scope.opt)" />
            </q-item-section>
          </q-item>
        </template>
      </q-select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { navigationMixin } from '../../mixins/navigationMixin';
import { Id } from '../../modules/types';
import { BudgetElementDto } from '../../modules/api/v1/dto/budget.dto';
import { CategoryDto, CategoryType } from '@shared/dto/category.dto';
import { UserAccessDto } from '@shared/dto/access.dto';
import { useCurrenciesStore } from 'stores/currencies';
import { useCategoriesStore } from 'stores/categories';
import { Icon } from '@shared/types';
import { useAvatar } from '../../composables/useAvatar';
import { useValidation } from '../../composables/useValidation';
import CurrencySelect from '../../components/CurrencySelect.vue';
import _ from 'lodash';

defineOptions({
  name: 'EnvelopeForm',
  mixins: [navigationMixin]
});

const { isNotEmpty, isValidBudgetEnvelopeName } = useValidation();
const { avatarUrl } = useAvatar();

const props = defineProps<{
  id: Id,
  name: string,
  icon: Icon,
  currencyId: Id,
  categoriesIds: Id[],
  elements: BudgetElementDto[],
  access: UserAccessDto[],
}>();

const emit = defineEmits(['update-name', 'update-currency', 'update-categories']);

const name = computed({
  get() {
    return props.name
  },
  set(value: string) {
    emit('update-name', value);
  }
});

const icon = computed(() => props.icon);

const accessList = computed(() => {
  const result: Record<string, UserAccessDto> = {};
  props.access.forEach((item) => {
    result[item.user.id] = item;
  });
  return result;
});

const currenciesStore = useCurrenciesStore();
const currency = computed({
  get() {
    if (!props.currencyId) {
      return null;
    }
    const currencyData = currenciesStore.currenciesHash[props.currencyId];
    return {
      value: currencyData.id,
      label: currencyData.code
    };
  },
  set(value: { label: string; value: Id } | null) {
    emit('update-currency', value?.value ?? null);
  }
});

const categoriesStore = useCategoriesStore();
const categorySearchFilter = ref('');

function filterCategories(val: string, update: (fn: () => void) => void): void {
  update(() => {
    categorySearchFilter.value = val.toLowerCase();
  });
}

function filterCategoriesAbort(): void {
  categorySearchFilter.value = '';
}

const categoriesOptions = computed(() => {
  return categoriesStore.categories
    .filter((item: CategoryDto) => {
      if (item.isArchived) return false;
      if (item.type === CategoryType.INCOME) return false;
      if (!categorySearchFilter.value) return true;
      return item.name.toLowerCase().includes(categorySearchFilter.value);
    })
    .map((item: CategoryDto) => ({
      value: item.id,
      label: item.name,
      description: item.name,
      icon: item.icon,
      ownerUserId: item.ownerUserId
    }));
});

const selectedCategories = computed({
  get() {
    return categoriesStore.categories
      .filter((item: CategoryDto) => props.categoriesIds.includes(item.id))
      .map((item: CategoryDto) => ({
        value: item.id,
        label: item.name,
        description: item.name,
        icon: item.icon,
        ownerUserId: item.ownerUserId
      }));
  },
  set(values: Array<{ value: Id; label: string; description: string; icon: Icon; ownerUserId: Id }>) {
    emit('update-categories', values.map(item => item.value));
  }
});

function handleSelection(checked: boolean, option: any): void {
  const currentValues = selectedCategories.value.map(item => ({ ...item }));

  if (checked) {
    if (!currentValues.some(item => item.value === option.value)) {
      currentValues.push({
        value: option.value,
        label: option.label,
        description: option.description,
        icon: option.icon,
        ownerUserId: option.ownerUserId
      });
    }
  } else {
    const index = currentValues.findIndex(item => item.value === option.value);
    if (index !== -1) {
      currentValues.splice(index, 1);
    }
  }

  emit('update-categories', currentValues.map(item => item.value));
}

</script>

