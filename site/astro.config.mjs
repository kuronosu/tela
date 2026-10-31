import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';
import { syncWiki, wikiWatcher } from './scripts/sync-wiki.mjs';

// Sync docs/wiki into src/content/docs/docs before Astro reads the content.
const sidebar = syncWiki();

export default defineConfig({
	site: 'https://telaui.dev',
	integrations: [
		wikiWatcher(),
		starlight({
			title: 'Tela',
			description:
				'A desktop UI framework in pure Go. No cgo, no webview, no native toolkits. Tailwind classes, React-style hooks.',
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/kuronosu/tela' }],
			sidebar,
			customCss: ['@fontsource-variable/inter', './src/styles/custom.css'],
			plugins: [starlightLinksValidator()],
		}),
	],
});
