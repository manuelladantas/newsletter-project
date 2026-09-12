export function useDigest() {
  const { apiBase } = useRuntimeConfig().public

  // Client-only: apiBase is a browser-reachable URL, not resolvable from the SSR container.
  const { data: digest, error, pending, refresh } = useFetch<Digest>(`${apiBase}/digest/today`, {
    server: false,
  })

  const running = ref(false)
  const runError = ref<string | null>(null)

  async function runNow() {
    running.value = true
    runError.value = null
    try {
      await $fetch(`${apiBase}/digest/run`, { method: 'POST' })
      await refresh()
    } catch (e: any) {
      runError.value = e?.data?.error ?? e?.message ?? 'Run failed'
    } finally {
      running.value = false
    }
  }

  const noDigestYet = computed(() => error.value?.statusCode === 404)

  const isEmpty = computed(() => !digest.value?.picks.length)

  const bySource = computed(() => {
    const groups: Record<string, Pick[]> = {}
    for (const p of digest.value?.picks ?? []) {
      ;(groups[p.source] ??= []).push(p)
    }
    return groups
  })

  const formattedDate = computed(() => {
    if (!digest.value?.date) return ''
    return new Date(digest.value.date).toLocaleDateString(undefined, {
      weekday: 'long',
      month: 'long',
      day: 'numeric',
    })
  })

  return { digest, error, pending, running, runError, runNow, noDigestYet, isEmpty, bySource, formattedDate }
}
