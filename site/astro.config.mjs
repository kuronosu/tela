import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';
import { syncWiki, wikiWatcher } from './scripts/sync-wiki.mjs';

// Sync docs/wiki into src/content/docs/docs before Astro reads the content.
const sidebar = syncWiki();

export default defineConfig({
	site: 'https://telaui.dev',
	vite: {
		server: {
			fs: {
				// The landing page imports files from outside site/: the screenshots
				// of ../docs/images and the source of ../examples/counter (?raw). Paths
				// are relative to site/, which stays allowed through '.'.
				allow: ['.', '../docs/images', '../examples/counter'],
			},
		},
	},
	integrations: [
		wikiWatcher(),
		starlight({
			title: 'Tela',
			// The header shows the large mark (one file for light and dark), redrawn
			// on whole pixels for 32 and 40 px: see src/components/SiteTitle.astro.
			components: {
				SiteTitle: './src/components/SiteTitle.astro',
				Hero: './src/components/Hero.astro',
			},
			favicon: '/favicon.svg',
			head: [
				{ tag: 'link', attrs: { rel: 'icon', href: '/favicon.ico', sizes: '16x16 32x32 48x48' } },
				{ tag: 'link', attrs: { rel: 'apple-touch-icon', href: '/apple-touch-icon.png' } },
				{ tag: 'meta', attrs: { property: 'og:image', content: 'https://telaui.dev/og.png' } },
				{ tag: 'meta', attrs: { property: 'og:image:width', content: '1280' } },
				{ tag: 'meta', attrs: { property: 'og:image:height', content: '640' } },
				{ tag: 'meta', attrs: { name: 'twitter:card', content: 'summary_large_image' } },
				{ tag: 'meta', attrs: { name: 'twitter:image', content: 'https://telaui.dev/og.png' } },
			],
			description:
				'A desktop UI framework in pure Go. No cgo, no webview, no native toolkits. Tailwind classes, React-style hooks.',
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/kuronosu/tela' }],
			sidebar,
			customCss: ['@fontsource-variable/inter', './src/styles/custom.css'],
			plugins: [starlightLinksValidator()],
		}),
	],
});
