/**
 * uPlot draws on a canvas, so it cannot use Tailwind classes. The chart reads
 * the same CSS custom properties the rest of the UI uses (DESIGN.md tokens)
 * instead of hardcoding colors.
 */

export type ChartTheme = {
	primary: string;
	grid: string;
	axis: string;
	surface: string;
	font: string;
};

const FALLBACK: ChartTheme = {
	primary: '#22d3ee',
	grid: '#2e3b4d',
	axis: '#93a1b5',
	surface: '#0e131b',
	font: "ui-monospace, 'SFMono-Regular', Menlo, monospace"
};

/** Read the design tokens from the document root, with safe fallbacks for SSR. */
export function readChartTheme(): ChartTheme {
	if (typeof document === 'undefined') return FALLBACK;
	const styles = getComputedStyle(document.documentElement);
	const token = (name: string, fallback: string) =>
		styles.getPropertyValue(name).trim() || fallback;
	return {
		primary: token('--color-primary', FALLBACK.primary),
		grid: token('--color-border', FALLBACK.grid),
		axis: token('--color-foreground-muted', FALLBACK.axis),
		surface: token('--color-surface-1', FALLBACK.surface),
		font: token('--font-mono', FALLBACK.font)
	};
}

/**
 * Turn a hex token into an rgba() color. Non-hex tokens (oklch, rgb) are
 * returned unchanged so a future palette change does not break the chart.
 */
export function withAlpha(color: string, alpha: number): string {
	const match = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(color.trim());
	if (!match) return color;
	let hex = match[1];
	if (hex.length === 3) {
		hex = hex
			.split('')
			.map((char) => char + char)
			.join('');
	}
	const red = Number.parseInt(hex.slice(0, 2), 16);
	const green = Number.parseInt(hex.slice(2, 4), 16);
	const blue = Number.parseInt(hex.slice(4, 6), 16);
	const clamped = Math.min(1, Math.max(0, alpha));
	return `rgba(${red}, ${green}, ${blue}, ${clamped})`;
}
