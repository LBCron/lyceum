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
    }),
  ],
});
