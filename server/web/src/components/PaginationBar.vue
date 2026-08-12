<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  page: number
  pageSize: number
  total: number
  pageSizeOptions?: number[]
  showPageSize?: boolean
}>(), {
  pageSizeOptions: () => [10, 20, 50],
  showPageSize: true
})

const emit = defineEmits<{
  change: [page: number]
  pageSizeChange: [pageSize: number]
}>()

const jumpPage = ref(String(props.page))
const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const safePage = computed(() => Math.min(Math.max(1, props.page), totalPages.value))
const rangeStart = computed(() => props.total === 0 ? 0 : (safePage.value - 1) * props.pageSize + 1)
const rangeEnd = computed(() => Math.min(safePage.value * props.pageSize, props.total))
const visiblePages = computed(() => {
  const start = Math.max(1, Math.min(safePage.value - 2, totalPages.value - 4))
  const end = Math.min(totalPages.value, start + 4)
  return Array.from({ length: end - start + 1 }, (_, index) => start + index)
})

watch(() => props.page, (value) => {
  jumpPage.value = String(value)
})

function goTo(page: number) {
  const next = Math.min(Math.max(1, page), totalPages.value)
  if (next !== props.page) emit('change', next)
}

function submitJump() {
  const value = Number.parseInt(jumpPage.value, 10)
  if (Number.isNaN(value)) {
    jumpPage.value = String(safePage.value)
    return
  }
  goTo(value)
  jumpPage.value = String(Math.min(Math.max(1, value), totalPages.value))
}

function changePageSize(event: Event) {
  emit('pageSizeChange', Number((event.target as HTMLSelectElement).value))
}
</script>

<template>
  <nav class="pagination" aria-label="分页">
    <div class="pagination-summary">
      <span>共 {{ total.toLocaleString() }} 条</span>
      <span v-if="total">当前 {{ rangeStart }}-{{ rangeEnd }}</span>
    </div>

    <div class="pagination-pages">
      <button class="pagination-button pagination-edge" type="button" title="第一页" :disabled="safePage <= 1" @click="goTo(1)">«</button>
      <button class="pagination-button" type="button" title="上一页" :disabled="safePage <= 1" @click="goTo(safePage - 1)">‹</button>
      <button v-for="item in visiblePages" :key="item" :class="['pagination-button', { active: item === safePage }]" type="button" :aria-current="item === safePage ? 'page' : undefined" @click="goTo(item)">{{ item }}</button>
      <button class="pagination-button" type="button" title="下一页" :disabled="safePage >= totalPages" @click="goTo(safePage + 1)">›</button>
      <button class="pagination-button pagination-edge" type="button" title="最后一页" :disabled="safePage >= totalPages" @click="goTo(totalPages)">»</button>
    </div>

    <div class="pagination-options">
      <label v-if="showPageSize">每页<select :value="pageSize" @change="changePageSize"><option v-for="option in pageSizeOptions" :key="option" :value="option">{{ option }}</option></select></label>
      <label class="pagination-jump">跳至<input v-model="jumpPage" inputmode="numeric" :aria-label="`跳转页码，共 ${totalPages} 页`" @keydown.enter="submitJump" @blur="submitJump">页</label>
      <span>共 {{ totalPages }} 页</span>
    </div>
  </nav>
</template>
