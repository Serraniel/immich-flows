import type { Schema } from './yamlschema';

export type Choice = { value: string; description?: string };

export type Field =
	| { kind: 'select'; choices: Choice[] }
	| { kind: 'text' }
	| { kind: 'number' }
	| { kind: 'boolean' }
	| { kind: 'checks'; choices: Choice[] }
	| { kind: 'list'; allowString: boolean } // one item per line
	| { kind: 'yaml' };

const defs = (s: Schema) => s.$defs ?? {};

export function stepTypes(flow: Schema): Choice[] {
	return (defs(flow).Step?.properties?.use?.anyOf ?? [])
		.filter((a) => a.const !== undefined)
		.map((a) => ({ value: String(a.const), description: a.description }));
}

export function triggers(flow: Schema): Choice[] {
	return (defs(flow).Trigger?.properties?.on?.enum ?? []).map((v) => ({ value: String(v) }));
}

// withSchema returns the schema of a step's with: block.
export function withSchema(flow: Schema, use: string | undefined): Schema | undefined {
	for (const c of defs(flow).Step?.allOf ?? []) {
		if (c.if?.properties?.use?.const === use) return c.then?.properties?.with;
	}
	return undefined;
}

function choices(s: Schema): Choice[] | undefined {
	const consts = (s.anyOf ?? []).filter((a) => a.const !== undefined);
	if (s.enum) return s.enum.map((v) => ({ value: String(v) }));
	if (consts.length)
		return consts.map((a) => ({ value: String(a.const), description: a.description }));
	return undefined;
}

export function field(s: Schema): Field {
	const c = choices(s);
	if (c) return { kind: 'select', choices: c };
	const alts = [...(s.oneOf ?? []), ...(s.anyOf ?? [])];
	const types = alts.length ? alts.map((a) => a.type) : [s.type];
	if (types.includes('array')) {
		const items = (alts.find((a) => a.type === 'array') ?? s).items;
		const ic = items && choices(items);
		if (ic && !types.includes('string')) return { kind: 'checks', choices: ic };
		return { kind: 'list', allowString: types.includes('string') };
	}
	switch (s.type) {
		case 'string':
			return { kind: 'text' };
		case 'integer':
		case 'number':
			return { kind: 'number' };
		case 'boolean':
			return { kind: 'boolean' };
	}
	return { kind: 'yaml' };
}

// listText shows a list (or a single template string) one item per line.
export function listText(v: unknown): string {
	if (Array.isArray(v)) return v.map(String).join('\n');
	return v === undefined || v === null ? '' : String(v);
}

// parseList reads one item per line. A single line is kept as a string if the field accepts
// strings or the line is a template that yields a list.
export function parseList(text: string, allowString: boolean): string | string[] | undefined {
	const items = text
		.split('\n')
		.map((l) => l.trim())
		.filter(Boolean);
	if (items.length === 0) return undefined;
	if (items.length === 1 && (allowString || items[0].includes('{{'))) return items[0];
	return items;
}
