<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Status from '#lib/Status.svelte';
	import { duration, retryRun } from '#lib/runs.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let run = $derived(data.run);
	let error = $state('');

	async function retry() {
		try {
			await retryRun(run.id);
			goto(resolve('/runs'));
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}
</script>

<p><a href={resolve('/runs')}>← Runs</a></p>
<h1>Run {run.id} <Status status={run.status} /></h1>

<dl>
	<dt>Flow</dt>
	<dd>{run.flowId}</dd>
	<dt>Event</dt>
	<dd>{run.eventType}</dd>
	{#if run.assetId}
		<dt>Asset</dt>
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<dd><a href="/photos/{run.assetId}" data-sveltekit-reload>{run.assetId}</a></dd>
	{/if}
	<dt>Started</dt>
	<dd>{new Date(run.startedAt).toLocaleString()} ({duration(run)})</dd>
	{#if run.error}
		<dt>Error</dt>
		<dd class="error">{run.error}</dd>
	{/if}
</dl>

{#if run.status !== 'running'}
	<button onclick={retry}>Run again</button>
{/if}
{#if error}<p role="alert">{error}</p>{/if}

<h2>Steps</h2>
<ol>
	{#each run.steps ?? [] as step (step.seq)}
		<li>
			<strong>{step.stepId ?? step.use}</strong>
			{#if step.stepId}<code>{step.use}</code>{/if}
			<Status status={step.status} />
			<small>{step.durationMs} ms</small>
			{#if step.error}<p class="error">{step.error}</p>{/if}
			{#if step.output}<pre>{JSON.stringify(step.output, null, 2)}</pre>{/if}
		</li>
	{/each}
</ol>

<style>
	dl {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 0.25rem 1rem;
	}
	dt {
		font-weight: 600;
	}
	.error {
		color: #cf222e;
		white-space: pre-wrap;
	}
	pre {
		background: color-mix(in srgb, currentColor 6%, transparent);
		padding: 0.5rem;
		overflow-x: auto;
	}
	li {
		margin-bottom: 0.75rem;
	}
</style>
