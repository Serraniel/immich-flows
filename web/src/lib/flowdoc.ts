// Edits a flow YAML document in place, keeping comments and formatting of untouched parts.
import { isMap, isSeq, parseDocument, type Document, type YAMLMap, type YAMLSeq } from 'yaml';

export type Path = (string | number)[];
export type Pos = { x: number; y: number };

export type StepData = {
	id?: string;
	name?: string;
	use?: string;
	if?: string;
	with?: Record<string, unknown>;
	cases?: { if?: string; steps?: StepData[] }[];
	continueOnError?: boolean;
};

export type FlowData = {
	id?: string;
	name?: string;
	trigger?: { on?: string; if?: string; cron?: string };
	steps?: StepData[];
	ui?: { positions?: Record<string, Pos> };
};

export const key = (path: Path) => path.join('.');

export function parse(text: string): Document {
	return parseDocument(text, { keepSourceTokens: false });
}

// text serializes the document. Untouched parts keep comments and blank lines; comment
// column alignment is not preserved.
export function text(doc: Document): string {
	return doc.toString({ flowCollectionPadding: false });
}

export function data(doc: Document): FlowData {
	const v = doc.toJS();
	return v && typeof v === 'object' && !Array.isArray(v) ? v : {};
}

// set writes a value at path. Empty values ('' / undefined / empty list) remove the key.
export function set(doc: Document, path: Path, value: unknown) {
	const empty = value === undefined || value === '' || (Array.isArray(value) && value.length === 0);
	if (empty) {
		if (doc.hasIn(path)) doc.deleteIn(path);
		return;
	}
	// Keep the style of an existing collection, e.g. [a, b].
	const prev = doc.getIn(path, true) as { flow?: boolean } | undefined;
	const flow = (isSeq(prev) || isMap(prev)) && prev.flow;
	doc.setIn(
		path,
		flow && typeof value === 'object' ? doc.createNode(value, { flow: true }) : value
	);
}

function seqAt(doc: Document, path: Path): YAMLSeq {
	let seq = doc.getIn(path, true);
	if (!isSeq(seq)) {
		doc.setIn(path, doc.createNode([]));
		seq = doc.getIn(path, true);
	}
	return seq as YAMLSeq;
}

// remap shifts saved positions of items in the list at listPath after a structural change.
// shift returns the new index of an item or undefined if it was removed.
function remap(doc: Document, listPath: Path, shift: (i: number) => number | undefined) {
	const positions = doc.getIn(['ui', 'positions'], true);
	if (!isMap(positions)) return;
	const prefix = key(listPath) + '.';
	const moved: [string, unknown][] = [];
	for (const pair of [...(positions as YAMLMap).items]) {
		const k = String((pair.key as { value?: unknown })?.value ?? pair.key);
		if (!k.startsWith(prefix)) continue;
		const [idx, ...rest] = k.slice(prefix.length).split('.');
		const i = Number(idx);
		if (!Number.isInteger(i)) continue;
		const n = shift(i);
		if (n === i) continue;
		positions.delete(k);
		if (n !== undefined) moved.push([[prefix + n, ...rest].join('.'), pair.value]);
	}
	for (const [k, v] of moved) positions.set(k, v);
}

export function insertStep(doc: Document, listPath: Path, index: number, step: StepData) {
	const seq = seqAt(doc, listPath);
	index = Math.max(0, Math.min(index, seq.items.length));
	remap(doc, listPath, (i) => (i >= index ? i + 1 : i));
	seq.items.splice(index, 0, doc.createNode(step));
}

export function removeAt(doc: Document, path: Path) {
	const listPath = path.slice(0, -1);
	const index = path.at(-1) as number;
	const seq = doc.getIn(listPath, true);
	if (!isSeq(seq)) return;
	seq.items.splice(index, 1);
	remap(doc, listPath, (i) => (i === index ? undefined : i > index ? i - 1 : i));
}

export function move(doc: Document, path: Path, delta: number) {
	const listPath = path.slice(0, -1);
	const from = path.at(-1) as number;
	const seq = doc.getIn(listPath, true);
	if (!isSeq(seq)) return;
	const to = from + delta;
	if (to < 0 || to >= seq.items.length) return;
	const [item] = seq.items.splice(from, 1);
	seq.items.splice(to, 0, item);
	remap(doc, listPath, (i) => {
		if (i === from) return to;
		if (from < to && i > from && i <= to) return i - 1;
		if (to < from && i >= to && i < from) return i + 1;
		return i;
	});
}

export function setPosition(doc: Document, nodeKey: string, p: Pos) {
	const node = doc.createNode({ x: Math.round(p.x), y: Math.round(p.y) }, { flow: true });
	doc.setIn(['ui', 'positions', nodeKey], node);
}

// stepAt returns the key of the innermost step or trigger containing offset, for mapping
// validation problems onto nodes.
export function stepAt(doc: Document, offset: number): string | undefined {
	let found: string | undefined;
	const inside = (n: unknown) => {
		const r = (n as { range?: [number, number, number] })?.range;
		return !!r && offset >= r[0] && offset < r[2];
	};
	if (inside(doc.getIn(['trigger'], true))) return 'trigger';
	const walk = (listPath: Path) => {
		const seq = doc.getIn(listPath, true);
		if (!isSeq(seq)) return;
		seq.items.forEach((item, i) => {
			if (!inside(item)) return;
			const p = [...listPath, i];
			found = key(p);
			const cases = doc.getIn([...p, 'cases'], true);
			if (isSeq(cases)) cases.items.forEach((_, c) => walk([...p, 'cases', c, 'steps']));
		});
	};
	walk(['steps']);
	return found;
}
