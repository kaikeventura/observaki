<script lang="ts">
	import { page } from '$app/state';
</script>

<svelte:head>
	<title>Not found · Observaki</title>
</svelte:head>

<div class="mx-auto max-w-2xl space-y-4 px-6 py-12">
	<p class="text-foreground-muted font-mono text-xs">{page.status}</p>
	<h1 class="text-2xl font-semibold tracking-tight">
		{page.status === 404 ? 'This page does not exist.' : 'Something went wrong on this page.'}
	</h1>
	<p class="text-foreground-muted max-w-[60ch] text-sm">
		{page.error?.message ?? 'The requested surface is not available in this build.'}
	</p>
	<div class="flex flex-wrap gap-2">
		<a
			href="/"
			class="bg-primary text-primary-foreground hover:bg-primary-hover focus-visible:outline-primary rounded-md px-3 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2"
			>Back to overview</a
		>
		<a
			href="/health"
			class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-3 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2"
			>Open system health</a
		>
	</div>
</div>
