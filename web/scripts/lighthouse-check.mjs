#!/usr/bin/env node
/**
 * Story 6.4 Lighthouse check: measures the Explore route on a reference laptop
 * profile (desktop, 50 Mbps) and enforces LCP < 1.5 s and INP < 200 ms.
 *
 *   node web/scripts/lighthouse-check.mjs [--url http://127.0.0.1:8099] [--out report.json]
 *
 * With no --url the script starts `bin/observaki serve` for `web/build` itself,
 * so `make lighthouse` only has to build. LCP comes from a Lighthouse
 * navigation run with 50 Mbps simulated throttling; INP comes from a Lighthouse
 * timespan run while the script drives real interactions on the mock Explore
 * (signal tabs, autocomplete, service filter), because INP is only collected in
 * unsimulated user flows.
 *
 * Lighthouse + puppeteer-core are dev-only dependencies; nothing here ships in
 * the web bundle.
 */
import { execFileSync, spawn } from 'node:child_process';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import lighthouse, { startTimespan } from 'lighthouse';
import puppeteer from 'puppeteer-core';

/** Story 6.4 AC2 budgets. */
export const BUDGETS = { lcpMs: 1_500, inpMs: 200 };

/** Reference laptop profile: desktop form factor, 50 Mbps, no CPU slowdown. */
const PROFILE = {
	formFactor: 'desktop',
	screenEmulation: { mobile: false, width: 1440, height: 900, deviceScaleFactor: 1 },
	rttMs: 40,
	// 50 Mbps is 50,000 Kbps; Lighthouse throttling is in Kbps.
	throughputKbps: 50_000,
	cpuSlowdownMultiplier: 1
};

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
		chrome: process.env.CHROME_BIN || 'google-chrome'
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
			console.log(
				'usage: lighthouse-check.mjs [--url URL] [--out FILE] [--port 8099] [--chrome PATH]'
			);
			process.exit(0);
		} else throw new Error(`unknown argument: ${arg}`);
	}
	return parsed;
}

async function shutdown() {
	for (const step of cleanup.reverse()) {
		try {
			await step();
		} catch {
			// best effort
		}
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
	return `http://127.0.0.1:${port}`;
}

function resolveChrome(candidate) {
	if (candidate.startsWith('/')) return candidate;
	try {
		return execFileSync('which', [candidate], { encoding: 'utf8' }).trim();
	} catch {
		throw new Error(`browser not found: ${candidate} (set CHROME_BIN)`);
	}
}

async function startBrowser(chrome) {
	const executablePath = resolveChrome(chrome);
	const profile = await mkdtemp(join(tmpdir(), 'observaki-lighthouse-'));
	cleanup.push(() => rm(profile, { recursive: true, force: true }));

	const browser = await puppeteer.launch({
		executablePath,
		headless: true,
		userDataDir: profile,
		args: ['--no-sandbox', '--disable-dev-shm-usage', '--no-first-run', '--disable-extensions']
	});
	cleanup.push(() => browser.close());
	return browser;
}

function lighthouseFlags(browser, overrides = {}) {
	return {
		port: Number(new URL(browser.wsEndpoint()).port),
		output: 'json',
		logLevel: 'error',
		formFactor: PROFILE.formFactor,
		screenEmulation: PROFILE.screenEmulation,
		throttling: {
			rttMs: PROFILE.rttMs,
			throughputKbps: PROFILE.throughputKbps,
			cpuSlowdownMultiplier: PROFILE.cpuSlowdownMultiplier
		},
		...overrides
	};
}

async function navigationRun(browser, url) {
	const result = await lighthouse(
		url,
		lighthouseFlags(browser, {
			throttlingMethod: 'simulate',
			onlyAudits: [
				'largest-contentful-paint',
				'first-contentful-paint',
				'cumulative-layout-shift',
				'total-blocking-time',
				'total-byte-weight',
				'interactive'
			]
		})
	);
	const lhr = result.lhr;
	return {
		lcpMs: lhr.audits['largest-contentful-paint']?.numericValue ?? null,
		fcpMs: lhr.audits['first-contentful-paint']?.numericValue ?? null,
		cls: lhr.audits['cumulative-layout-shift']?.numericValue ?? null,
		tbtMs: lhr.audits['total-blocking-time']?.numericValue ?? null,
		transferBytes: lhr.audits['total-byte-weight']?.numericValue ?? null,
		interactiveMs: lhr.audits['interactive']?.numericValue ?? null
	};
}

/**
 * Real user interactions on the mock Explore while Lighthouse records a
 * timespan. The flow is the one the story promises: cycle signals, autocomplete
 * a query and change the service filter.
 */
async function driveInteractions(page) {
	await page.click('[data-signal-tab="traces"]');
	await sleep(120);
	await page.click('[data-signal-tab="metrics"]');
	await sleep(120);
	await page.focus('[data-query-input]');
	await page.keyboard.type('serv', { delay: 10 });
	await sleep(120);
	await page.keyboard.press('ArrowDown');
	await page.keyboard.press('Enter');
	await sleep(120);
	await page.select('#service-filter', 'checkout');
	await sleep(120);
	await page.click('[data-range="6h"]');
	await sleep(120);
	await page.click('[data-signal-tab="logs"]');
	await sleep(120);
	await page.click('[data-signal-tab="traces"]');
	await sleep(120);
}

async function timespanRun(browser, page, url) {
	await page.setViewport(PROFILE.screenEmulation);
	await page.goto(url, { waitUntil: 'load' });
	await page.waitForSelector('[data-query-input]');
	// Let hydration settle before the measured window starts.
	await sleep(400);

	const timespan = await startTimespan(
		page,
		lighthouseFlags(browser, {
			throttlingMethod: 'provided',
			disableStorageReset: true
		})
	);
	await driveInteractions(page);
	const result = await timespan.endTimespan();
	const audit = result.lhr.audits['interaction-to-next-paint'];
	return {
		inpMs: audit?.numericValue ?? null,
		inpDisplay: audit?.displayValue ?? 'n/a',
		interactions: audit?.details?.items?.length ?? null
	};
}

function formatMs(value) {
	return value === null ? 'n/a' : `${Math.round(value)} ms`;
}

async function main() {
	const base = options.url ? options.url.replace(/\/$/, '') : await startServer(options.port);
	const url = `${base}/explore`;
	const browser = await startBrowser(options.chrome);
	const chromeVersion = await browser.version();

	const navigation = await navigationRun(browser, url);
	const timespan = await timespanRun(browser, await browser.newPage(), url);

	const checks = {
		lcpUnderBudget: navigation.lcpMs !== null && navigation.lcpMs < BUDGETS.lcpMs,
		inpUnderBudget: timespan.inpMs !== null && timespan.inpMs < BUDGETS.inpMs
	};
	const pass = checks.lcpUnderBudget && checks.inpUnderBudget;

	const report = {
		generatedAt: new Date().toISOString(),
		url,
		chrome: chromeVersion,
		profile: PROFILE,
		budgets: BUDGETS,
		metrics: { ...navigation, ...timespan },
		checks,
		pass
	};

	console.log(`Lighthouse (${chromeVersion}) on ${url}`);
	console.log(
		`  LCP ${formatMs(navigation.lcpMs)} (budget < ${BUDGETS.lcpMs} ms) · ` +
			`INP ${formatMs(timespan.inpMs)} (budget < ${BUDGETS.inpMs} ms) · ` +
			`CLS ${navigation.cls ?? 'n/a'} · TBT ${formatMs(navigation.tbtMs)} · ` +
			`${navigation.transferBytes ? `${(navigation.transferBytes / 1024).toFixed(1)} KB` : 'n/a'}`
	);
	console.log(`  ${pass ? 'pass' : 'FAIL'}`);

	if (options.out) {
		await writeFile(options.out, `${JSON.stringify(report, null, 2)}\n`);
	}

	if (!pass) {
		if (!checks.lcpUnderBudget) {
			console.error(`LCP ${formatMs(navigation.lcpMs)} exceeds the ${BUDGETS.lcpMs} ms budget`);
		}
		if (!checks.inpUnderBudget) {
			console.error(
				`INP ${formatMs(timespan.inpMs)} exceeds the ${BUDGETS.inpMs} ms budget ` +
					'(or was not observed; check the interaction flow)'
			);
		}
		process.exitCode = 1;
	}
}

try {
	await main();
} catch (error) {
	console.error(error.message);
	process.exitCode = 1;
} finally {
	await shutdown();
}
