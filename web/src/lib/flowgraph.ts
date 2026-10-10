import { key, type FlowData, type Path, type Pos, type StepData } from './flowdoc';

export type Kind = 'trigger' | 'step' | 'switch' | 'case';

export type GraphNode = {
	id: string; // node key: 'trigger' or the YAML path, e.g. steps.2.cases.0
	kind: Kind;
	path: Path;
	label: string;
	detail?: string;
	position: Pos;
};

export type GraphEdge = { id: string; source: string; target: string };

export const COL = 260;
export const ROW = 110;

// Width in columns of a list of steps.
function width(steps: StepData[] | undefined): number {
	return Math.max(1, ...(steps ?? []).map((s) => (s.use === 'switch' ? casesWidth(s) : 1)));
}
function casesWidth(s: StepData): number {
	const cases = s.cases ?? [];
	return Math.max(
		1,
		cases.reduce((n, c) => n + width(c.steps), 0)
	);
}

const stepLabel = (s: StepData) => s.name || s.id || s.use || '(step)';

// graph lays out a flow top to bottom. Switch cases branch side by side and join after.
// Saved positions (ui.positions) override the computed ones.
export function graph(flow: FlowData): { nodes: GraphNode[]; edges: GraphEdge[] } {
	const nodes: GraphNode[] = [];
	const edges: GraphEdge[] = [];
	const saved = flow.ui?.positions ?? {};
	const add = (n: Omit<GraphNode, 'position'>, auto: Pos, prev: string[]) => {
		const p = saved[n.id];
		const position = p && Number.isFinite(p.x) && Number.isFinite(p.y) ? p : auto;
		nodes.push({ ...n, position });
		for (const s of prev) edges.push({ id: `${s}->${n.id}`, source: s, target: n.id });
	};

	// walk places steps starting at column x, row y, connected from prev.
	// Returns the nodes that connect to whatever follows and the next free row.
	const walk = (
		steps: StepData[] | undefined,
		listPath: Path,
		x: number,
		y: number,
		prev: string[]
	): { prev: string[]; y: number } => {
		(steps ?? []).forEach((s, i) => {
			const path = [...listPath, i];
			const id = key(path);
			const isSwitch = s.use === 'switch';
			const span = isSwitch ? casesWidth(s) : 1;
			add(
				{
					id,
					kind: isSwitch ? 'switch' : 'step',
					path,
					label: stepLabel(s),
					detail: s.if ? `if ${s.if}` : s.use
				},
				{ x: (x + (span - 1) / 2) * COL, y: y * ROW },
				prev
			);
			y++;
			if (!isSwitch) {
				prev = [id];
				return;
			}
			const ends: string[] = [];
			let cx = x;
			let maxY = y;
			(s.cases ?? []).forEach((c, ci) => {
				const cpath = [...path, 'cases', ci];
				const cid = key(cpath);
				const w = width(c.steps);
				add(
					{ id: cid, kind: 'case', path: cpath, label: c.if ? `if ${c.if}` : 'default' },
					{ x: (cx + (w - 1) / 2) * COL, y: y * ROW },
					[id]
				);
				const r = walk(c.steps, [...cpath, 'steps'], cx, y + 1, [cid]);
				ends.push(...r.prev);
				maxY = Math.max(maxY, r.y);
				cx += w;
			});
			prev = ends.length ? ends : [id];
			y = maxY;
		});
		return { prev, y };
	};

	const total = width(flow.steps);
	add(
		{
			id: 'trigger',
			kind: 'trigger',
			path: ['trigger'],
			label: flow.trigger?.on ?? '(trigger)',
			detail: flow.trigger?.if ? `if ${flow.trigger.if}` : undefined
		},
		{ x: ((total - 1) / 2) * COL, y: 0 },
		[]
	);
	walk(flow.steps, ['steps'], 0, 1, ['trigger']);
	return { nodes, edges };
}
