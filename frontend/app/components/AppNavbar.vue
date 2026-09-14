<template>
  <nav class="border-b border-divider">
    <ul class="mx-auto m-0 flex max-w-[760px] list-none gap-ds-6 px-4 p-0 md:px-ds-6">
      <li v-for="link in links" :key="link.to">
        <NuxtLink
          :to="link.to"
          class="block border-b-2 border-transparent py-ds-3 font-heading text-sm font-semibold uppercase tracking-[0.02em] text-neutral-700 no-underline transition-colors hover:text-text"
          active-class="border-accent text-text"
        >
          {{ link.label }}
        </NuxtLink>
      </li>
    </ul>
  </nav>
</template>

<script setup lang="ts">
const links = [
  { to: '/', label: 'Digest' },
  { to: '/favorites', label: 'Favorites' },
]
</script>
