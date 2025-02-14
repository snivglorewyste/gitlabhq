<script>
import { GlTokenSelector } from '@gitlab/ui';
import { debounce } from 'lodash';
import { getIdFromGraphQLId } from '~/graphql_shared/utils';
import { DEFAULT_DEBOUNCE_AND_THROTTLE_MS } from '~/lib/utils/constants';

import groupWorkItemsQuery from '../../graphql/group_work_items.query.graphql';
import projectWorkItemsQuery from '../../graphql/project_work_items.query.graphql';
import {
  WORK_ITEMS_TYPE_MAP,
  I18N_WORK_ITEM_SEARCH_INPUT_PLACEHOLDER,
  sprintfWorkItem,
} from '../../constants';

export default {
  components: {
    GlTokenSelector,
  },
  inject: ['isGroup'],
  props: {
    value: {
      type: Array,
      required: false,
      default: () => [],
    },
    fullPath: {
      type: String,
      required: true,
    },
    childrenType: {
      type: String,
      required: false,
      default: '',
    },
    childrenIds: {
      type: Array,
      required: false,
      default: () => [],
    },
    parentWorkItemId: {
      type: String,
      required: true,
    },
    areWorkItemsToAddValid: {
      type: Boolean,
      required: false,
      default: false,
    },
  },
  apollo: {
    availableWorkItems: {
      query() {
        return this.isGroup ? groupWorkItemsQuery : projectWorkItemsQuery;
      },
      variables() {
        return {
          fullPath: this.fullPath,
          searchTerm: this.search?.title || this.search,
          types: this.childrenType ? [this.childrenType] : [],
          in: this.search ? 'TITLE' : undefined,
        };
      },
      skip() {
        return !this.searchStarted;
      },
      update(data) {
        return data.workspace.workItems.nodes.filter(
          (wi) => !this.childrenIds.includes(wi.id) && this.parentWorkItemId !== wi.id,
        );
      },
    },
  },
  data() {
    return {
      availableWorkItems: [],
      search: '',
      searchStarted: false,
    };
  },
  computed: {
    workItemsToAdd: {
      get() {
        return this.value;
      },
      set(workItemsToAdd) {
        this.$emit('input', workItemsToAdd);
      },
    },
    isLoading() {
      return this.$apollo.queries.availableWorkItems.loading;
    },
    addInputPlaceholder() {
      return sprintfWorkItem(I18N_WORK_ITEM_SEARCH_INPUT_PLACEHOLDER, this.childrenTypeName);
    },
    childrenTypeName() {
      return WORK_ITEMS_TYPE_MAP[this.childrenType]?.name;
    },
    tokenSelectorContainerClass() {
      return !this.areWorkItemsToAddValid ? 'gl-inset-border-1-red-500!' : '';
    },
  },
  created() {
    this.debouncedSearchKeyUpdate = debounce(this.setSearchKey, DEFAULT_DEBOUNCE_AND_THROTTLE_MS);
  },
  methods: {
    getIdFromGraphQLId,
    setSearchKey(value) {
      this.search = value;
    },
    handleFocus() {
      this.searchStarted = true;
      this.$emit('searching', true);
    },
    handleMouseOver() {
      this.timeout = setTimeout(() => {
        this.searchStarted = true;
      }, DEFAULT_DEBOUNCE_AND_THROTTLE_MS);
    },
    handleMouseOut() {
      clearTimeout(this.timeout);
    },
    handleBlur() {
      this.$emit('searching', false);
    },
    focusInputText() {
      this.$nextTick(() => {
        if (this.areWorkItemsToAddValid) {
          this.$refs.tokenSelector.$el.querySelector('input[type="text"]').focus();
        }
      });
    },
  },
};
</script>
<template>
  <gl-token-selector
    ref="tokenSelector"
    v-model="workItemsToAdd"
    :dropdown-items="availableWorkItems"
    :loading="isLoading"
    :placeholder="addInputPlaceholder"
    menu-class="gl-dropdown-menu-wide dropdown-reduced-height gl-min-h-7!"
    :container-class="tokenSelectorContainerClass"
    data-testid="work-item-token-select-input"
    @text-input="debouncedSearchKeyUpdate"
    @focus="handleFocus"
    @mouseover.native="handleMouseOver"
    @mouseout.native="handleMouseOut"
    @token-add="focusInputText"
    @token-remove="focusInputText"
    @blur="handleBlur"
  >
    <template #token-content="{ token }"> {{ token.iid }} {{ token.title }} </template>
    <template #dropdown-item-content="{ dropdownItem }">
      <div class="gl-display-flex">
        <div class="gl-text-secondary gl-font-sm gl-mr-4">{{ dropdownItem.iid }}</div>
        <div class="gl-text-truncate">{{ dropdownItem.title }}</div>
      </div>
    </template>
  </gl-token-selector>
</template>
