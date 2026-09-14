<script setup lang="ts">
const { error, pending, running, runError, runNow, noDigestYet, isEmpty, bySource, formattedDate } = useDigest()
const { isFavorited, toggle, toggleErrors } = useFavorites()
</script>

<template>
  <div>
    <DigestHeader
      :formatted-date="formattedDate"
      :running="running"
      :run-error="runError"
      @run="runNow"
    />

    <DigestStatus
      :pending="pending"
      :no-digest-yet="noDigestYet"
      :error="error"
      :is-empty="isEmpty"
    />

    <div class="flex flex-col gap-ds-4">
      <DigestSource
        v-for="(picks, source) in bySource"
        :key="source"
        :source="source"
        :picks="picks"
        :is-favorited="isFavorited"
        :toggle-errors="toggleErrors"
        @toggle="toggle"
      />
    </div>
  </div>
</template>
