<script setup lang="ts">
const { error, pending, running, runError, runNow, noDigestYet, isEmpty, bySource, formattedDate } = useDigest()
</script>

<template>
  <main class="mx-auto max-w-[760px] px-4 py-ds-8 md:px-ds-6">
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
      <DigestSource v-for="(picks, source) in bySource" :key="source" :source="source" :picks="picks" />
    </div>
  </main>
</template>
