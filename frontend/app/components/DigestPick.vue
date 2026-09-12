<script setup lang="ts">
defineProps<{
  pick: Pick
}>()
</script>

<template>
  <li>
    <a :href="pick.url" target="_blank" rel="noopener" class="font-semibold text-inherit">
      {{ pick.title }}
    </a>
    <p class="mt-1 text-sm opacity-75">{{ pick.reason }}</p>
  </li>
</template>
