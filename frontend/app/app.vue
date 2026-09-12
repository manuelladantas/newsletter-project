<script setup lang="ts">
const { error, pending, running, runError, runNow, noDigestYet, isEmpty, bySource, formattedDate } = useDigest()
</script>

<template>
  <main class="p-4 md:max-w-3xl md:mx-auto md:p-8">
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

    <DigestSource v-for="(picks, source) in bySource" :key="source" :source="source" :picks="picks" />
  </main>
</template>
