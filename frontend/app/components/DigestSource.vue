<script setup lang="ts">
defineProps<{
  source: string
  picks: Pick[]
}>()
</script>

<template>
  <ul class="m-0 flex list-none flex-col gap-ds-4 p-0">
    <DigestPick v-for="p in picks" :key="p.url" :pick="p" />
  </ul>
</template>
