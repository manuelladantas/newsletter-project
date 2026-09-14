<script setup lang="ts">
const { data, pending, error, isEmpty, showPagination, page, goTo, remove, removeErrors } = useFavoritesList()
</script>

<template>
  <div>
    <header class="mb-ds-6">
      <h1 class="m-0 font-heading text-[34px] font-semibold uppercase leading-none tracking-[0.02em]">Favorites</h1>
    </header>

    <p v-if="pending" class="font-body text-sm text-neutral-700">Loading…</p>
    <p v-else-if="error" class="font-body text-sm text-accent-800">Could not load favorites: {{ error.message }}</p>
    <p v-else-if="isEmpty" class="font-body text-sm text-neutral-700">No favorites yet.</p>

    <ul v-else-if="data" class="m-0 flex list-none flex-col gap-ds-4 p-0">
      <DigestPick
        v-for="f in data.items"
        :key="f.id"
        :pick="f"
        :favorited="true"
        :error="removeErrors[f.url]"
        @toggle="remove(f)"
      />
    </ul>

    <FavoritesPagination v-if="showPagination && data" :page="page" :total-pages="data.totalPages" @go="goTo" />
  </div>
</template>
