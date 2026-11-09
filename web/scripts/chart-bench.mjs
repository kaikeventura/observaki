#!/usr/bin/env node
/**
 * Story 6.2 chart bench: drives the real /dashboards page in headless Chrome
 * and measures the acceptance criteria.
 *
 *   node web/scripts/chart-bench.mjs [--minutes 10] [--url http://...] [--out file.json]
 *
 * With no --url the script builds nothing: it expects `bin/observaki` and
 * `web/build` to exist and starts the server itself. Run it from the repo root
 * (the `make chart-bench` target does the build and calls it).
 *
 * No third-party dependency: it talks to Chrome over the DevTools protocol with
 * the WebSocket client built into Node.
 */
import { spawn } from 'node:child_process';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';

const options = parseArgs(process.argv.slice(2));
const cleanup = [];

process.on('SIGINT', async () => {
	await shutdown();
	process.exit(130);
});

function parseArgs(argv) {
	const parsed = {
		minutes: 10,
		url: '',
		out: '',
		port: 8099,
		chrome: process.env.CHROME_BIN || 'google-chrome',
		viewport: { width: 1440, height: 900 }
	};
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i];
		const next = () => {
			i += 1;
			if (i >= argv.length) throw new Error(`missing value for ${arg}`);
			return argv[i];
		};
		if (arg === '--minutes') parsed.minutes = Number(next());
		else if (arg === '--url') parsed.url = next();
		else if (arg === '--out') parsed.out = next();
		else if (arg === '--port') parsed.port = Number(next());
		else if (arg === '--chrome') parsed.chrome = next();
		else if (arg === '--help' || arg === '-h') {
			console.log(
				'usage: chart-bench.mjs [--minutes 10] [--url URL] [--out FILE] [--port 8099] [--chrome PATH]'
			);
			process.exit(0);
		} else throw new Error(`unknown argument: ${arg}`);
	}
	if (!Number.isFinite(parsed.minutes) || parsed.minutes < 0)
		throw new Error('--minutes must be >= 0');
	return parsed;
}

class Session {
	constructor(ws) {
		this.ws = ws;
		this.nextId = 0;
		this.pending = new Map();
		ws.addEventListener('message', (event) => {
			const message = JSON.parse(event.data);
			if (!message.id || !this.pending.has(message.id)) return;
			const { resolve, reject } = this.pending.get(message.id);
			this.pending.delete(message.id);
			if (message.error) reject(new Error(message.error.message));
			else resolve(message.result);
		});
	}

	static async connect(url) {
		const ws = new WebSocket(url);
		await new Promise((resolve, reject) => {
			ws.addEventListener('open', resolve, { once: true });
			ws.addEventListener('error', () => reject(new Error('devtools websocket failed')), {
				once: true
			});
		});
		return new Session(ws);
	}

	send(method, params = {}) {
		const id = (this.nextId += 1);
		this.ws.send(JSON.stringify({ id, method, params }));
		return new Promise((resolve, reject) => this.pending.set(id, { resolve, reject }));
	}

	async evaluate(expression) {
		const result = await this.send('Runtime.evaluate', {
			expression,
			awaitPromise: true,
			returnByValue: true
		});
		if (result.exceptionDetails) {
			throw new Error(result.exceptionDetails.exception?.description ?? 'evaluation failed');
		}
		return result.result.value;
	}

	close() {
		this.ws.close();
	}
}

async function waitForHttp(url, timeoutMs) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		try {
			const response = await fetch(url);
			if (response.ok) return true;
		} catch {
			// server not up yet
		}
		await sleep(200);
	}
	throw new Error(`server did not answer at ${url}`);
}

async function startServer(port) {
	const child = spawn('./bin/observaki', ['serve'], {
		env: {
			...process.env,
			OBSERVAKI_HTTP_ADDR: `127.0.0.1:${port}`,
			OBSERVAKI_WEB_DIR: 'web/build'
		},
		stdio: ['ignore', 'ignore', 'inherit']
	});
	cleanup.push(async () => {
		child.kill('SIGTERM');
	});
	await waitForHttp(`http://127.0.0.1:${port}/healthz`, 20_000);
	return `http://127.0.0.1:${port}/dashboards`;
}

async function startChrome(chrome, debugPort, profile) {
	const child = spawn(
		chrome,
		[
			'--headless=new',
			'--no-first-run',
			'--no-default-browser-check',
			'--disable-extensions',
			'--disable-sync',
			'--hide-scrollbars',
			'--enable-precise-memory-info',
			'--disable-background-timer-throttling',
			'--disable-renderer-backgrounding',
			'--disable-backgrounding-occluded-windows',
			`--window-size=${options.viewport.width},${options.viewport.height}`,
			`--remote-debugging-port=${debugPort}`,
			`--user-data-dir=${profile}`,
			'about:blank'
		],
		{ stdio: ['ignore', 'ignore', 'inherit'] }
	);
	cleanup.push(async () => {
		child.kill('SIGKILL');
	});

	const deadline = Date.now() + 20_000;
	while (Date.now() < deadline) {
		try {
			const response = await fetch(`http://127.0.0.1:${debugPort}/json/list`);
			const targets = await response.json();
			const page = targets.find((target) => target.type === 'page' && target.webSocketDebuggerUrl);
			if (page) return page.webSocketDebuggerUrl;
		} catch {
			// chrome not up yet
		}
		await sleep(200);
	}
	throw new Error('chrome devtools endpoint did not come up');
}

async function waitForChart(session, timeoutMs = 20_000) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const ready = await session.evaluate(
			`Boolean(document.querySelector('[data-chart-ready="true"]'))`
		);
		if (ready) return true;
		await sleep(100);
	}
	throw new Error('chart never became ready');
}

const CHART_STATE = `(() => {
	const host = document.querySelector('[data-chart-ready]');
	if (!host) return null;
	return {
		renderMs: Number(host.dataset.chartRenderMs || 0),
		paintMs: Number(host.dataset.chartPaintMs || 0),
		instances: Number(host.dataset.chartInstances || 0),
		updates: Number(host.dataset.chartUpdates || 0),
		drawn: Number(host.dataset.chartDrawn || 0),
		rangeMin: Number(host.dataset.chartRangeMin),
		rangeMax: Number(host.dataset.chartRangeMax),
		dataMin: Number(host.dataset.chartDataMin),
		dataMax: Number(host.dataset.chartDataMax),
		zoomed: host.dataset.chartZoomed === 'true',
		canvases: document.querySelectorAll('canvas').length,
		cls: Number(window.__observakiCls || 0)
	};
})()`;

function pickRange(state) {
	return {
		min: state.rangeMin,
		max: state.rangeMax,
		dataMin: state.dataMin,
		dataMax: state.dataMax,
		zoomed: state.zoomed
	};
}

async function hover(session, x, y) {
	await session.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x, y, buttons: 0 });
	await sleep(60);
	return session.evaluate(`(() => {
		const el = document.querySelector('[data-chart-tooltip]');
		if (!el) return null;
		return {
			time: Number(el.dataset.chartTooltipTime),
			value: Number(el.dataset.chartTooltipValue),
			text: el.innerText.replace(/\\n/g, ' ')
		};
	})()`);
}

async function pressKey(session, key, code) {
	await session.send('Input.dispatchKeyEvent', { type: 'keyDown', key, code });
	await session.send('Input.dispatchKeyEvent', { type: 'keyUp', key, code });
	await sleep(80);
}

async function run() {
	const url = options.url || (await startServer(options.port));
	const profile = await mkdtemp(join(tmpdir(), 'observaki-chart-'));
	cleanup.push(async () => {
		await rm(profile, { recursive: true, force: true });
	});

	const debugPort = 9400 + Math.floor(Math.random() * 300);
	const wsUrl = await startChrome(options.chrome, debugPort, profile);
	const session = await Session.connect(wsUrl);
	cleanup.push(async () => session.close());

	await session.send('Page.enable');
	await session.send('Runtime.enable');
	await session.send('Page.addScriptToEvaluateOnNewDocument', {
		source: `window.__observakiCls = 0;
			new PerformanceObserver((list) => {
				for (const entry of list.getEntries()) {
					if (!entry.hadRecentInput) window.__observakiCls += entry.value;
				}
			}).observe({ type: 'layout-shift', buffered: true });`
	});

	const navigation = await session.send('Page.navigate', { url });
	if (navigation.errorText) throw new Error(`navigation failed: ${navigation.errorText}`);
	await waitForChart(session);

	const load = await session.evaluate(`(() => {
		const nav = performance.getEntriesByType('navigation')[0];
		return {
			durationMs: Math.round(nav.duration * 10) / 10,
			domContentLoadedMs: Math.round(nav.domContentLoadedEventEnd * 10) / 10,
			transferSize: nav.transferSize,
			heapUsed: performance.memory ? performance.memory.usedJSHeapSize : 0
		};
	})()`);

	const initial = await session.evaluate(CHART_STATE);
	const plotRect = await session.evaluate(`(() => {
		const over = document.querySelector('.u-over');
		const rect = over.getBoundingClientRect();
		return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
	})()`);

	const hoverLeft = await hover(
		session,
		plotRect.x + plotRect.width * 0.3,
		plotRect.y + plotRect.height * 0.5
	);
	const hoverRight = await hover(
		session,
		plotRect.x + plotRect.width * 0.7,
		plotRect.y + plotRect.height * 0.5
	);

	const singleEvent = await session.evaluate(`(async () => {
		const over = document.querySelector('.u-over');
		const rect = over.getBoundingClientRect();
		const cx = rect.x + rect.width / 2;
		const cy = rect.y + rect.height / 2;
		const times = [];
		for (let i = 0; i < 10; i += 1) {
			const started = performance.now();
			over.dispatchEvent(new WheelEvent('wheel', {
				deltaY: i % 2 === 0 ? -120 : 120,
				clientX: cx,
				clientY: cy,
				bubbles: true,
				cancelable: true
			}));
			await Promise.resolve();
			await Promise.resolve();
			times.push(performance.now() - started);
		}
		return {
			avgMs: Math.round((times.reduce((sum, value) => sum + value, 0) / times.length) * 100) / 100,
			maxMs: Math.round(Math.max(...times) * 100) / 100
		};
	})()`);

	const idleFps = await session.evaluate(`new Promise((resolve) => {
		let frames = 0;
		const start = performance.now();
		function frame(now) {
			frames += 1;
			if (now - start < 1000) requestAnimationFrame(frame);
			else resolve({ frames, ms: Math.round(now - start), fps: Math.round((frames / ((now - start) / 1000)) * 10) / 10 });
		}
		requestAnimationFrame(frame);
	})`);

	await session.evaluate(`window.__benchLoop = function (kind, everyN, durationMs) {
		return new Promise((resolve) => {
			const over = document.querySelector('.u-over');
			const rect = over.getBoundingClientRect();
			const cx = rect.x + rect.width / 2;
			const cy = rect.y + rect.height / 2;
			let frames = 0;
			let last = 0;
			let worstFrame = 0;
			let workTotal = 0;
			const start = performance.now();
			function frame(now) {
				frames += 1;
				if (last) worstFrame = Math.max(worstFrame, now - last);
				last = now;
				const workStart = performance.now();
				if (frames % everyN === 0) {
					if (kind === 'hover') {
						const x = rect.x + rect.width * (0.2 + 0.6 * ((frames % 120) / 120));
						over.dispatchEvent(new MouseEvent('mousemove', { clientX: x, clientY: cy, bubbles: true }));
					} else {
						over.dispatchEvent(new WheelEvent('wheel', {
							deltaY: Math.floor(frames / 30) % 2 === 0 ? -120 : 120,
							shiftKey: kind === 'pan',
							clientX: cx,
							clientY: cy,
							bubbles: true,
							cancelable: true
						}));
					}
				}
				workTotal += performance.now() - workStart;
				if (now - start < durationMs) requestAnimationFrame(frame);
				else resolve({
					frames,
					durationMs: Math.round(now - start),
					fps: Math.round((frames / ((now - start) / 1000)) * 10) / 10,
					worstFrameMs: Math.round(worstFrame * 10) / 10,
					avgWorkMs: Math.round((workTotal / frames) * 100) / 100
				});
			}
			requestAnimationFrame(frame);
		});
	}`);

	const interaction = {};
	for (const [kind, everyN] of [
		['hover', 1],
		['zoom', 4],
		['pan', 4]
	]) {
		const runs = [];
		for (let run = 0; run < 3; run += 1) {
			runs.push(await session.evaluate(`window.__benchLoop('${kind}', ${everyN}, 1500)`));
		}
		interaction[kind] = runs;
	}

	// Control: same wheel loop with the canvas hidden, to separate the chart
	// work from headless (software) canvas rasterization.
	const rasterProbe = await session.evaluate(`new Promise((resolve) => {
		const canvas = document.querySelector('canvas');
		const previous = canvas.style.visibility;
		canvas.style.visibility = 'hidden';
		window.__benchLoop('zoom', 4, 1500).then((result) => {
			canvas.style.visibility = previous;
			resolve(result);
		});
	})`);

	// Drag-select zoom through real input events (the mouse alternative to the
	// keyboard and the wheel).
	const dragBefore = await session.evaluate(CHART_STATE);
	await session.send('Input.dispatchMouseEvent', {
		type: 'mousePressed',
		x: plotRect.x + plotRect.width * 0.3,
		y: plotRect.y + plotRect.height * 0.5,
		button: 'left',
		buttons: 1,
		clickCount: 1
	});
	for (let step = 1; step <= 6; step += 1) {
		await session.send('Input.dispatchMouseEvent', {
			type: 'mouseMoved',
			x: plotRect.x + plotRect.width * (0.3 + 0.06 * step),
			y: plotRect.y + plotRect.height * 0.5,
			button: 'left',
			buttons: 1
		});
		await sleep(30);
	}
	await session.send('Input.dispatchMouseEvent', {
		type: 'mouseReleased',
		x: plotRect.x + plotRect.width * 0.66,
		y: plotRect.y + plotRect.height * 0.5,
		button: 'left',
		buttons: 0,
		clickCount: 1
	});
	await sleep(150);
	const dragAfter = await session.evaluate(CHART_STATE);

	// Keyboard zoom, pan and reset on the focused plot.
	await session.evaluate(`document.querySelector('[data-chart-ready]').focus()`);
	await pressKey(session, '0', 'Digit0');
	const beforeKeys = await session.evaluate(CHART_STATE);
	await pressKey(session, '+', 'Equal');
	const afterZoom = await session.evaluate(CHART_STATE);
	await pressKey(session, 'ArrowRight', 'ArrowRight');
	const afterPan = await session.evaluate(CHART_STATE);
	await pressKey(session, '0', 'Digit0');
	const afterReset = await session.evaluate(CHART_STATE);

	// Live updates: the canvas node must survive and the instance count stay at 1.
	await session.evaluate(`document.querySelector('canvas').__observakiProbe = 'kept'`);
	const liveBefore = await session.evaluate(CHART_STATE);
	await sleep(5200);
	const liveAfter = await session.evaluate(CHART_STATE);
	const canvasReused = await session.evaluate(
		`document.querySelector('canvas')?.__observakiProbe === 'kept'`
	);

	// Memory soak at 1 Hz. The first two samples cover warm-up and are reported
	// separately so the growth figure is not dominated by JIT/GC noise.
	const samples = [];
	const soakMs = options.minutes * 60_000;
	const startedAt = Date.now();
	while (Date.now() - startedAt < soakMs) {
		const sample = await session.evaluate(`(() => {
			const host = document.querySelector('[data-chart-ready]');
			return {
				heap: performance.memory ? performance.memory.usedJSHeapSize : 0,
				updates: Number(host?.dataset.chartUpdates || 0),
				instances: Number(host?.dataset.chartInstances || 0)
			};
		})()`);
		samples.push({ atMs: Date.now() - startedAt, ...sample });
		await sleep(Math.min(15_000, Math.max(0, soakMs - (Date.now() - startedAt))));
	}
	if (samples.length === 0) samples.push({ atMs: 0, heap: 0, updates: 0, instances: 0 });

	const heaps = samples.map((sample) => sample.heap);
	const warm = samples.slice(2);
	const warmHeaps = warm.length > 0 ? warm.map((sample) => sample.heap) : heaps;
	const heapStart = warmHeaps[0];
	const heapEnd = warmHeaps[warmHeaps.length - 1];
	const heapMax = Math.max(...heaps);
	const heapMin = Math.min(...heaps);

	const fpsValues = Object.values(interaction)
		.flat()
		.map((run) => run.fps)
		.sort((a, b) => a - b);
	const minFps = fpsValues[0];
	const medianFps = fpsValues[Math.floor(fpsValues.length / 2)];

	const report = {
		generatedAt: new Date().toISOString(),
		url,
		chrome: options.chrome,
		viewport: options.viewport,
		soakMinutes: options.minutes,
		load,
		chart: initial,
		tooltip: { left: hoverLeft, right: hoverRight },
		singleEvent,
		idleFps,
		interaction,
		medianFps,
		minFps,
		rasterProbe,
		dragZoom: {
			before: { min: dragBefore.rangeMin, max: dragBefore.rangeMax },
			after: { min: dragAfter.rangeMin, max: dragAfter.rangeMax }
		},
		keyboard: {
			before: pickRange(beforeKeys),
			afterZoom: pickRange(afterZoom),
			afterPan: pickRange(afterPan),
			afterReset: pickRange(afterReset)
		},
		live: {
			before: liveBefore.updates,
			after: liveAfter.updates,
			instances: liveAfter.instances,
			canvasReused
		},
		memory: {
			samples,
			heapStart,
			heapEnd,
			heapMin,
			heapMax,
			growthBytes: heapEnd - heapStart,
			rangeBytes: heapMax - heapMin
		},
		pass: {
			firstPaintUnder100ms: initial.paintMs > 0 && initial.paintMs < 100,
			interactionNear60fps: minFps >= 55,
			tooltipCorrect:
				hoverLeft !== null &&
				hoverRight !== null &&
				hoverRight.time > hoverLeft.time &&
				hoverLeft.time >= initial.rangeMin &&
				hoverRight.time <= initial.rangeMax &&
				hoverLeft.value > 0 &&
				hoverRight.value > 0,
			keyboardZoom:
				afterZoom.rangeMax - afterZoom.rangeMin < beforeKeys.rangeMax - beforeKeys.rangeMin,
			dragZoom: dragAfter.rangeMax - dragAfter.rangeMin < dragBefore.rangeMax - dragBefore.rangeMin,
			keyboardPan: afterPan.rangeMin > afterZoom.rangeMin,
			keyboardReset:
				afterReset.zoomed === false &&
				Math.abs(afterReset.rangeMin - afterReset.dataMin) < 2 &&
				Math.abs(afterReset.rangeMax - afterReset.dataMax) < 2,
			liveReusesPlot:
				liveAfter.instances === 1 && canvasReused && liveAfter.updates > liveBefore.updates,
			memoryStable: heapEnd - heapStart < 8 * 1024 * 1024 && heapMax - heapMin < 16 * 1024 * 1024
		}
	};
	report.pass.all = Object.values(report.pass).every(Boolean);

	if (options.out) await writeFile(options.out, `${JSON.stringify(report, null, 2)}\n`);
	console.log(JSON.stringify(report, null, 2));
	return report;
}

async function shutdown() {
	for (const task of cleanup.reverse()) {
		try {
			await task();
		} catch {
			// best effort
		}
	}
}

run()
	.then(async (report) => {
		await shutdown();
		process.exit(report.pass.all ? 0 : 1);
	})
	.catch(async (error) => {
		console.error(error);
		await shutdown();
		process.exit(1);
	});
