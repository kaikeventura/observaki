#!/usr/bin/env node
/**
 * Story 6.4 bundle budget: proves the production build stays inside the Phase 0
 * frontend budget and fails the build on a regression.
 *
 *   node web/scripts/bundle-budget.mjs [--build web/build] [--json report.json]
 *
 * "Initial JS" is what the entry document preloads: the SvelteKit runtime, the
 * root layout and the entry route. A route budget counts every chunk the route
 * can pull in, including the lazily imported uPlot chunk on /dashboards.
 *
 * No third-party dependency: the build output is HTML + ES modules on disk.
 */
import { readFile, writeFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

/** KiB budgets from Story 6.4 AC1. */
export const BUDGETS = {
	initial: 180,
	dashboards: 250
};

const ROUTES = [
	{ id: 'initial', html: 'index.html', budget: BUDGETS.initial, eager: true },
	{ id: 'dashboards', html: 'dashboards.html', budget: BUDGETS.dashboards },
	{ id: 'explore', html: 'explore.html', budget: null }
];

function parseArgs(argv) {
	const options = { build: join(ROOT, 'web/build'), json: '' };
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i];
		if (arg === '--build') options.build = resolve(argv[(i += 1)]);
		else if (arg === '--json') options.json = resolve(argv[(i += 1)]);
		else if (arg === '--help' || arg === '-h') {
			console.log('usage: bundle-budget.mjs [--build DIR] [--json FILE]');
			process.exit(0);
		} else throw new Error(`unknown argument: ${arg}`);
	}
	return options;
}

function KiB(bytes) {
	return bytes / 1024;
}

function formatKiB(bytes) {
	return `${KiB(bytes).toFixed(1)} KB`;
}

/** Absolute script/module references in a built document. */
function documentAssets(html, file) {
	const assets = new Set();
	const link = /<link[^>]+href="([^"]+\.js)"[^>]*>/g;
	const script = /<script[^>]+src="([^"]+\.js)"/g;
	for (const match of html.matchAll(link)) assets.add(resolveUrl(match[1], file));
	for (const match of html.matchAll(script)) assets.add(resolveUrl(match[1], file));
	return assets;
}

function documentStyles(html, file) {
	const assets = new Set();
	const link = /<link[^>]+href="([^"]+\.css)"[^>]*>/g;
	for (const match of html.matchAll(link)) {
		const href = match[1];
		assets.add(resolve(dirname(file), href));
	}
	return assets;
}

function resolveUrl(href, file) {
	return resolve(dirname(file), href);
}

/** `./chunk.js` and `import("./chunk.js")` both appear in Vite chunks. */
function chunkImports(source, file) {
	const assets = new Set();
	const imports = /["'](\.{1,2}\/[^"']+\.js)["']/g;
	for (const match of source.matchAll(imports)) assets.add(resolve(dirname(file), match[1]));
	return assets;
}

async function closure(entries, { followDynamic }) {
	const seen = new Map();
	const queue = [...entries];
	while (queue.length > 0) {
		const file = queue.pop();
		if (seen.has(file)) continue;
		const source = await readFile(file, 'utf8');
		seen.set(file, Buffer.byteLength(source));
		if (!followDynamic) continue;
		for (const imported of chunkImports(source, file)) {
			if (!seen.has(imported)) queue.push(imported);
		}
	}
	return seen;
}

async function gzipSizes(files) {
	let raw = 0;
	let gzip = 0;
	for (const file of files) {
		const buffer = await readFile(file);
		raw += buffer.byteLength;
		// Level 9 matches what the release artifacts are served with.
		gzip += gzipSync(buffer, { level: 9 }).byteLength;
	}
	return { raw, gzip, count: files.length };
}

async function main() {
	const options = parseArgs(process.argv.slice(2));
	const results = [];

	for (const route of ROUTES) {
		const file = join(options.build, route.html);
		if (!existsSync(file)) {
			throw new Error(`missing ${file}; run the web build first`);
		}
		const html = await readFile(file, 'utf8');
		const entries = [...documentAssets(html, file)];
		if (entries.length === 0) throw new Error(`no scripts found in ${route.html}`);

		// The initial budget is what the document preloads (what blocks first
		// paint). Route budgets include every chunk the route can reach.
		const files = await closure(entries, { followDynamic: !route.eager });
		const js = await gzipSizes([...files.keys()]);
		const css = await gzipSizes([...documentStyles(html, file)]);
		const pass = route.budget === null || KiB(js.gzip) <= route.budget;

		results.push({
			route: route.id,
			html: route.html,
			budgetKiB: route.budget,
			jsFiles: js.count,
			jsRawBytes: js.raw,
			jsGzipBytes: js.gzip,
			cssGzipBytes: css.gzip,
			pass
		});
	}

	for (const result of results) {
		const budget = result.budgetKiB === null ? 'no budget' : `budget ${result.budgetKiB} KB gzip`;
		const verdict = result.pass ? 'pass' : 'FAIL';
		console.log(
			`${result.route.padEnd(11)} JS ${formatKiB(result.jsGzipBytes).padStart(9)} ` +
				`(${result.jsFiles} files, ${formatKiB(result.jsRawBytes)} raw, CSS ` +
				`${formatKiB(result.cssGzipBytes)}) · ${budget} · ${verdict}`
		);
	}

	const failed = results.filter((result) => !result.pass);
	if (options.json) {
		await writeFile(
			options.json,
			`${JSON.stringify({ generatedAt: new Date().toISOString(), budgets: BUDGETS, results }, null, 2)}\n`
		);
	}

	if (failed.length > 0) {
		console.error('');
		for (const result of failed) {
			console.error(
				`bundle budget exceeded: ${result.route} is ${formatKiB(result.jsGzipBytes)} gzip, ` +
					`budget ${result.budgetKiB} KB`
			);
		}
		console.error('Reduce the chunk or update the Story 6.4 budget on purpose (and record it).');
		process.exitCode = 1;
	}
}

main().catch((error) => {
	console.error(error.message);
	process.exitCode = 1;
});
