<script lang="ts">
	import { invalidate } from '$app/navigation';
	import {
		bulkTags,
		filterTree,
		kind,
		moveTag,
		tree,
		type Bulk,
		type Move,
		type Plan,
		type TagNode
	} from '#lib/tags.ts';
	import type { PageProps } from './$types';

	type Op = { type: 'move'; input: Move } | { type: 'bulk'; input: Bulk };

	let { data }: PageProps = $props();
	let query = $state('');
	let nodes = $derived(filterTree(tree(data.tags), query));

	let move = $state<Move>({ from: '', to: '' });
	let bulk = $state<Bulk>({ match: '', replace: '' });

	// The previewed operation; confirming applies exactly this input.
	let op = $state<Op>();
	let plans = $state<Plan[]>([]);
	let done = $state<Plan[]>();
	let busy = $state(false);
	let error = $state('');

	async function run(next: Op, dryRun: boolean) {
		busy = true;
		error = '';
		try {
			const res =
				next.type === 'move'
					? [await moveTag(next.input, dryRun)]
					: ((await bulkTags(next.input, dryRun)).moves ?? []);
			if (dryRun) {
				op = next;
				plans = res;
				done = undefined;
			} else {
				op = undefined;
				plans = [];
				done = res;
				await invalidate('app:tags');
			}
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}

	function pick(n: TagNode) {
		move = { from: n.value, to: n.value };
	}
</script>

{#snippet branch(list: TagNode[])}
	<ul>
		{#each list as n (n.value)}
			<li>
				<button
					class="tag"
					class:missing={!n.tag}
					class:selected={n.value === move.from}
					onclick={() => pick(n)}
				>
					{#if n.tag?.color}<span class="dot" style:background={n.tag.color}></span>{/if}
					{n.name}
				</button>
				{#if n.children.length}{@render branch(n.children)}{/if}
			</li>
		{/each}
	</ul>
{/snippet}

<h1>Tags</h1>

<div class="cols">
	<section>
		<input type="search" placeholder="Filter" bind:value={query} />
		{#if nodes.length}
			{@render branch(nodes)}
		{:else}
			<p>No tags.</p>
		{/if}
	</section>

	<section>
		<h2>Rename, move or merge</h2>
		<form
			onsubmit={(e) => {
				e.preventDefault();
				run({ type: 'move', input: { ...move } }, true);
			}}
		>
			<label>From <input bind:value={move.from} required placeholder="XX" /></label>
			<label>To <input bind:value={move.to} required placeholder="YY/XX" /></label>
			<button disabled={busy}>Preview</button>
		</form>

		<h2>Bulk</h2>
		<form
			onsubmit={(e) => {
				e.preventDefault();
				run({ type: 'bulk', input: { ...bulk } }, true);
			}}
		>
			<label
				>Match (regex) <input bind:value={bulk.match} required placeholder="^service-a/" /></label
			>
			<label>Replace <input bind:value={bulk.replace} placeholder="services/a/" /></label>
			<button disabled={busy}>Preview</button>
		</form>

		{#if error}<p role="alert" class="error">{error}</p>{/if}

		{#if op}
			<h2>Preview</h2>
			{#if plans.length === 0}
				<p>No matching tags.</p>
			{:else}
				{@render planTable(plans)}
				<p>
					<button disabled={busy} onclick={() => op && run(op, false)}>
						{busy ? 'Applying…' : 'Apply'}
					</button>
					<button disabled={busy} onclick={() => (op = undefined)}>Cancel</button>
				</p>
			{/if}
		{/if}

		{#if done}
			<h2>Done</h2>
			{@render planTable(done)}
			{#if done.some((p) => p.settled === false)}
				<p class="error">
					Immich brought some source tags back from sidecar files. Run the same operation again
					later.
				</p>
			{/if}
		{/if}
	</section>
</div>

{#snippet planTable(list: Plan[])}
	<table>
		<thead><tr><th>From</th><th>To</th><th></th><th>Assets</th></tr></thead>
		<tbody>
			{#each list as p (p.move.from)}
				{#each p.changes ?? [] as c (c.from)}
					<tr>
						<td>{c.from}</td>
						<td>{c.to}</td>
						<td>{kind(c)}</td>
						<td>{c.assets}</td>
					</tr>
				{/each}
			{/each}
		</tbody>
	</table>
	<p>{list.reduce((n, p) => n + p.assets, 0)} assets affected.</p>
{/snippet}

<style>
	.cols {
		display: grid;
		grid-template-columns: minmax(14rem, 1fr) 2fr;
		gap: 2rem;
	}
	@media (max-width: 48rem) {
		.cols {
			grid-template-columns: 1fr;
		}
	}
	ul {
		list-style: none;
		margin: 0;
		padding-left: 1rem;
	}
	section > ul {
		padding-left: 0;
	}
	.tag {
		background: none;
		border: 0;
		padding: 0.1rem 0.3rem;
		color: inherit;
		font: inherit;
		cursor: pointer;
		border-radius: 0.25rem;
	}
	.tag.missing {
		opacity: 0.6;
	}
	.tag.selected {
		background: color-mix(in srgb, currentColor 15%, transparent);
	}
	.dot {
		display: inline-block;
		width: 0.6rem;
		height: 0.6rem;
		border-radius: 50%;
	}
	form {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: end;
	}
	label {
		display: flex;
		flex-direction: column;
		font-size: 0.875rem;
	}
	input[type='search'] {
		width: 100%;
		box-sizing: border-box;
		margin-bottom: 0.5rem;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		text-align: left;
		padding: 0.35rem 0.5rem;
		border-bottom: 1px solid color-mix(in srgb, currentColor 12%, transparent);
	}
	.error {
		color: #cf222e;
	}
</style>
