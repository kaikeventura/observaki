<script lang="ts">
	import {
		applySuggestion,
		suggestionsFor,
		signalLabel,
		type Signal,
		type Suggestion
	} from '#lib/explore/filters';

	/**
	 * Query bar with mock autocomplete (Story 6.4, AC3). The combobox pattern is
	 * explicit (`role="combobox"` + `aria-activedescendant`) so the keyboard
	 * surface works with a screen reader, not just with the mouse.
	 */
	let {
		value = $bindable(''),
		signal,
		onrun
	}: {
		value?: string;
		signal: Signal;
		onrun?: (query: string) => void;
	} = $props();

	const uid = $props.id();
	const inputId = `${uid}-query`;
	const listId = `${uid}-suggestions`;
	const helpId = `${uid}-help`;
	const statusId = `${uid}-status`;

	let open = $state(false);
	let active = $state(-1);
	let input = $state<HTMLInputElement>();

	const suggestions = $derived<Suggestion[]>(open ? suggestionsFor(signal, value) : []);
	const activeId = $derived(active >= 0 ? `${uid}-option-${active}` : undefined);
	const signalName = $derived(signalLabel(signal));

	function show(): void {
		open = true;
		active = -1;
	}

	function close(): void {
		open = false;
		active = -1;
	}

	function choose(suggestion: Suggestion): void {
		value = applySuggestion(value, suggestion);
		active = -1;
		open = true;
		input?.focus();
	}

	function run(): void {
		close();
		onrun?.(value);
	}

	function onkeydown(event: KeyboardEvent): void {
		if (event.metaKey || event.ctrlKey) {
			if (event.key === 'Enter') {
				event.preventDefault();
				run();
			}
			return;
		}

		switch (event.key) {
			case 'ArrowDown':
				event.preventDefault();
				show();
				if (suggestions.length > 0) {
					active = (active + 1) % suggestions.length;
				}
				break;
			case 'ArrowUp':
				event.preventDefault();
				show();
				if (suggestions.length > 0) {
					active = active <= 0 ? suggestions.length - 1 : active - 1;
				}
				break;
			case 'Enter':
				if (open && active >= 0 && suggestions[active]) {
					event.preventDefault();
					choose(suggestions[active]);
				} else {
					event.preventDefault();
					run();
				}
				break;
			case 'Escape':
				if (open) {
					event.preventDefault();
					close();
				}
				break;
			case 'Tab':
				if (open && active >= 0 && suggestions[active]) choose(suggestions[active]);
				break;
		}
	}
</script>

<div class="space-y-2">
	<label for={inputId} class="text-foreground-muted text-xs font-medium">Query</label>
	<div class="flex flex-wrap items-stretch gap-2">
		<div class="relative min-w-0 flex-1">
			<input
				bind:this={input}
				id={inputId}
				data-query-input
				type="text"
				role="combobox"
				aria-autocomplete="list"
				aria-expanded={open && suggestions.length > 0}
				aria-controls={listId}
				aria-activedescendant={activeId}
				aria-describedby={`${helpId} ${statusId}`}
				autocomplete="off"
				spellcheck="false"
				placeholder={`Search ${signalName.toLowerCase()}… e.g. service:checkout`}
				class="border-border-strong bg-surface-2 text-foreground placeholder:text-foreground-subtle focus-visible:outline-primary w-full rounded-md border px-3 py-2 font-mono text-sm focus-visible:outline-2 focus-visible:outline-offset-2"
				bind:value
				onfocus={show}
				oninput={show}
				{onkeydown}
				onblur={close}
			/>

			{#if open && suggestions.length > 0}
				<ul
					id={listId}
					role="listbox"
					aria-label={`${signalName} suggestions`}
					class="border-border bg-surface-2 absolute z-10 mt-1 max-h-72 w-full overflow-auto rounded-md border shadow-lg"
				>
					{#each suggestions as suggestion, index (suggestion.value)}
						<li
							id={`${uid}-option-${index}`}
							role="option"
							aria-selected={index === active}
							class="flex cursor-pointer items-center justify-between gap-3 px-3 py-2 text-xs {index ===
							active
								? 'bg-surface-3 text-foreground'
								: 'text-foreground-muted hover:bg-surface-3 hover:text-foreground'}"
							onmousedown={(event) => {
								// Keep focus in the input so the combobox stays open until choose().
								event.preventDefault();
								choose(suggestion);
							}}
						>
							<span class="font-mono">{suggestion.value}</span>
							<span class="text-foreground-subtle">{suggestion.kind} · {suggestion.detail}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>

		<button
			type="button"
			class="bg-primary text-primary-foreground hover:bg-primary-hover focus-visible:outline-primary rounded-md px-3 py-2 text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2"
			onclick={run}
		>
			Run query <span class="font-mono text-[11px] opacity-70">⌘↵</span>
		</button>
	</div>

	<p id={helpId} class="text-foreground-subtle text-xs">
		Autocomplete uses mock {signalName.toLowerCase()} data: fields, values and metric names.
		<kbd class="border-border rounded border px-1 font-mono text-[10px]">↑</kbd>/<kbd
			class="border-border rounded border px-1 font-mono text-[10px]">↓</kbd
		>
		navigate, <kbd class="border-border rounded border px-1 font-mono text-[10px]">Enter</kbd>
		accepts,
		<kbd class="border-border rounded border px-1 font-mono text-[10px]">⌘Enter</kbd> runs.
	</p>

	<p id={statusId} class="sr-only" aria-live="polite">
		{#if open && suggestions.length > 0}
			{suggestions.length} suggestions available for {signalName}.
		{:else if open}
			No suggestions for {signalName}.
		{/if}
	</p>
</div>
