import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			// Every route is prerendered into web/build and the Go static handler
			// serves index.html as the fallback for unknown paths (OBSERVAKI_WEB_DIR).
			adapter: adapter()
		})
	],
	server: {
		port: Number(process.env.WEB_PORT ?? 5173),
		strictPort: true,
		proxy: {
			'/api': {
				target: process.env.OBSERVAKI_API_URL ?? 'http://localhost:8080',
				changeOrigin: true
			}
		}
	}
});
