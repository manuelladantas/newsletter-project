<script setup lang="ts">
defineProps<{
  source: string
  picks: Pick[]
}>()
</script>

<template>
  <section class="mb-6">
    <h2 class="text-lg font-semibold mb-2 md:text-xl">{{ source }}</h2>
    <ol class="list-decimal pl-5 flex flex-col gap-3">
      <DigestPick v-for="p in picks" :key="p.url" :pick="p" />
    </ol>
  </section>
</template>
