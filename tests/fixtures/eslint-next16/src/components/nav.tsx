import type { Route } from 'next'
import Link from 'next/link'

export function Nav({ city }: { city: string }) {
  return <Link href={`/${city}` as Route}>{city}</Link> // expect: @/lib/routes
}
