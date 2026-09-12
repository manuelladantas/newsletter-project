<script setup lang="ts">
import type { NuxtError } from '#app'

defineProps<{
  pending: boolean
  noDigestYet: boolean
  error: NuxtError | null | undefined
  isEmpty: boolean
}>()
</script>

<template>
  <p v-if="pending" class="opacity-80">Loading…</p>
  <p v-else-if="noDigestYet" class="opacity-80">
    No digest yet. It runs every morning at 9am, or press "Run now".
  </p>
  <p v-else-if="error" class="opacity-80 text-red-700">Could not load digest: {{ error.message }}</p>
  <p v-else-if="isEmpty" class="opacity-80">
    Today's run produced no picks — every source was skipped. Check the backend logs.
  </p>
</template>
