<script lang="ts">
	import { resolve } from '$app/paths';
	import Status from '#lib/Status.svelte';
	import { duration, listRuns, type Run } from '#lib/runs.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let more = $state<Run[]>([]);
	let done = $state(false);
	let runs = $derived([...data.runs, ...more]);

	// New filter results replace pages loaded with "Load more".
	$effect(() => {
		void data.runs;
		more = [];
		done = false;
	});

	async function loadMore() {
		const last = runs.at(-1);
		if (!last) return;
		const next = await listRuns({ flow: data.flow, status: data.status, before: last.id });
		more = [...more, ...next];
		done = next.length === 0;
	}
</script>

<h1>Runs</h1>

<form method="GET" action={resolve('/runs')}>
	<input name="flow" placeholder="Flow ID" value={data.flow} />
	<select name="status" value={data.status}>
		<option value="">Any status</option>
		<option>success</option>
		<option>failed</option>
		<option>running</option>
	</select>
	<button>Filter</button>
</form>

{#if runs.length === 0}
	<p>No runs.</p>
{:else}
	<table>
		<thead>
			<tr
				><th>#</th><th>Flow</th><th>Event</th><th>Asset</th><th>Status</th><th>Started</th><th
					>Took</th
				></tr
			>
		</thead>
		<tbody>
			{#each runs as run (run.id)}
				<tr>
					<td><a href={resolve('/runs/[id]', { id: String(run.id) })}>{run.id}</a></td>
					<td>{run.flowId}</td>
					<td>{run.eventType}</td>
					<td>
						{#if run.assetId}
							<!-- Immich asset page, outside this app. -->
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
							<a href="/photos/{run.assetId}" data-sveltekit-reload>{run.assetId.slice(0, 8)}</a>
						{/if}
					</td>
					<td><Status status={run.status} /></td>
					<td>{new Date(run.startedAt).toLocaleString()}</td>
					<td>{duration(run)}</td>
				</tr>
			{/each}
		</tbody>
	</table>
	{#if !done && runs.length >= 50}
		<button onclick={loadMore}>Load more</button>
	{/if}
{/if}

<style>
	form {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1rem;
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
</style>
