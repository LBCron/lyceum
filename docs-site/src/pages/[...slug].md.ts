import { getCollection } from 'astro:content';
import { pageMarkdown } from '../lib/llms.mjs';

export async function getStaticPaths() {
  const docs = await getCollection('docs');
  return docs.map((d) => ({
    params: { slug: d.id },
    props: { entry: { id: d.id, title: d.data.title, description: d.data.description ?? '', body: d.body ?? '' } },
  }));
}

export function GET({ props }: { props: { entry: Parameters<typeof pageMarkdown>[0] } }) {
  return new Response(pageMarkdown(props.entry), { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } });
}
