function errorMessage(e: any, fallback: string): string {
  return e?.data?.error ?? e?.message ?? fallback;
}

/** Digest-side favorites state: which urls are saved, and toggling a pick. */
export function useFavorites() {
  const { apiBase } = useRuntimeConfig().public;

  // Client-only, like useDigest. On failure `urlToId` stays {} so every star renders
  // outlined and starring still works.
  const { data: urlToId } = useFetch<Record<string, number>>(
    `${apiBase}/favorites/urls`,
    {
      key: "favorites-urls",
      server: false,
      default: () => ({}),
    },
  );

  const toggleErrors = ref<Record<string, string>>({});

  function isFavorited(url: string) {
    return url in urlToId.value;
  }

  // Waits for the server before flipping the star, so a failure never needs a revert.
  async function toggle(pick: Pick) {
    delete toggleErrors.value[pick.url];
    try {
      const id = urlToId.value[pick.url];
      if (!id) {
        const saved = await $fetch<Favorite>(`${apiBase}/favorites`, {
          method: "POST",
          body: pick,
        });
        urlToId.value = { ...urlToId.value, [pick.url]: saved.id };
      } else {
        await $fetch(`${apiBase}/favorites/${id}`, { method: "DELETE" });
        const { [pick.url]: _, ...rest } = urlToId.value;
        urlToId.value = rest;
      }
    } catch (e: any) {
      toggleErrors.value[pick.url] = errorMessage(
        e,
        "Could not update favorite",
      );
    }
  }

  return { isFavorited, toggle, toggleErrors };
}

/** Favorites page: paginated list driven by the `?page=` query param. */
export function useFavoritesList() {
  const { apiBase } = useRuntimeConfig().public;
  const route = useRoute();
  const router = useRouter();

  const page = computed(() => {
    const n = Number.parseInt(String(route.query.page ?? ""), 10);
    return Number.isFinite(n) && n >= 1 ? n : 1;
  });

  const { data, pending, error, refresh } = useFetch<FavoritesPage>(
    `${apiBase}/favorites`,
    {
      query: { page },
      server: false,
      watch: [page],
    },
  );

  // The backend clamps out-of-range pages; keep the URL in sync with what it returned.
  watch(data, (d) => {
    if (d && d.page !== page.value) goTo(d.page);
  });

  const isEmpty = computed(
    () => !pending.value && !error.value && data.value?.totalItems === 0,
  );
  const showPagination = computed(
    () => !pending.value && !error.value && !isEmpty.value && !!data.value,
  );

  const removeErrors = ref<Record<string, string>>({});

  function goTo(p: number) {
    router.push({
      query: { ...route.query, page: p === 1 ? undefined : String(p) },
    });
  }

  async function remove(fav: Favorite) {
    if (!data.value) return;
    delete removeErrors.value[fav.url];
    try {
      await $fetch(`${apiBase}/favorites/${fav.id}`, { method: "DELETE" });
    } catch (e: any) {
      removeErrors.value[fav.url] = errorMessage(
        e,
        "Could not remove favorite",
      );
      return;
    }
    data.value = {
      ...data.value,
      items: data.value.items.filter((f) => f.id !== fav.id),
    };
    if (data.value.items.length === 0 && page.value > 1) {
      goTo(page.value - 1); // page change triggers the refetch
    } else {
      await refresh();
    }
  }

  return {
    data,
    pending,
    error,
    isEmpty,
    showPagination,
    page,
    goTo,
    remove,
    removeErrors,
  };
}
