import { api, apiUrl, ApiError } from './api';
import type { Schema } from './yamlschema';

export type Flow = {
	id: string;
	name?: string;
	description?: string;
	enabled?: boolean;
	trigger: { on: string; if?: string };
};

export type Entry = { source: string; flow?: Flow; error?: string };
export type Problem = { line: number; col: number; message: string };

export class InvalidFlow extends Error {
	constructor(readonly problems: Problem[]) {
		super(problems.map((p) => `${p.line}:${p.col}: ${p.message}`).join('\n'));
	}
}

const file = (source: string) =>
	`flow-files/${source.split('/').map(encodeURIComponent).join('/')}`;

export const listFlows = (f: typeof fetch = fetch) => api<Entry[]>('flows', undefined, f);
export const flowSchema = (f: typeof fetch = fetch) => api<Schema>('flows/schema', undefined, f);

export async function readFlow(source: string, f: typeof fetch = fetch): Promise<string> {
	const res = await f(apiUrl(file(source)), { credentials: 'same-origin' });
	if (!res.ok) throw new ApiError(res.status, (await res.text()) || res.statusText);
	return res.text();
}

async function put(source: string, yaml: string, dryRun: boolean): Promise<Response> {
	return fetch(apiUrl(file(source)) + (dryRun ? '?dryRun=true' : ''), {
		method: 'PUT',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/yaml' },
		body: yaml
	});
}

// saveFlow writes a flow file. Validation problems throw InvalidFlow.
export async function saveFlow(source: string, yaml: string): Promise<void> {
	const res = await put(source, yaml, false);
	if (res.ok) return;
	const body = await res.text();
	if (res.status === 422) throw new InvalidFlow(JSON.parse(body).problems);
	throw new ApiError(res.status, body || res.statusText);
}

export async function checkFlow(source: string, yaml: string): Promise<Problem[]> {
	const res = await put(source, yaml, true);
	if (!res.ok) throw new ApiError(res.status, (await res.text()) || res.statusText);
	return (await res.json()).problems;
}

export const setEnabled = (source: string, enabled: boolean) =>
	api<void>(file(source), { method: 'PATCH', body: JSON.stringify({ enabled }) });
export const deleteFlow = (source: string) => api<void>(file(source), { method: 'DELETE' });

// offset converts a 1-based line and column to an offset in text, clamped to its length.
export function offset(text: string, line: number, col: number): number {
	let at = 0;
	for (let l = 1; l < line; l++) {
		const nl = text.indexOf('\n', at);
		if (nl < 0) return text.length;
		at = nl + 1;
	}
	const end = text.indexOf('\n', at);
	return Math.min(at + Math.max(col - 1, 0), end < 0 ? text.length : end);
}

export const template = `id: my-flow
name: My flow
trigger:
  on: manual
steps:
  - use: log
    with:
      message: 'Asset {{ asset.id }}'
`;
