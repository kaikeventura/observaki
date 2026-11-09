#!/usr/bin/env node
/**
 * Story 6.3 table bench: drives the real /explore page in headless Chrome and
 * measures the acceptance criteria of the virtualized 10k-row table.
 *
 *   node web/scripts/table-bench.mjs [--url http://...] [--out file.json]
 *
 * With no --url the script builds nothing: it expects `bin/observaki` and
 * `web/build` to exist and starts the server itself. Run it from the repo root
 * (the `make table-bench` target does the build and calls it).
 *
 * No third-party dependency: it talks to Chrome over the DevTools protocol with
 * the WebSocket client built into Node.
 */
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';

const options = parseArgs(process.argv.slice(2));
const cleanup = [];

process.on('SIGINT', async () => {
	await shutdown();
	process.exit(130);
});

function parseArgs(argv) {
	const parsed = {
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
		if (arg === '--url') parsed.url = next();
		else if (arg === '--out') parsed.out = next();
		else if (arg === '--port') parsed.port = Number(next());
		else if (arg === '--chrome') parsed.chrome = next();
		else if (arg === '--help' || arg === '-h') {
			console.log('usage: table-bench.mjs [--url URL] [--out FILE] [--port 8099] [--chrome PATH]');
			process.exit(0);
		} else throw new Error(`unknown argument: ${arg}`);
	}
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
	return `http://127.0.0.1:${port}/explore`;
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

async function waitForSelector(session, selector, timeoutMs = 5_000) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const found = await session.evaluate(
			`Boolean(document.querySelector(${JSON.stringify(selector)}))`
		);
		if (found) return true;
		await sleep(50);
	}
	throw new Error(`selector never appeared: ${selector}`);
}

const TABLE_STATE = `(() => {
	const host = document.querySelector('[data-table-ready="true"]');
	if (!host) return null;
	const rows = [...host.querySelectorAll('tr[data-row-id]')];
	const widths = {};
	for (const col of document.querySelectorAll('col[data-column]')) {
		widths[col.dataset.column] = Number(col.dataset.width);
	}
	return {
		total: Number(host.dataset.tableTotal),
		rendered: Number(host.dataset.tableRendered),
		start: Number(host.dataset.tableStart),
		end: Number(host.dataset.tableEnd),
		scrollTop: Number(host.dataset.tableScrollTop),
		sort: host.dataset.tableSort,
		selected: host.dataset.tableSelected,
		selectedIndex: Number(host.dataset.tableSelectedIndex),
		detail: host.dataset.tableDetail === 'true',
		resizes: Number(host.dataset.tableResizes),
		rowHeight: Number(host.dataset.tableRowHeight),
		viewport: host.clientHeight,
		scrollHeight: host.scrollHeight,
		domRows: rows.length,
		durations: rows.map((row) => Number(row.dataset.rowDuration)),
		firstIndex: Number(rows[0]?.dataset.rowIndex),
		widths,
		cls: Number(window.__observakiCls || 0)
	};
})()`;

function isAscending(values) {
	return values.every((value, index) => index === 0 || value >= values[index - 1]);
}

function isDescending(values) {
	return values.every((value, index) => index === 0 || value <= values[index - 1]);
}

async function clickButtonByText(session, text) {
	return session.evaluate(`(() => {
		const button = [...document.querySelectorAll('button')].find(
			(candidate) => candidate.textContent.trim() === ${JSON.stringify(text)}
		);
		if (!button) return false;
		button.click();
		return true;
	})()`);
}

async function run() {
	const url = options.url || (await startServer(options.port));
	const profile = await mkdtemp(join(tmpdir(), 'observaki-table-'));
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
			window.__observakiShifts = [];
			new PerformanceObserver((list) => {
				for (const entry of list.getEntries()) {
					if (!entry.hadRecentInput) {
						window.__observakiCls += entry.value;
						if (window.__observakiShifts.length < 40) {
							window.__observakiShifts.push({
								value: entry.value,
								hadRecentInput: entry.hadRecentInput,
								sources: entry.sources.map((source) => ({
									node: source.node ? source.node.nodeName : '',
									id: source.node ? source.node.id : '',
									className: source.node ? String(source.node.className || '').slice(0, 80) : '',
									previous: source.previousRect,
									current: source.currentRect
								}))
							});
						}
					}
				}
			}).observe({ type: 'layout-shift', buffered: true });`
	});

	const navigation = await session.send('Page.navigate', { url });
	if (navigation.errorText) throw new Error(`navigation failed: ${navigation.errorText}`);
	await waitForSelector(session, '[data-table-ready="true"]');
	await sleep(150);

	const load = await session.evaluate(`(() => {
		const nav = performance.getEntriesByType('navigation')[0];
		return {
			durationMs: Math.round(nav.duration * 10) / 10,
			domContentLoadedMs: Math.round(nav.domContentLoadedEventEnd * 10) / 10,
			transferSize: nav.transferSize,
			heapUsed: performance.memory ? performance.memory.usedJSHeapSize : 0
		};
	})()`);

	const initial = await session.evaluate(TABLE_STATE);
	const clsAtLoad = await session.evaluate(
		`({ value: window.__observakiCls, shifts: window.__observakiShifts })`
	);

	// Scroll through the 10k rows with real wheel input while a page-side rAF
	// loop counts frames. This exercises browser scrolling, Svelte's row diff
	// and layout together, exactly like a user wheel gesture.
	await session.evaluate(`window.__startTableFrames = function (durationMs) {
		window.__tableFrameResult = new Promise((resolve) => {
			const host = document.querySelector('[data-table-ready="true"]');
			let frames = 0;
			let last = 0;
			let worstFrame = 0;
			let maxDom = 0;
			const start = performance.now();
			function frame(now) {
				frames += 1;
				if (last) worstFrame = Math.max(worstFrame, now - last);
				last = now;
				maxDom = Math.max(maxDom, host.querySelectorAll('tr[data-row-id]').length);
				if (now - start < durationMs) requestAnimationFrame(frame);
				else resolve({
					frames,
					durationMs: Math.round(now - start),
					fps: Math.round((frames / ((now - start) / 1000)) * 10) / 10,
					worstFrameMs: Math.round(worstFrame * 10) / 10,
					maxDom,
					endScrollTop: Math.round(host.scrollTop)
				});
			}
			requestAnimationFrame(frame);
		});
		return true;
	}`);

	const scrollPoint = await session.evaluate(`(() => {
		const rect = document.querySelector('[data-table-ready="true"]').getBoundingClientRect();
		return { x: rect.x + rect.width / 2, y: rect.y + Math.min(200, rect.height / 2) };
	})()`);

	const scrollRuns = [];
	for (let run = 0; run < 3; run += 1) {
		await session.evaluate(`document.querySelector('[data-table-ready="true"]').scrollTop = 0`);
		await session.evaluate('window.__startTableFrames(1500)');
		const started = Date.now();
		while (Date.now() - started < 1500) {
			await session.send('Input.dispatchMouseEvent', {
				type: 'mouseWheel',
				x: scrollPoint.x,
				y: scrollPoint.y,
				deltaX: 0,
				deltaY: 600
			});
			await sleep(10);
		}
		scrollRuns.push(await session.evaluate('window.__tableFrameResult'));
	}
	const scrollFps = scrollRuns.map((run) => run.fps).sort((a, b) => a - b);
	const minScrollFps = scrollFps[0];
	const maxDom = Math.max(...scrollRuns.map((run) => run.maxDom));

	const idleFps = await session.evaluate(`new Promise((resolve) => {
		let frames = 0;
		const start = performance.now();
		function frame(now) {
			frames += 1;
			if (now - start < 1000) requestAnimationFrame(frame);
			else resolve({ frames, fps: Math.round((frames / ((now - start) / 1000)) * 10) / 10 });
		}
		requestAnimationFrame(frame);
	})`);
	const clsAfterScroll = await session.evaluate('window.__observakiCls');

	// Back to the top so the assertions look at the first sorted rows.
	await session.evaluate(`document.querySelector('[data-table-ready="true"]').scrollTop = 0`);
	await sleep(120);

	const resizeAria = await session.evaluate(`(() => {
		const handle = document.querySelector('[data-resizer="service"]');
		handle.focus();
		for (let i = 0; i < 4; i += 1) {
			handle.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }));
		}
		return true;
	})()`);
	await sleep(120);
	const afterResize = await session.evaluate(TABLE_STATE);
	const clsAfterResize = await session.evaluate('window.__observakiCls');
	const resizeAriaNow = await session.evaluate(
		`document.querySelector('[data-resizer="service"]').getAttribute('aria-valuenow')`
	);

	// Pointer resize through real input events, the mouse path next to the
	// keyboard path above.
	const resizerRect = await session.evaluate(`(() => {
		const rect = document.querySelector('[data-resizer="service"]').getBoundingClientRect();
		return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
	})()`);
	await session.send('Input.dispatchMouseEvent', {
		type: 'mousePressed',
		x: resizerRect.x,
		y: resizerRect.y,
		button: 'left',
		buttons: 1,
		clickCount: 1
	});
	for (let step = 1; step <= 4; step += 1) {
		await session.send('Input.dispatchMouseEvent', {
			type: 'mouseMoved',
			x: resizerRect.x + step * 12,
			y: resizerRect.y,
			button: 'left',
			buttons: 1
		});
		await sleep(25);
	}
	await session.send('Input.dispatchMouseEvent', {
		type: 'mouseReleased',
		x: resizerRect.x + 48,
		y: resizerRect.y,
		button: 'left',
		buttons: 0,
		clickCount: 1
	});
	await sleep(120);
	const afterPointerResize = await session.evaluate(TABLE_STATE);

	// Sort by duration: first click ascending, second click descending. The
	// width set by the keyboard resize and the selected row must survive.
	await session.evaluate(`document.querySelector('th[data-column="duration"] button').click()`);
	await sleep(150);
	const afterSortAsc = await session.evaluate(TABLE_STATE);

	await session.evaluate(`document.querySelector('th[data-column="duration"] button').click()`);
	await sleep(150);
	const afterSortDesc = await session.evaluate(TABLE_STATE);
	const clsAfterSort = await session.evaluate('window.__observakiCls');

	// Select the first sorted row, confirming the drawer and the id.
	await session.evaluate(`document.querySelector('tr[data-row-id]').click()`);
	await sleep(150);
	const afterSelect = await session.evaluate(TABLE_STATE);
	const detailText = await session.evaluate(`(() => {
		const panel = document.querySelector('[data-table-detail-panel]');
		return panel ? panel.innerText.replace(/\\s+/g, ' ').trim() : '';
	})()`);
	const detailId = await session.evaluate(
		`document.querySelector('[data-table-detail-id]')?.getAttribute('data-table-detail-id') || ''`
	);
	const clsAfterSelect = await session.evaluate('window.__observakiCls');

	await session.evaluate(
		`window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))`
	);
	await sleep(80);
	const afterEscape = await session.evaluate(TABLE_STATE);

	// j moves the selection; Enter opens it again.
	const beforeJ = await session.evaluate(TABLE_STATE);
	await session.evaluate(`(() => {
		const host = document.querySelector('[data-table-ready="true"]');
		host.focus();
		host.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', bubbles: true, cancelable: true }));
		return true;
	})()`);
	await sleep(120);
	const afterJ = await session.evaluate(TABLE_STATE);
	await session.evaluate(`(() => {
		const host = document.querySelector('[data-table-ready="true"]');
		host.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
		return true;
	})()`);
	await sleep(120);
	const afterEnter = await session.evaluate(TABLE_STATE);
	await session.evaluate(
		`window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))`
	);
	await sleep(80);

	// Empty result: the frame stays and the retry restores 10k rows.
	await session.evaluate(`document.querySelector('[data-scenario="empty"]').click()`);
	await waitForSelector(session, '[data-table-state-frame="empty"]');
	const emptyState = await session.evaluate(`(() => {
		const frame = document.querySelector('[data-table-state-frame="empty"]');
		const retry = [...document.querySelectorAll('button')].some(
			(button) => button.textContent.trim() === 'Retry query'
		);
		return { text: frame?.innerText.replace(/\\s+/g, ' ').trim() || '', retry };
	})()`);
	await clickButtonByText(session, 'Retry query');
	await waitForSelector(session, '[data-table-ready="true"]');
	await sleep(150);
	const afterEmptyRetry = await session.evaluate(TABLE_STATE);

	// Error result: alert + request id, then retry.
	await session.evaluate(`document.querySelector('[data-scenario="error"]').click()`);
	await waitForSelector(session, '[data-table-state-frame="error"]');
	const errorState = await session.evaluate(`(() => {
		const frame = document.querySelector('[data-table-state-frame="error"]');
		const alert = document.querySelector('[role="alert"]');
		const retry = [...document.querySelectorAll('button')].some(
			(button) => button.textContent.trim() === 'Retry query'
		);
		return {
			text: frame?.innerText.replace(/\\s+/g, ' ').trim() || '',
			alert: alert?.innerText.replace(/\\s+/g, ' ').trim() || '',
			retry
		};
	})()`);
	await clickButtonByText(session, 'Retry query');
	await waitForSelector(session, '[data-table-ready="true"]');
	await sleep(150);
	const afterErrorRetry = await session.evaluate(TABLE_STATE);

	const final = await session.evaluate(TABLE_STATE);
	const widthsBefore = initial.widths;
	const widthsAfterResize = afterResize.widths;
	const widthsAfterPointerResize = afterPointerResize.widths;
	const widthsAfterSort = afterSortDesc.widths;
	const widthsAfterSelect = afterSelect.widths;

	const report = {
		generatedAt: new Date().toISOString(),
		url,
		chrome: options.chrome,
		viewport: options.viewport,
		load,
		initial,
		cls: {
			atLoad: clsAtLoad,
			stages: {
				afterScroll: clsAfterScroll,
				afterResize: clsAfterResize,
				afterSort: clsAfterSort,
				afterSelect: clsAfterSelect
			},
			session: await session.evaluate(
				`({ value: window.__observakiCls, shifts: window.__observakiShifts })`
			)
		},
		scroll: {
			runs: scrollRuns,
			minFps: minScrollFps,
			medianFps: scrollFps[1],
			maxDom
		},
		idleFps,
		resize: {
			ariaDispatched: resizeAria,
			beforeWidth: widthsBefore.service,
			afterWidth: widthsAfterResize.service,
			ariaValueNow: Number(resizeAriaNow),
			afterPointerWidth: widthsAfterPointerResize.service,
			widthAfterSort: widthsAfterSort.service,
			widthAfterSelect: widthsAfterSelect.service,
			resizes: afterPointerResize.resizes
		},
		sort: {
			asc: { sort: afterSortAsc.sort, durations: afterSortAsc.durations },
			desc: { sort: afterSortDesc.sort, durations: afterSortDesc.durations }
		},
		selection: {
			afterSelect: {
				id: afterSelect.selected,
				index: afterSelect.selectedIndex,
				detail: afterSelect.detail
			},
			detailId,
			detailText,
			afterEscape: { id: afterEscape.selected, detail: afterEscape.detail },
			beforeJ: { id: beforeJ.selected, index: beforeJ.selectedIndex },
			afterJ: { id: afterJ.selected, index: afterJ.selectedIndex },
			afterEnter: { id: afterEnter.selected, detail: afterEnter.detail }
		},
		states: {
			empty: emptyState,
			afterEmptyRetry: { total: afterEmptyRetry.total, sort: afterEmptyRetry.sort },
			error: errorState,
			afterErrorRetry: { total: afterErrorRetry.total }
		},
		final,
		pass: {
			virtualized:
				initial.total === 10_000 &&
				initial.rendered > 0 &&
				initial.rendered < 60 &&
				initial.domRows === initial.rendered,
			scrollNear60fps: minScrollFps >= 55,
			domStaysBounded: maxDom < 60,
			sortWorks:
				afterSortAsc.sort === 'duration:asc' &&
				afterSortDesc.sort === 'duration:desc' &&
				isAscending(afterSortAsc.durations) &&
				isDescending(afterSortDesc.durations),
			resizeByKeyboard:
				widthsAfterResize.service > widthsBefore.service &&
				Number(resizeAriaNow) === widthsAfterResize.service,
			resizeByPointer:
				widthsAfterPointerResize.service > widthsAfterResize.service &&
				afterPointerResize.resizes > afterResize.resizes,
			resizePreserved:
				widthsAfterSort.service === widthsAfterPointerResize.service &&
				widthsAfterSelect.service === widthsAfterPointerResize.service,
			sortPreserved: afterSelect.sort === 'duration:desc' && afterEscape.sort === 'duration:desc',
			selectionOpensDetail:
				afterSelect.detail &&
				afterSelect.selected !== '' &&
				detailId === afterSelect.selected &&
				detailText.includes(afterSelect.selected),
			escapeKeepsSelection: !afterEscape.detail && afterEscape.selected === afterSelect.selected,
			keyboardMovesSelection:
				afterJ.selectedIndex === beforeJ.selectedIndex + 1 && afterJ.selected !== beforeJ.selected,
			enterOpensDetail: afterEnter.detail,
			emptyStateRetries:
				emptyState.text.includes('No logs match this filter.') &&
				emptyState.retry &&
				afterEmptyRetry.total === 10_000,
			errorStateRetries:
				errorState.retry &&
				errorState.alert.includes('The query failed.') &&
				errorState.alert.includes('req_01J8Z3K4') &&
				afterErrorRetry.total === 10_000,
			widthsUnchangedBySortAndSelection:
				JSON.stringify(widthsAfterSort) === JSON.stringify(widthsAfterPointerResize) &&
				JSON.stringify(widthsAfterSelect) === JSON.stringify(widthsAfterPointerResize)
		}
	};
	report.pass.all = Object.values(report.pass).every(Boolean);

	if (options.out) {
		await mkdir(dirname(options.out), { recursive: true });
		await writeFile(options.out, `${JSON.stringify(report, null, 2)}\n`);
	}
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
