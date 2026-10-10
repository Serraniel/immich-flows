<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { deleteFlow, readFlow, runOnAll, saveFlow, setEnabled, type Entry } from '#lib/flows.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let error = $state('');
	let notice = $state('');

	async function act(f: () => Promise<unknown>) {
		error = '';
		try {
			await f();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
		await invalidate('app:flows');
	}

	const edit = (source: string) => resolve('/flows/edit/[...source]', { source });

	async function exportFlow(e: Entry) {
		const blob = new Blob([await readFlow(e.source)], { type: 'application/yaml' });
		const a = document.createElement('a');
		a.href = URL.createObjectURL(blob);
		a.download = e.source.split('/').at(-1)!;
		a.click();
		URL.revokeObjectURL(a.href);
	}

	async function importFiles(input: HTMLInputElement) {
		const files = [...(input.files ?? [])];
		input.value = '';
		await act(async () => {
			for (const f of files) {
				if (data.entries.some((e) => e.source === f.name) && !confirm(`Replace ${f.name}?`)) {
					continue;
				}
				await saveFlow(f.name, await f.text());
			}
		});
	}

	function backfill(e: Entry) {
		const id = e.flow!.id;
		if (!confirm(`Run ${id} on all your assets now, regardless of its trigger?`)) return;
		notice = '';
		act(async () => {
			const r = await runOnAll(id);
			notice = `${id}: ${r.queued} runs queued. Progress: Runs page.`;
		});
	}

	function remove(e: Entry) {
		if (confirm(`Delete ${e.source}?`)) act(() => deleteFlow(e.source));
	}
</script>

<h1>Flows</h1>

{#if !data.user?.isAdmin}
	<p>Only Immich admins can manage flows.</p>
{:else}
	<p class="actions">
		<a href={edit('')}>New flow</a>
		<label class="import">
			Import
			<input
				type="file"
				accept=".yaml,.yml"
				multiple
				onchange={(e) => importFiles(e.currentTarget)}
			/>
		</label>
	</p>

	{#if error}<p role="alert" class="error">{error}</p>{/if}
	{#if notice}<p role="status">{notice}</p>{/if}

	{#if data.entries.length === 0}
		<p>No flow files.</p>
	{:else}
		<table>
			<thead>
				<tr><th>File</th><th>Flow</th><th>Trigger</th><th>Enabled</th><th></th></tr>
			</thead>
			<tbody>
				{#each data.entries as e (e.source)}
					<tr>
						<td><a href={edit(e.source)}>{e.source}</a></td>
						<td>
							{#if e.flow}{e.flow.name ?? e.flow.id} <small>{e.flow.id}</small>{/if}
							{#if e.error}<pre class="error">{e.error}</pre>{/if}
						</td>
						<td>{e.flow?.trigger.on ?? ''}</td>
						<td>
							{#if e.flow && !e.error}
								<input
									type="checkbox"
									aria-label="Enabled"
									checked={e.flow.enabled !== false}
									onchange={(ev) => act(() => setEnabled(e.source, ev.currentTarget.checked))}
								/>
							{/if}
						</td>
						<td class="row-actions">
							{#if e.flow && !e.error && e.flow.enabled !== false}
								<button title="Run on all your existing assets" onclick={() => backfill(e)}
									>Run on all</button
								>
							{/if}
							<button onclick={() => exportFlow(e)}>Export</button>
							<button onclick={() => remove(e)}>Delete</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
{/if}

<style>
	.actions {
		display: flex;
		gap: 1rem;
		align-items: center;
	}
	.import {
		cursor: pointer;
		text-decoration: underline;
	}
	.import input {
		display: none;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		text-align: left;
		vertical-align: top;
		padding: 0.35rem 0.5rem;
		border-bottom: 1px solid color-mix(in srgb, currentColor 12%, transparent);
	}
	.row-actions {
		white-space: nowrap;
	}
	.error {
		color: #cf222e;
		white-space: pre-wrap;
		margin: 0.25rem 0 0;
		font-size: 0.875rem;
	}
	small {
		opacity: 0.7;
	}
</style>
