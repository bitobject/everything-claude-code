'use server'

// A Server Action module may import updateTag; the other bans still apply here.
import { updateTag } from 'next/cache'
import { unstable_cache } from 'next/cache' // expect: Cache Components replace it

export async function touchLead(tag: string) {
  updateTag(tag)
  return unstable_cache(async () => tag, [tag])()
}
