import { NextResponse, type NextRequest } from 'next/server'

export function proxy(request: NextRequest) {
  return NextResponse.next({ request })
}

export function middleware(request: NextRequest) { // expect: export function proxy
  return NextResponse.next({ request })
}

// The shared bans still apply in this file.
export async function warm() {
  await fetch('https://api.example.test/v1/health', { next: { revalidate: 60 } }) // expect: next.revalidate
}

export const config = { matcher: ['/account/:path*', '/cabinet/:path*', '/admin/:path*'] }
