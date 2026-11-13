// Data functions: the Next.js 16 rules that apply to every file under src/.
import { unstable_cache } from 'next/cache' // expect: Cache Components replace it
import { updateTag } from 'next/cache' // expect: only in a Server Action
import { cacheLife, cacheTag, revalidateTag } from 'next/cache'
import { cookies, headers } from 'next/headers'

export const legacy = unstable_cache(async () => 1, ['legacy'])

export async function getCity(slug: string) {
  'use cache'
  cacheLife('days')
  cacheTag(`city:${slug}`)
  const jar = await cookies() // expect: inside 'use cache'
  return { slug, theme: jar.get('theme')?.value }
}

export const getTopic = async (slug: string) => {
  'use cache: remote'
  const sent = await headers() // expect: inside 'use cache'
  return { slug, agent: sent.get('user-agent') }
}

// 'use cache: private' may read the request (Next.js docs), and so may code outside a cache.
export async function getPrivateCity(slug: string) {
  'use cache: private'
  const jar = await cookies()
  return { slug, theme: jar.get('theme')?.value }
}

export async function sessionCookie() {
  const jar = await cookies()
  return jar.get('__Host-session')?.value
}

// A cached function declared inside: only its own body is a cache scope.
export async function withInnerCache(slug: string) {
  const jar = await cookies()
  const cached = async () => {
    'use cache'
    return slug
  }
  return { theme: jar.get('theme')?.value, slug: await cached() }
}

export async function forwardEverything() {
  return { cookie: (await cookies()).toString() } // expect: sessionHeaders
}

export function forwardThePromise() {
  return { cookie: cookies().toString() } // expect: sessionHeaders
}

export async function oldFetchCache() {
  const res = await fetch('https://api.example.test/v1/cities', { next: { revalidate: 60 } }) // expect: next.revalidate
  return res.ok
}

export function refresh(tag: string) {
  revalidateTag(tag, 'max')
  revalidateTag(tag, { expire: 0 })
  updateTag(tag)
}
