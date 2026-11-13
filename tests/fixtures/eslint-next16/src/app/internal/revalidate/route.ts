import { revalidateTag } from 'next/cache'

// Route handlers take the same route segment config as pages and layouts.
export const dynamic = 'force-dynamic' // expect: route segment config

export async function POST(request: Request) {
  const { tag } = (await request.json()) as { tag: string }
  revalidateTag(tag, 'max')
  return new Response(null, { status: 204 })
}
