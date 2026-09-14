<script setup lang="ts">
defineProps<{
  page: number
  totalPages: number
}>()

const emit = defineEmits<{
  go: [page: number]
}>()
</script>

<template>
  <nav class="mt-ds-6 flex items-center justify-between gap-ds-4" aria-label="Pagination">
    <button
      type="button"
      class="cursor-pointer rounded-none border border-divider bg-transparent px-ds-4 py-ds-2 font-heading text-sm font-semibold uppercase tracking-[0.02em] text-text transition-colors hover:bg-text/7 active:bg-text/14 disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent"
      :disabled="page <= 1"
      @click="emit('go', page - 1)"
    >
      Prev
    </button>
    <p class="m-0 font-body text-[13px] text-neutral-700">Page {{ page }} of {{ totalPages }}</p>
    <button
      type="button"
      class="cursor-pointer rounded-none border border-divider bg-transparent px-ds-4 py-ds-2 font-heading text-sm font-semibold uppercase tracking-[0.02em] text-text transition-colors hover:bg-text/7 active:bg-text/14 disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent"
      :disabled="page >= totalPages"
      @click="emit('go', page + 1)"
    >
      Next
    </button>
  </nav>
</template>
