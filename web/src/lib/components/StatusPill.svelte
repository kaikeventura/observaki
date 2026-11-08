<script lang="ts">
	export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger';

	let {
		label,
		tone = 'neutral',
		marker,
		class: className = ''
	}: {
		label: string;
		tone?: StatusTone;
		marker?: string;
		class?: string;
	} = $props();

	const toneClass: Record<StatusTone, string> = {
		neutral: 'border-border text-foreground-muted',
		info: 'border-info/40 bg-info/10 text-info',
		success: 'border-success/40 bg-success/10 text-success',
		warning: 'border-warning/40 bg-warning/10 text-warning',
		danger: 'border-danger/40 bg-danger/10 text-danger'
	};

	const toneMarker: Record<StatusTone, string> = {
		neutral: '○',
		info: 'i',
		success: '●',
		warning: '▲',
		danger: '■'
	};
</script>

<span
	class="inline-flex w-fit items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-[11px] leading-none font-medium {toneClass[
		tone
	]} {className}"
>
	<span aria-hidden="true">{marker ?? toneMarker[tone]}</span>
	<span>{label}</span>
</span>
