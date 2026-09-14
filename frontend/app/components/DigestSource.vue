<script setup lang="ts">
defineProps<{
  source: string
  picks: Pick[]
  isFavorited: (url: string) => boolean
  toggleErrors: Record<string, string>
}>()

const emit = defineEmits<{
  toggle: [pick: Pick]
}>()
</script>

<template>
  <ul class="m-0 flex list-none flex-col gap-ds-4 p-0">
    <DigestPick
      v-for="p in picks"
      :key="p.url"
      :pick="p"
      :favorited="isFavorited(p.url)"
      :error="toggleErrors[p.url]"
      @toggle="emit('toggle', $event)"
    />
  </ul>
</template>
