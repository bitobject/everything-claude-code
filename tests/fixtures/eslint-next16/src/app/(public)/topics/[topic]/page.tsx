// An empty array that is filled before it is returned is not an empty result.
export async function generateStaticParams() {
  const params: { topic: string }[] = []
  params.push({ topic: 'divorce' })
  return params
}

export default async function Page({ params }: { params: Promise<{ topic: string }> }) {
  const { topic } = await params
  return <h1>{topic}</h1>
}
