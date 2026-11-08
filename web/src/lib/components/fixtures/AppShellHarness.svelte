<script lang="ts">
	import AppShell from '../AppShell.svelte';

	let { activePath = '/' }: { activePath?: string } = $props();
</script>

<AppShell {activePath}>
	<p>Shell content</p>
</AppShell>
