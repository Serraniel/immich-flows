// Schema-driven completion for block-style YAML, resolved from indentation.
// Flow-style ({…}, […]) and multi-line scalars are not understood; completion is best effort.

export type Schema = {
	$ref?: string;
	$defs?: Record<string, Schema>;
	type?: string | string[];
	description?: string;
	properties?: Record<string, Schema>;
	items?: Schema;
	enum?: unknown[];
	const?: unknown;
	anyOf?: Schema[];
	oneOf?: Schema[];
	allOf?: Schema[];
	if?: Schema;
	then?: Schema;
};

export type Option = { label: string; detail?: string; info?: string; apply: string };

export type Context = {
	kind: 'key' | 'value';
	key?: string; // for values
	from: number; // offset in the current line where the completed word starts
	options: Option[];
};

type Line = { indent: number; dash: number | null; key?: string; value?: string; blank: boolean };

function parseLine(s: string): Line {
	const m = /^(\s*)((?:-\s+)*)(.*)$/.exec(s)!;
	const rest = m[3].replace(/\s+#.*$/, '');
	if (rest === '' || rest.startsWith('#')) {
		return { indent: m[1].length + m[2].length, dash: m[2] ? m[1].length : null, blank: !m[2] };
	}
	const kv = /^([^\s:#'"][^:#]*?|'[^']*'|"[^"]*"):(?:\s+(.*))?$/.exec(rest);
	return {
		indent: m[1].length + m[2].length,
		dash: m[2] ? m[1].length : null,
		key: kv ? kv[1].replace(/^['"]|['"]$/g, '') : undefined,
		value: kv ? (kv[2] ?? '').trim().replace(/^['"]|['"]$/g, '') : undefined,
		blank: false
	};
}

type Mapping = { siblings: Record<string, string>; item: boolean; dashCol: number; start: number };

// mappingAt collects the keys of the block mapping containing line idx at indent ki.
function mappingAt(lines: Line[], idx: number, ki: number, skip = -1): Mapping {
	const siblings: Record<string, string> = {};
	const add = (j: number) => {
		const l = lines[j];
		if (j !== skip && l.indent === ki && l.key) siblings[l.key] = l.value ?? '';
	};
	let start = 0;
	let item = false;
	let dashCol = 0;
	for (let j = idx; j >= 0; j--) {
		const l = lines[j];
		if (l.blank) continue;
		if (l.indent < ki) {
			start = j + 1;
			break;
		}
		if (l.indent === ki) {
			add(j);
			if (l.dash !== null) {
				start = j;
				item = true;
				dashCol = l.dash;
				break;
			}
		}
	}
	for (let j = idx + 1; j < lines.length; j++) {
		const l = lines[j];
		if (l.blank) continue;
		if (l.indent < ki || (l.indent === ki && l.dash !== null)) break;
		add(j);
	}
	return { siblings, item, dashCol, start };
}

type Frame = { key?: string; item: boolean; siblings: Record<string, string> };

// frames returns the mappings from the document root to the one containing line idx.
function frames(lines: Line[], idx: number, ki: number): Frame[] {
	const out: Frame[] = [];
	let skip = idx;
	for (;;) {
		const m = mappingAt(lines, idx, ki, skip);
		skip = -1;
		const limit = m.item ? m.dashCol : ki - 1;
		let parent = -1;
		for (let j = m.start - 1; j >= 0; j--) {
			const l = lines[j];
			if (!l.blank && l.indent <= limit) {
				parent = l.key !== undefined && l.value === '' ? j : -2;
				break;
			}
		}
		out.unshift({
			key: parent >= 0 ? lines[parent].key : undefined,
			item: m.item,
			siblings: m.siblings
		});
		if (parent < 0) return parent === -1 ? out : [];
		idx = parent;
		ki = lines[parent].indent;
	}
}

function deref(root: Schema, s: Schema | undefined): Schema | undefined {
	for (let i = 0; s?.$ref && i < 16; i++) {
		const name = s.$ref.replace(/^#\/\$defs\//, '');
		s = root.$defs?.[name];
	}
	return s;
}

// matches reports whether siblings satisfy a schema's const properties (for if/then).
function matches(cond: Schema, siblings: Record<string, string>): boolean {
	return Object.entries(cond.properties ?? {}).every(
		([k, v]) => v.const === undefined || siblings[k] === String(v.const)
	);
}

function properties(
	root: Schema,
	s: Schema | undefined,
	siblings: Record<string, string>
): Record<string, Schema> {
	s = deref(root, s);
	const props: Record<string, Schema> = { ...s?.properties };
	for (const c of s?.allOf ?? []) {
		if (c.if && !matches(c.if, siblings)) continue;
		const add = c.if ? c.then : c;
		for (const [k, v] of Object.entries(add?.properties ?? {})) props[k] = v;
	}
	return props;
}

function valueOptions(root: Schema, s: Schema | undefined): Option[] {
	s = deref(root, s);
	if (!s) return [];
	const out: Option[] = (s.enum ?? []).map((v) => ({ label: String(v), apply: String(v) }));
	for (const alt of [...(s.anyOf ?? []), ...(s.oneOf ?? [])]) {
		if (alt.const !== undefined) {
			out.push({ label: String(alt.const), info: alt.description, apply: String(alt.const) });
		}
	}
	if (s.type === 'boolean') out.push(...['true', 'false'].map((v) => ({ label: v, apply: v })));
	return out;
}

function typeName(s: Schema | undefined): string | undefined {
	if (!s) return undefined;
	if (s.$ref) return s.$ref.replace(/^#\/\$defs\//, '');
	return Array.isArray(s.type) ? s.type.join(' | ') : s.type;
}

// complete returns completion options at offset pos in text.
export function complete(root: Schema, text: string, pos: number): Context | null {
	const lineStart = text.lastIndexOf('\n', pos - 1) + 1;
	let lineEnd = text.indexOf('\n', pos);
	if (lineEnd < 0) lineEnd = text.length;
	const cur = text.slice(lineStart, pos);
	const before = text.slice(0, lineStart).split('\n').slice(0, -1);
	const after = text.slice(lineEnd + 1).split('\n');
	if (lineEnd === text.length) after.length = 0;

	const m = /^(\s*)((?:-\s+)*)(.*)$/.exec(cur)!;
	const rest = m[3];
	const kv = /^([\w.-]+):\s+([^\s#'"]*)$/.exec(rest);
	const isKey = /^[\w.-]*$/.test(rest);
	if (!kv && !isKey) return null;

	const curLine: Line = {
		indent: m[1].length + m[2].length,
		dash: m[2] ? m[1].length : null,
		key: kv ? kv[1] : undefined,
		value: kv ? kv[2] : undefined,
		blank: false
	};
	const lines = [...before.map(parseLine), curLine, ...after.map(parseLine)];
	const fs = frames(lines, before.length, curLine.indent);
	if (fs.length === 0) return null;

	// Walk from the root schema to the mapping at the cursor.
	let node: Schema | undefined = root;
	let props = properties(root, node, fs[0].siblings);
	for (let i = 1; i < fs.length; i++) {
		node = deref(root, props[fs[i].key!]);
		if (fs[i].item) node = deref(root, node?.items);
		props = properties(root, node, fs[i].siblings);
	}
	if (fs.length === 1 && fs[0].item) return null; // root is not a sequence

	if (kv) {
		return {
			kind: 'value',
			key: kv[1],
			from: cur.length - kv[2].length,
			options: valueOptions(root, props[kv[1]])
		};
	}
	const used = fs.at(-1)!.siblings;
	const options = Object.entries(props)
		.filter(([k]) => !(k in used))
		.map(([k, v]) => {
			const t = deref(root, v);
			const block = t?.type === 'object' || t?.type === 'array' || !!t?.properties;
			return {
				label: k,
				detail: typeName(v),
				info: v.description ?? t?.description,
				apply: block ? `${k}:` : `${k}: `
			};
		});
	return { kind: 'key', from: cur.length - rest.length, options };
}
