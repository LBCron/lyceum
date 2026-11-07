// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  output: 'static',
  integrations: [
    starlight({
      title: 'Lyceum',
      description: 'Spec-driven development with a coding agent, gated by approvals and recorded test runs.',
      favicon: '/favicon.svg',
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/oritromax/lyceum' }],
      editLink: {
        baseUrl: 'https://github.com/oritromax/lyceum/edit/main/docs-site/',
      },
      sidebar: [
        {
          label: 'Start',
          items: ['start/what-is-lyceum', 'start/install', 'start/quickstart'],
        },
        {
          label: 'Concepts',
          items: ['concepts/units-and-gates', 'concepts/approvals', 'concepts/acceptance-criteria'],
        },
        {
          label: 'Guides',
          items: ['guides/agents', 'guides/ci', 'guides/existing-projects'],
        },
        {
          label: 'Reference',
          items: ['reference/commands', 'reference/config'],
        },
      ],
    }),
  ],
});
