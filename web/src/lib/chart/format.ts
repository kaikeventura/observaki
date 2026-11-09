/**
 * Formatting helpers shared by the chart axis, tooltip and accessible table.
 * Times are formatted in the browser's timezone; the panel header names it.
 */

const numberFormatters = new Map<string, Intl.NumberFormat>();
const timeFormatters = new Map<string, Intl.DateTimeFormat>();

function numbers(key: string, options: Intl.NumberFormatOptions): Intl.NumberFormat {
	let formatter = numberFormatters.get(key);
	if (!formatter) {
		formatter = new Intl.NumberFormat('en-US', options);
		numberFormatters.set(key, formatter);
	}
	return formatter;
}

function times(key: string, options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
	let formatter = timeFormatters.get(key);
	if (!formatter) {
		formatter = new Intl.DateTimeFormat('en-GB', { ...options, hour12: false });
		timeFormatters.set(key, formatter);
	}
	return formatter;
}

export function timezoneName(): string {
	return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

function pad(value: number): string {
	return String(value).padStart(2, '0');
}

/** Full timestamp for the tooltip: `2026-11-08 14:03:21` in local time. */
export function formatTimestamp(seconds: number): string {
	const date = new Date(seconds * 1000);
	const day = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
	const time = `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
	return `${day} ${time}`;
}

/** Axis tick label; precision follows the visible span. */
export function formatAxisTime(seconds: number, spanSeconds: number): string {
	const date = new Date(seconds * 1000);
	if (spanSeconds <= 600) {
		return times('axis-seconds', {
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit'
		}).format(date);
	}
	if (spanSeconds <= 3 * 86_400) {
		return times('axis-minutes', { hour: '2-digit', minute: '2-digit' }).format(date);
	}
	return `${MONTHS[date.getMonth()]} ${pad(date.getDate())}`;
}

export function formatValue(value: number, unit?: string): string {
	const formatted = numbers('value', {
		maximumFractionDigits: 2,
		minimumFractionDigits: 0
	}).format(value);
	return unit ? `${formatted} ${unit}` : formatted;
}

/** Compact value for the Y axis: 1.2k, 18.4k, 1.1M. */
export function formatAxisValue(value: number): string {
	return numbers('axis', {
		notation: 'compact',
		maximumFractionDigits: 1
	}).format(value);
}

export function formatDuration(seconds: number): string {
	if (seconds < 120) return `${Math.round(seconds)} s`;
	if (seconds < 3600) return `${Math.round(seconds / 60)} min`;
	if (seconds < 48 * 3600) return `${Math.round(seconds / 3600)} h`;
	return `${Math.round(seconds / 86_400)} d`;
}

/** `2026-11-08 12:00:00 → 14:00:00 (2 h)` for the live region and table. */
export function formatRangeLabel(range: [number, number]): string {
	const span = Math.max(0, range[1] - range[0]);
	return `${formatTimestamp(range[0])} → ${formatTimestamp(range[1])} (${formatDuration(span)})`;
}
