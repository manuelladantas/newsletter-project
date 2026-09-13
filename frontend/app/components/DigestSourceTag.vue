<script setup lang="ts">
const props = defineProps<{
  source: string
}>()

const SOURCE_CLASSES: Record<string, string> = {
  'Hacker News': 'border-tag-hn text-tag-hn',
  ByteByteGo: 'border-tag-bbg text-tag-bbg',
  Medium: 'border-tag-medium text-tag-medium',
  'Dev.to': 'border-tag-devto text-tag-devto',
}

// Unknown sources fall back to the DS steel-accent outline tag.
const colorClasses = computed(() => SOURCE_CLASSES[props.source] ?? 'border-accent text-accent')
</script>

<template>
  <span
    class="inline-flex items-center self-start rounded-none border bg-transparent px-[10px] py-[3px] font-body text-[11px] tracking-[0.02em]"
    :class="colorClasses"
  >
    {{ source }}
  </span>
</template>
