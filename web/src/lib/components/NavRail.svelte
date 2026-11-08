<script lang="ts">
	import { isActivePath, primaryNav, secondaryNav } from '#lib/navigation';
	import NavIcon from './NavIcon.svelte';

	let { activePath = '/' }: { activePath?: string } = $props();

	const linkBase =
		'focus-visible:outline-primary flex items-center gap-2.5 rounded-md border-l-2 px-2.5 py-2 text-sm focus-visible:outline-2 focus-visible:outline-offset-2';
	const linkIdle =
		'border-transparent text-foreground-muted hover:bg-surface-2 hover:text-foreground';
	const linkActive = 'border-primary bg-surface-3 text-primary';
</script>

<nav
	aria-label="Primary"
	class="border-border bg-surface-1 flex flex-col gap-6 border-b px-3 py-3 lg:h-dvh lg:border-r lg:border-b-0"
>
	<a
		href="/"
		class="focus-visible:outline-primary flex w-fit items-center gap-2 rounded-md px-2 py-1.5 text-sm font-semibold tracking-tight focus-visible:outline-2 focus-visible:outline-offset-2"
		aria-label="Observaki overview"
	>
		<span class="border-primary inline-block size-3 rounded-full border-2" aria-hidden="true"
		></span>
		<span>Observaki</span>
	</a>

	<div>
		<p
			class="text-foreground-subtle px-2.5 pb-2 text-[11px] font-semibold tracking-[0.04em] uppercase"
		>
			Investigate
		</p>
		<ul class="space-y-0.5">
			{#each primaryNav as item (item.href)}
				{@const active = isActivePath(activePath, item.href)}
				<li>
					<a
						href={item.href}
						aria-current={active ? 'page' : undefined}
						class="{linkBase} {active ? linkActive : linkIdle}"
					>
						<NavIcon name={item.icon} />
						<span>{item.label}</span>
					</a>
				</li>
			{/each}
		</ul>
	</div>

	<div class="mt-auto">
		<p
			class="text-foreground-subtle px-2.5 pb-2 text-[11px] font-semibold tracking-[0.04em] uppercase"
		>
			Reference
		</p>
		<ul class="space-y-0.5">
			{#each secondaryNav as item (item.href)}
				{@const active = isActivePath(activePath, item.href)}
				<li>
					<a
						href={item.href}
						aria-current={active ? 'page' : undefined}
						class="{linkBase} {active ? linkActive : linkIdle}"
					>
						<NavIcon name={item.icon} />
						<span>{item.label}</span>
					</a>
				</li>
			{/each}
		</ul>
	</div>
</nav>
