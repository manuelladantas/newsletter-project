<script setup lang="ts">
defineProps<{
  pick: Pick
  favorited: boolean
  error?: string | null
}>()

const emit = defineEmits<{
  toggle: [pick: Pick]
}>()
</script>

<template>
  <li
    class="flex flex-col gap-ds-2 rounded-xl border border-neutral-200 bg-white p-ds-4 transition-shadow duration-150 hover:shadow-lg"
  >
    <div class="flex items-start justify-between gap-ds-3">
      <a
        :href="pick.url"
        target="_blank"
        rel="noopener"
        class="flex min-w-0 flex-1 flex-col gap-ds-3 text-text no-underline"
      >
        <DigestSourceTag :source="pick.source" />
        <span class="font-heading text-xl font-semibold leading-[1.25] break-words">{{ pick.title }}</span>
        <span class="font-body text-[13px] text-neutral-700">{{ pick.reason }}</span>
      </a>
      <button
        type="button"
        class="flex size-9 flex-none cursor-pointer items-center justify-center rounded-none border-0 bg-transparent transition-colors hover:bg-accent/10 active:bg-accent/18"
        :class="favorited ? 'text-accent-700' : 'text-text'"
        :aria-pressed="favorited"
        :aria-label="favorited ? 'Remove from favorites' : 'Add to favorites'"
        @click.prevent.stop="emit('toggle', pick)"
      >
        <svg
          width="18"
          height="18"
          viewBox="0 0 24 24"
          :fill="favorited ? 'currentColor' : 'none'"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M12 17.27L18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z" />
        </svg>
      </button>
    </div>
    <p v-if="error" class="m-0 font-body text-[13px] text-accent-800">{{ error }}</p>
  </li>
</template>
