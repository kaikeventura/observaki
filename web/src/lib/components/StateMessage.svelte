<script lang="ts">
	import {
		STATE_FRAME_MIN_HEIGHT,
		stateDefaults,
		type StatePillTone,
		type StateVariant
	} from '#lib/states';
	import StatusPill from './StatusPill.svelte';

	let {
		variant = 'empty',
		title,
		body,
		requestId,
		primaryLabel,
		secondaryLabel,
		onprimary,
		onsecondary,
		primaryDisabled = false
	}: {
		variant?: StateVariant;
		title?: string;
		body?: string;
		requestId?: string;
		primaryLabel?: string;
		secondaryLabel?: string;
		onprimary?: () => void;
		onsecondary?: () => void;
		primaryDisabled?: boolean;
	} = $props();

	const defaults = $derived(stateDefaults[variant]);
	const resolvedTitle = $derived(title ?? defaults.title);
	const resolvedBody = $derived(body ?? defaults.body);
	const resolvedPrimary = $derived(primaryLabel ?? defaults.primary);
	const resolvedSecondary = $derived(secondaryLabel ?? defaults.secondary);
	const pill = $derived(defaults.pill);
	const tone: StatePillTone = $derived(defaults.tone ?? 'neutral');
	const isAlert = $derived(variant === 'error' || variant === 'no_permission');
</script>

<div
	class="border-border bg-surface-2 flex flex-col justify-center gap-3 rounded-lg border p-4"
	style:min-height={STATE_FRAME_MIN_HEIGHT}
	data-state-frame={variant}
	data-state-height={STATE_FRAME_MIN_HEIGHT}
	role={isAlert ? 'alert' : 'status'}
	aria-live={isAlert ? 'assertive' : 'polite'}
>
	{#if variant === 'loading'}
		<p class="sr-only">{resolvedTitle}. {resolvedBody}</p>
		<div class="space-y-2" aria-hidden="true">
			<div class="bg-surface-3 h-3 w-1/3 animate-pulse rounded-sm"></div>
			<div class="bg-surface-3 h-8 w-2/3 animate-pulse rounded-sm"></div>
			<div class="bg-surface-3 h-3 w-full animate-pulse rounded-sm"></div>
			<div class="bg-surface-3 h-3 w-5/6 animate-pulse rounded-sm"></div>
		</div>
	{:else}
		{#if pill}
			<StatusPill label={pill} {tone} />
		{/if}
		<p class="text-sm font-semibold">{resolvedTitle}</p>
		<p class="text-foreground-muted max-w-[60ch] text-xs">{resolvedBody}</p>
		{#if requestId}
			<p class="text-foreground-subtle font-mono text-[11px]">Request ID {requestId}</p>
		{/if}
	{/if}
	{#if resolvedPrimary || resolvedSecondary}
		<div class="flex flex-wrap gap-2 pt-1">
			{#if resolvedPrimary && onprimary}
				<button
					type="button"
					class="bg-primary text-primary-foreground hover:bg-primary-hover focus-visible:outline-primary rounded-md px-2.5 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
					onclick={onprimary}
					disabled={primaryDisabled}
				>
					{resolvedPrimary}
				</button>
			{/if}
			{#if resolvedSecondary && onsecondary}
				<button
					type="button"
					class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2.5 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2"
					onclick={onsecondary}
				>
					{resolvedSecondary}
				</button>
			{/if}
		</div>
	{/if}
</div>
