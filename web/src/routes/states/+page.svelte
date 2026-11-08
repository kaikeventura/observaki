<script lang="ts">
	import StateMessage from '#lib/components/StateMessage.svelte';
	import { stateVariants, type StateVariant } from '#lib/states';

	const labels: Record<StateVariant, string> = {
		loading: 'Loading',
		empty: 'Empty',
		error: 'Error',
		timeout: 'Timeout',
		partial: 'Partial data',
		rate_limited: 'Rate limited',
		no_permission: 'No permission'
	};

	let selected = $state<StateVariant>('empty');
	let notice = $state('');

	function announce(action: string) {
		notice = `${action} for the “${labels[selected]}” state is handled by the shell in Phase 0; real data actions arrive with the Query Service.`;
	}
</script>

<svelte:head>
	<title>State gallery · Observaki</title>
	<meta
		name="description"
		content="Every Observaki UI state with its copy and next action, kept stable so the layout does not shift."
	/>
</svelte:head>

<div class="mx-auto max-w-6xl space-y-6 px-6 py-8">
	<header class="space-y-1">
		<h1 class="text-2xl font-semibold tracking-tight">State gallery</h1>
		<p class="text-foreground-muted max-w-[72ch] text-sm">
			Every state keeps one sentence of what happened plus a next action. The frame reserves its
			height, so switching states never moves the page.
		</p>
	</header>

	<section aria-labelledby="trigger-heading" class="space-y-4">
		<h2 id="trigger-heading" class="text-foreground-muted text-sm font-medium">Trigger a state</h2>

		<div role="group" aria-label="UI states" class="flex flex-wrap gap-2">
			{#each stateVariants as variant (variant)}
				<button
					type="button"
					aria-pressed={selected === variant}
					class="focus-visible:outline-primary rounded-md border px-2.5 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2 {selected ===
					variant
						? 'border-primary bg-surface-3 text-primary'
						: 'border-border-strong bg-surface-1 text-foreground-muted hover:bg-surface-3 hover:text-foreground'}"
					onclick={() => {
						selected = variant;
						notice = '';
					}}
				>
					{labels[variant]}
				</button>
			{/each}
		</div>

		<div class="border-border bg-surface-1 space-y-3 rounded-lg border p-4">
			<StateMessage
				variant={selected}
				requestId={selected === 'error' ? 'req_01J8Z3K4' : undefined}
				primaryDisabled={selected === 'rate_limited'}
				onprimary={() => announce('The primary action')}
				onsecondary={() => announce('The secondary action')}
			/>
			<p class="text-foreground-muted min-h-5 text-xs" role="status" aria-live="polite">
				{notice}
			</p>
		</div>
	</section>
</div>
