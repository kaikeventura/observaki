<script lang="ts">
	import type { NavIconName } from '#lib/navigation';

	let {
		name,
		class: className = 'size-4'
	}: {
		name: NavIconName;
		class?: string;
	} = $props();
</script>

<svg
	class={className}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.75"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
	focusable="false"
>
	{#if name === 'explore'}
		<circle cx="11" cy="11" r="6.5" />
		<path d="m16 16 4 4" />
	{:else if name === 'dashboards'}
		<rect x="4" y="4" width="7" height="7" rx="1.5" />
		<rect x="13" y="4" width="7" height="4" rx="1.5" />
		<rect x="13" y="10" width="7" height="10" rx="1.5" />
		<rect x="4" y="13" width="7" height="7" rx="1.5" />
	{:else if name === 'alerts'}
		<path d="M6.5 9.5a5.5 5.5 0 0 1 11 0c0 4 1.5 5.5 1.5 5.5h-14s1.5-1.5 1.5-5.5Z" />
		<path d="M10 18a2 2 0 0 0 4 0" />
	{:else if name === 'settings'}
		<path d="M4 7h8" />
		<circle cx="15" cy="7" r="2.5" />
		<path d="M20 7h-2.5M4 17h2.5" />
		<circle cx="9" cy="17" r="2.5" />
		<path d="M14 17h6" />
	{:else if name === 'states'}
		<rect x="4" y="4" width="16" height="16" rx="2" />
		<path d="M4 10h16M10 10v10" />
	{:else if name === 'health'}
		<path d="M3 12h4l2-5 3 10 2.5-5H21" />
	{/if}
</svg>
