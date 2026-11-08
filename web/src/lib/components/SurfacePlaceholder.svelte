<script lang="ts">
	import { activeProject } from '#lib/projects';
	import type { StateVariant } from '#lib/states';
	import StateMessage from './StateMessage.svelte';

	let {
		title,
		description,
		stateTitle,
		stateBody,
		primaryLabel,
		primaryOutcome,
		secondaryLabel,
		secondaryOutcome,
		variant = 'empty'
	}: {
		title: string;
		description: string;
		stateTitle: string;
		stateBody: string;
		primaryLabel: string;
		primaryOutcome: string;
		secondaryLabel: string;
		secondaryOutcome: string;
		variant?: StateVariant;
	} = $props();

	const project = $derived($activeProject);
	let notice = $state('');

	function act(outcome: string) {
		notice = outcome;
	}
</script>

<div class="mx-auto max-w-6xl space-y-6 px-6 py-8">
	<header class="space-y-1">
		<p class="text-foreground-muted font-mono text-[11px]">
			{project.organization} / {project.name} · {project.environment}
		</p>
		<h1 class="text-2xl font-semibold tracking-tight">{title}</h1>
		<p class="text-foreground-muted max-w-[72ch] text-sm">{description}</p>
	</header>

	<section
		aria-label="{title} results"
		class="border-border bg-surface-1 space-y-3 rounded-lg border p-4"
	>
		<StateMessage
			{variant}
			title={stateTitle}
			body={stateBody}
			{primaryLabel}
			{secondaryLabel}
			onprimary={() => act(primaryOutcome)}
			onsecondary={() => act(secondaryOutcome)}
		/>
		<p class="text-foreground-muted min-h-5 text-xs" role="status" aria-live="polite">{notice}</p>
	</section>
</div>
