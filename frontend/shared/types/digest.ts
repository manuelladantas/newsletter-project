export interface Pick {
  source: string
  title: string
  url: string
  reason: string
}

export interface Digest {
  date: string
  picks: Pick[]
}
