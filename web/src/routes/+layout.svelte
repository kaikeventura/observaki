<script lang="ts">
	import '../app.css';
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import AppShell from '#lib/components/AppShell.svelte';

	let { children } = $props();

	const activePath = $derived(page.url.pathname);

	afterNavigate((navigation) => {
		// SPA route changes move focus to the content so screen readers announce
		// the new surface instead of leaving focus on a nav link.
		if (navigation.type === 'enter') return;
		document.getElementById('main-content')?.focus();
	});
</script>

<AppShell {activePath}>
	{@render children()}
</AppShell>
