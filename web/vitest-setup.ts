import '@testing-library/jest-dom/vitest';

// jsdom has no ResizeObserver. Svelte 5 uses one for `bind:clientWidth` /
// `bind:clientHeight`, so components that virtualize on the viewport size need
// a no-op implementation in tests. Components must keep a fallback size for the
// first render, which is what the virtualized table does.
if (typeof globalThis.ResizeObserver === 'undefined') {
	class ResizeObserverStub {
		observe(): void {}
		unobserve(): void {}
		disconnect(): void {}
	}
	globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;
}
