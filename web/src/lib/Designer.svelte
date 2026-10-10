<script lang="ts">
	import {
		Background,
		Controls,
		SvelteFlow,
		type Edge,
		type Node,
		type NodeTypes
	} from '@xyflow/svelte';
	import '@xyflow/svelte/dist/style.css';
	import { untrack } from 'svelte';
	import FlowNode from './FlowNode.svelte';
	import {
		data,
		insertStep,
		key,
		move,
		parse,
		removeAt,
		set,
		setPosition,
		stepAt,
		text,
		type Path,
		type StepData
	} from './flowdoc';
	import { checkFlow, offset, type Problem } from './flows';
	import { graph, type Kind } from './flowgraph';
	import { stepTypes, triggers, withSchema } from './stepschema';
	import WithForm from './WithForm.svelte';
	import type { Schema } from './yamlschema';

	type Props = { value: string; source: string; schema: Schema };
	let { value = $bindable(), source, schema }: Props = $props();

	// The document is parsed once; edits go to it and are written back to value.
	const doc = parse(untrack(() => value));
	const parseError = doc.errors[0]?.message;

	const nodeTypes: NodeTypes = { flow: FlowNode };
	let nodes = $state.raw<Node[]>([]);
	let edges = $state.raw<Edge[]>([]);
	let selected = $state<{ id: string; kind: Kind; path: Path }>();
	let flow = $state(data(doc));
	let errors = $state<Record<string, string>>({});
	let other = $state<Problem[]>([]);

	let steps = $derived(stepTypes(schema));

	function rebuild() {
		flow = data(doc);
		const g = graph(flow);
		nodes = g.nodes.map((n) => ({
			id: n.id,
			type: 'flow',
			position: n.position,
			selected: n.id === selected?.id,
			data: { kind: n.kind, label: n.label, detail: n.detail, error: errors[n.id], path: n.path }
		}));
		edges = g.edges.map((e) => ({ ...e, type: 'smoothstep' }));
		if (selected && !g.nodes.some((n) => n.id === selected!.id)) selected = undefined;
	}

	let timer: ReturnType<typeof setTimeout> | undefined;
	function validate() {
		clearTimeout(timer);
		timer = setTimeout(async () => {
			const text = value;
			try {
				const problems = await checkFlow(source || 'new.yaml', text);
				const at = parse(text);
				const byNode: Record<string, string> = {};
				const rest: Problem[] = [];
				for (const p of problems) {
					const k = stepAt(at, offset(text, p.line, p.col));
					if (k) byNode[k] = byNode[k] ? `${byNode[k]}\n${p.message}` : p.message;
					else rest.push(p);
				}
				errors = byNode;
				other = rest;
			} catch (e) {
				other = [{ line: 0, col: 0, message: e instanceof Error ? e.message : String(e) }];
			}
			rebuild();
		}, 500);
	}

	function commit(select?: Path) {
		value = text(doc);
		if (select) {
			const id = key(select);
			selected = { id, kind: 'step', path: select };
		}
		rebuild();
		if (selected) {
			const n = nodes.find((x) => x.id === selected!.id);
			if (n) selected = { id: n.id, kind: n.data.kind as Kind, path: n.data.path as Path };
		}
		validate();
	}

	if (!parseError) {
		rebuild();
		validate();
	}

	function at(path: Path): StepData | undefined {
		let v: unknown = flow;
		for (const p of path) v = (v as Record<string | number, unknown> | undefined)?.[p];
		return v as StepData | undefined;
	}

	// Where a new step goes: after the selected step, first in the selected case, or at the end.
	function insertionPoint(): { list: Path; index: number } {
		const s = selected;
		if (s?.kind === 'step' || s?.kind === 'switch') {
			return { list: s.path.slice(0, -1), index: (s.path.at(-1) as number) + 1 };
		}
		if (s?.kind === 'case') return { list: [...s.path, 'steps'], index: 0 };
		if (s?.kind === 'trigger') return { list: ['steps'], index: 0 };
		return { list: ['steps'], index: flow.steps?.length ?? 0 };
	}

	// A case needs at least one step to be valid.
	const newCase = () => ({
		if: 'true',
		steps: [{ use: 'log', with: { message: 'replace me' } }]
	});

	function add(use: string) {
		const { list, index } = insertionPoint();
		const step: StepData = use === 'switch' ? { use, cases: [newCase()] } : { use };
		insertStep(doc, list, index, step);
		commit([...list, index]);
	}

	function addCase(path: Path) {
		const cases = at(path)?.cases ?? [];
		// Keep a trailing default case last.
		const index = cases.length && !cases.at(-1)?.if ? cases.length - 1 : cases.length;
		insertStep(doc, [...path, 'cases'], index, newCase() as StepData);
		commit();
	}

	function shift(path: Path, delta: number) {
		move(doc, path, delta);
		const next = [...path.slice(0, -1), (path.at(-1) as number) + delta];
		selected = { id: key(next), kind: selected!.kind, path: next };
		commit();
	}

	function remove(path: Path) {
		removeAt(doc, path);
		selected = undefined;
		commit();
	}

	const field = (path: Path, k: string, v: unknown) => {
		set(doc, [...path, k], v);
		commit();
	};

	function withChange(path: Path, k: string | undefined, v: unknown) {
		set(doc, k === undefined ? [...path, 'with'] : [...path, 'with', k], v);
		const w = at(path)?.with;
		if (w && Object.keys(w).length === 0) set(doc, [...path, 'with'], undefined);
		commit();
	}

	function dragged({ nodes: moved }: { nodes: Node[] }) {
		for (const n of moved) setPosition(doc, n.id, n.position);
		value = text(doc);
		flow = data(doc);
	}

	function resetLayout() {
		set(doc, ['ui', 'positions'], undefined);
		const ui = flow.ui;
		if (ui && Object.keys(ui).length <= 1) set(doc, ['ui'], undefined);
		commit();
	}

	const len = (path: Path) => {
		const list = at(path.slice(0, -1)) as unknown;
		return Array.isArray(list) ? list.length : 0;
	};
</script>

{#if parseError}
	<p class="error">The YAML cannot be parsed, fix it in the YAML tab first: {parseError}</p>
{:else}
	<div class="designer">
		<aside class="palette">
			<h3>Add step</h3>
			{#each steps as s (s.value)}
				<button title={s.description} onclick={() => add(s.value)}>{s.value}</button>
			{/each}
			<p><small>Inserted after the selected node.</small></p>
			<button onclick={resetLayout}>Reset layout</button>
		</aside>

		<div class="canvas">
			<SvelteFlow
				bind:nodes
				bind:edges
				{nodeTypes}
				fitView
				colorMode="system"
				nodesConnectable={false}
				deleteKey={null}
				onnodeclick={({ node }) =>
					(selected = {
						id: node.id,
						kind: node.data.kind as Kind,
						path: node.data.path as Path
					})}
				onpaneclick={() => (selected = undefined)}
				onnodedragstop={dragged}
			>
				<Background />
				<Controls showLock={false} />
			</SvelteFlow>
		</div>

		<aside class="inspector">
			{#if !selected}
				<p><small>Select a node to edit it.</small></p>
			{:else if selected.kind === 'trigger'}
				{@const t = flow.trigger ?? {}}
				<h3>Trigger</h3>
				<label>
					on
					<select
						value={t.on ?? ''}
						onchange={(e) => field(['trigger'], 'on', e.currentTarget.value)}
					>
						{#each triggers(schema) as c (c.value)}<option>{c.value}</option>{/each}
					</select>
				</label>
				<label>
					if
					<input
						value={t.if ?? ''}
						onchange={(e) => field(['trigger'], 'if', e.currentTarget.value)}
					/>
				</label>
				{#if t.on === 'schedule'}
					<label>
						cron <small>(minute hour day month weekday)</small>
						<input
							value={t.cron ?? ''}
							placeholder="0 3 * * *"
							onchange={(e) => field(['trigger'], 'cron', e.currentTarget.value)}
						/>
					</label>
				{/if}
			{:else if selected.kind === 'case'}
				{@const path = selected.path}
				{@const c = at(path) as { if?: string } | undefined}
				<h3>Case</h3>
				<label>
					if <small>(empty: default, must be last)</small>
					<input value={c?.if ?? ''} onchange={(e) => field(path, 'if', e.currentTarget.value)} />
				</label>
				<p class="actions">
					<button disabled={path.at(-1) === 0} onclick={() => shift(path, -1)}>←</button>
					<button disabled={(path.at(-1) as number) >= len(path) - 1} onclick={() => shift(path, 1)}
						>→</button
					>
					<button onclick={() => remove(path)}>Delete case</button>
				</p>
			{:else}
				{@const path = selected.path}
				{@const s = at(path) ?? {}}
				<h3>{s.use}</h3>
				{#if errors[selected.id]}<pre class="error">{errors[selected.id]}</pre>{/if}
				<label>
					use
					<select value={s.use ?? ''} onchange={(e) => field(path, 'use', e.currentTarget.value)}>
						{#each steps as c (c.value)}<option>{c.value}</option>{/each}
					</select>
				</label>
				<label
					>id <input
						value={s.id ?? ''}
						onchange={(e) => field(path, 'id', e.currentTarget.value)}
					/></label
				>
				<label
					>name <input
						value={s.name ?? ''}
						onchange={(e) => field(path, 'name', e.currentTarget.value)}
					/></label
				>
				<label
					>if <input
						value={s.if ?? ''}
						onchange={(e) => field(path, 'if', e.currentTarget.value)}
					/></label
				>
				<label class="inline">
					<input
						type="checkbox"
						checked={s.continueOnError === true}
						onchange={(e) => field(path, 'continueOnError', e.currentTarget.checked || undefined)}
					/>
					continueOnError
				</label>
				{#if s.use === 'switch'}
					<button onclick={() => addCase(path)}>Add case</button>
				{:else}
					<h4>with</h4>
					{#key selected.id + s.use}
						<WithForm
							schema={withSchema(schema, s.use)}
							value={s.with}
							onchange={(k, v) => withChange(path, k, v)}
						/>
					{/key}
				{/if}
				<p class="actions">
					<button disabled={path.at(-1) === 0} onclick={() => shift(path, -1)}>↑</button>
					<button disabled={(path.at(-1) as number) >= len(path) - 1} onclick={() => shift(path, 1)}
						>↓</button
					>
					<button onclick={() => remove(path)}>Delete</button>
				</p>
			{/if}
		</aside>
	</div>
	{#if other.length}
		<pre class="error">{other
				.map((p) => (p.line ? `${p.line}:${p.col}: ` : '') + p.message)
				.join('\n')}</pre>
	{/if}
{/if}

<style>
	.designer {
		display: grid;
		grid-template-columns: 9rem 1fr 18rem;
		gap: 0.75rem;
		height: 70vh;
		min-height: 28rem;
	}
	@media (max-width: 60rem) {
		.designer {
			grid-template-columns: 1fr;
			height: auto;
		}
		.canvas {
			height: 60vh;
		}
	}
	.canvas {
		border: 1px solid color-mix(in srgb, currentColor 20%, transparent);
		border-radius: 0.25rem;
		overflow: hidden;
	}
	aside {
		overflow-y: auto;
		font-size: 0.85rem;
	}
	.palette {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.palette button {
		text-align: left;
		font-family: ui-monospace, monospace;
	}
	h3 {
		margin: 0 0 0.5rem;
		font-size: 1rem;
	}
	h4 {
		margin: 0.75rem 0 0.4rem;
	}
	.inspector label {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		margin-bottom: 0.5rem;
	}
	.inspector label.inline {
		flex-direction: row;
		align-items: center;
		gap: 0.4rem;
	}
	.inspector input:not([type='checkbox']),
	.inspector select {
		font-family: ui-monospace, monospace;
	}
	.actions {
		display: flex;
		gap: 0.4rem;
	}
	.error {
		color: #cf222e;
		white-space: pre-wrap;
		font-size: 0.85rem;
	}
</style>
