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
  <header class="flex flex-col gap-2 mb-6 md:flex-row md:flex-wrap md:items-baseline">
    <h1 class="text-2xl font-bold md:text-3xl">Daily Digest</h1>
    <p v-if="formattedDate" class="opacity-70">{{ formattedDate }}</p>
    <button
      class="self-start px-4 py-2 border border-current rounded-lg bg-transparent cursor-pointer disabled:opacity-50 disabled:cursor-default md:ml-auto md:self-center"
      :disabled="running"
      @click="emit('run')"
    >
      {{ running ? 'Running…' : 'Run now' }}
    </button>
    <p v-if="runError" class="text-red-700 md:basis-full">{{ runError }}</p>
  </header>
</template>
