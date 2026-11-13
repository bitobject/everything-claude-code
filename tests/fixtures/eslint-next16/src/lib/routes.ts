import type { Route } from 'next'

// The one file that casts to Route.
export function cityHref(city: string): Route {
  return `/${city}` as Route
}
