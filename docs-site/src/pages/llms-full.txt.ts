import { getCollection } from 'astro:content';
import { llmsFull } from '../lib/llms.mjs';

export async function GET() {
  const docs = await getCollection('docs');
  const entries = docs.map((d) => ({ id: d.id, title: d.data.title, description: d.data.description ?? '', body: d.body ?? '' }));
  return new Response(llmsFull(entries), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
}
