<script setup lang="ts">
defineProps<{
  formattedDate: string
  running: boolean
  runError: string | null
}>()

const emit = defineEmits<{
  run: []
}>()
</script>

<template>
  <header class="mb-ds-6 flex flex-col gap-ds-3 md:flex-row md:flex-wrap md:items-baseline md:gap-ds-4">
    <h1 class="m-0 font-heading text-[34px] font-semibold uppercase leading-none tracking-[0.02em]">Daily Digest</h1>
    <p v-if="formattedDate" class="font-body text-[13px] text-neutral-700 md:ml-auto">{{ formattedDate }}</p>
    <button
      type="button"
      class="cursor-pointer self-start rounded-none border border-divider bg-transparent px-ds-4 py-ds-2 font-heading text-sm font-semibold uppercase tracking-[0.02em] text-text transition-colors hover:bg-text/7 active:bg-text/14 disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-transparent md:self-center"
      :disabled="running"
      @click="emit('run')"
    >
      {{ running ? 'Running…' : 'Run now' }}
    </button>
    <p v-if="runError" class="font-body text-[13px] text-accent-800 md:basis-full">{{ runError }}</p>
  </header>
</template>
