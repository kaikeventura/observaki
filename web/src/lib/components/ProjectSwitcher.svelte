<script lang="ts">
	import { onMount } from 'svelte';
	import { activeProject, projects, type Project } from '#lib/projects';
	import StatusPill from './StatusPill.svelte';

	let open = $state(false);
	let announcement = $state('');
	let wrapper = $state<HTMLDivElement | null>(null);
	let trigger = $state<HTMLButtonElement | null>(null);

	const project = $derived($activeProject);

	onMount(() => {
		// Restore after hydration so the prerendered text and the first client
		// render always match.
		activeProject.restore();
	});

	function toggle() {
		open = !open;
	}

	function select(next: Project) {
		activeProject.select(next.id);
		open = false;
		announcement = `Switched to ${next.organization} ${next.name}.`;
		trigger?.focus();
	}

	function onkeydown(event: KeyboardEvent) {
		if (event.key !== 'Escape' || !open) return;
		event.preventDefault();
		open = false;
		trigger?.focus();
	}

	function onfocusout(event: FocusEvent) {
		if (!open || !wrapper) return;
		const next = event.relatedTarget;
		if (next instanceof Node && wrapper.contains(next)) return;
		open = false;
	}

	$effect(() => {
		if (!open) return;
		const ondocumentclick = (event: MouseEvent) => {
			if (!wrapper) return;
			if (event.target instanceof Node && wrapper.contains(event.target)) return;
			open = false;
		};
		document.addEventListener('click', ondocumentclick);
		return () => document.removeEventListener('click', ondocumentclick);
	});
</script>

<svelte:window {onkeydown} />

<div class="relative" bind:this={wrapper}>
	<button
		type="button"
		bind:this={trigger}
		{onfocusout}
		class="border-border-strong bg-surface-2 hover:bg-surface-3 focus-visible:outline-primary flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-left text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
		aria-expanded={open}
		aria-controls="project-switcher-menu"
		aria-haspopup="true"
		onclick={toggle}
	>
		<span class="text-foreground-muted">Project</span>
		<span class="text-foreground font-medium">{project.organization} / {project.name}</span>
		<StatusPill label={project.environment} />
		<svg
			class="text-foreground-muted size-3 transition-transform {open ? 'rotate-180' : ''}"
			viewBox="0 0 12 12"
			fill="none"
			stroke="currentColor"
			stroke-width="1.5"
			aria-hidden="true"
		>
			<path d="m2.5 4.5 3.5 3.5 3.5-3.5" />
		</svg>
	</button>

	{#if open}
		<div
			id="project-switcher-menu"
			{onfocusout}
			class="border-border-strong bg-surface-2 absolute top-full left-0 z-30 mt-2 w-72 rounded-lg border p-1 shadow-lg"
		>
			<p
				class="text-foreground-subtle px-2 pt-1 pb-2 text-[11px] font-semibold tracking-[0.04em] uppercase"
			>
				Switch project
			</p>
			<ul>
				{#each projects as candidate (candidate.id)}
					{@const current = candidate.id === project.id}
					<li>
						<button
							type="button"
							class="focus-visible:outline-primary flex w-full items-center justify-between gap-3 rounded-md px-2 py-2 text-left text-xs focus-visible:outline-2 focus-visible:outline-offset-2 {current
								? 'bg-surface-3 text-primary'
								: 'text-foreground-muted hover:bg-surface-3 hover:text-foreground'}"
							aria-pressed={current}
							aria-label="Switch to {candidate.organization} {candidate.name} ({candidate.environment})"
							onclick={() => select(candidate)}
						>
							<span class="flex flex-col gap-0.5">
								<span class="font-medium">{candidate.organization} / {candidate.name}</span>
								<span class="text-foreground-subtle font-mono text-[11px]">{candidate.id}</span>
							</span>
							{#if current}
								<span class="text-primary font-mono text-[11px]">Current</span>
							{/if}
						</button>
					</li>
				{/each}
			</ul>
			<p class="text-foreground-subtle border-border mt-1 border-t px-2 py-1.5 text-[11px]">
				Mock projects for the Phase 0 shell.
			</p>
		</div>
	{/if}

	<p class="sr-only" role="status" aria-live="polite">{announcement}</p>
</div>
