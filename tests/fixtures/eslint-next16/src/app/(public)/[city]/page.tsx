import Image from 'next/image'

// Route segment config that Cache Components removed; ensureStatic is the option we use.
export const revalidate = 3600 // expect: route segment config
export const dynamic = 'force-static' // expect: route segment config
export const dynamicParams = false // expect: route segment config
export const fetchCache = 'force-cache' // expect: route segment config
export const ensureStatic = 'navigation'

export async function generateStaticParams() {
  return [] // expect: at least one param
}

export default async function Page({ params }: { params: Promise<{ city: string }> }) {
  const { city } = await params
  return (
    <main>
      <h1>{city}</h1>
      <Image src="/a.webp" alt="" width={8} height={8} priority /> {/* expect: priority is deprecated */}
      <Image src="/b.webp" alt="" width={8} height={8} fetchPriority="high" /> {/* expect: alone still renders */}
      <Image src="/c.webp" alt="" width={8} height={8} loading="eager" fetchPriority="high" />
      <Image src="/d.webp" alt="" width={8} height={8} preload loading="eager" /> {/* expect: preload is not combined */}
      <Image src="/g.webp" alt="" width={8} height={8} preload fetchPriority="high" /> {/* expect: preload is not combined */}
      <Image src="/e.webp" alt="" width={8} height={8} preload />
      <Image src="/f.webp" alt="" width={8} height={8} fetchPriority="low" />
    </main>
  )
}
