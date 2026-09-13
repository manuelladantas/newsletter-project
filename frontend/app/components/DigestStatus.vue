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
  <p v-if="pending" class="font-body text-sm text-neutral-700">Loading…</p>
  <p v-else-if="noDigestYet" class="font-body text-sm text-neutral-700">
    No digest yet. It runs every morning at 9am, or press "Run now".
  </p>
  <p v-else-if="error" class="font-body text-sm text-accent-800">Could not load digest: {{ error.message }}</p>
  <p v-else-if="isEmpty" class="font-body text-sm text-neutral-700">
    Today's run produced no picks — every source was skipped. Check the backend logs.
  </p>
</template>
