import type { APIContext } from 'astro';
import { getCollection } from 'astro:content';
import { llmsTxt } from '../lib/llms.mjs';

export async function GET({ site }: APIContext) {
  const docs = await getCollection('docs');
  const entries = docs.map((d) => ({ id: d.id, title: d.data.title, description: d.data.description ?? '', body: d.body ?? '' }));
  return new Response(llmsTxt(entries, site!.origin), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
}
