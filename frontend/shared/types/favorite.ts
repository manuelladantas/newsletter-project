export interface Favorite extends Pick {
  id: number
}

export interface FavoritesPage {
  items: Favorite[]
  page: number
  pageSize: number
  totalItems: number
  totalPages: number
}
