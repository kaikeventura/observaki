<script lang="ts">
	import { SIGNALS, type Signal } from '#lib/explore/filters';

	/**
	 * Signal tabs (Story 6.4, AC3). A real tablist: roving tabindex, arrow/Home/
	 * End navigation and a 2px underline on the active tab (DESIGN.md).
	 */
	let {
		signal,
		panelId,
		ontab
	}: { signal: Signal; panelId: string; ontab: (next: Signal) => void } = $props();

	const uid = $props.id();
	let buttons: HTMLButtonElement[] = [];

	function select(next: Signal, focus = false): void {
		ontab(next);
		if (focus) {
			const index = SIGNALS.findIndex((entry) => entry.id === next);
			buttons[index]?.focus();
		}
	}

	function onkeydown(event: KeyboardEvent, index: number): void {
		let next: number | undefined;
		if (event.key === 'ArrowRight') next = (index + 1) % SIGNALS.length;
		else if (event.key === 'ArrowLeft') next = (index - 1 + SIGNALS.length) % SIGNALS.length;
		else if (event.key === 'Home') next = 0;
		else if (event.key === 'End') next = SIGNALS.length - 1;
		if (next === undefined) return;
		event.preventDefault();
		select(SIGNALS[next].id, true);
	}
</script>

<div role="tablist" aria-label="Signal" class="border-border flex gap-1 border-b">
	{#each SIGNALS as entry, index (entry.id)}
		<button
			bind:this={buttons[index]}
			type="button"
			role="tab"
			id={`${uid}-tab-${entry.id}`}
			aria-selected={signal === entry.id}
			aria-controls={panelId}
			tabindex={signal === entry.id ? 0 : -1}
			data-signal-tab={entry.id}
			class="-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors {signal === entry.id
				? 'border-primary text-foreground'
				: 'text-foreground-muted hover:text-foreground border-transparent'}"
			onclick={() => select(entry.id)}
			onkeydown={(event) => onkeydown(event, index)}
		>
			{entry.label}
		</button>
	{/each}
</div>
