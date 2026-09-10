#!/usr/bin/env node

import { pathToFileURL } from "node:url";

const modulePath = process.argv[2];
if (!modulePath) {
	console.error("usage: pi-anchor-provider.mjs /absolute/path/to/pi/hashline/hash.js");
	process.exit(2);
}

const hashline = await import(pathToFileURL(modulePath).href);
if (typeof hashline.initHasher !== "function" || typeof hashline.lineHashes !== "function") {
	throw new Error(`${modulePath} does not export initHasher and lineHashes`);
}

let input = "";
for await (const chunk of process.stdin) input += chunk;
const request = JSON.parse(input);
if (request.protocol_version !== 1 || !Array.isArray(request.files)) {
	throw new Error("unsupported Grepple anchor protocol request");
}

await hashline.initHasher();
const files = [];
for (const file of request.files) {
	const hashes = await hashline.lineHashes(file.content, file.path);
	files.push({
		path: file.path,
		sha256: file.sha256,
		anchors: file.lines.map((line) => {
			const anchor = hashes[line - 1];
			if (!anchor) throw new Error(`Pi did not return an anchor for ${file.path}:${line}`);
			return { line, anchor };
		}),
	});
}

process.stdout.write(`${JSON.stringify({ protocol_version: 1, files })}\n`);
