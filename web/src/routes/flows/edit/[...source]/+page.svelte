<script lang="ts">
	import { beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import FlowEditor from '#lib/FlowEditor.svelte';
	import { InvalidFlow, saveFlow } from '#lib/flows.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// Reset when navigating to another file; the editor is keyed on data.source.
	let yaml = $derived(data.yaml);
	let saved = $derived(data.yaml);
	let name = $derived(data.source || 'my-flow.yaml');
	let error = $state('');
	let mode = $state<'yaml' | 'visual'>('yaml');
	let busy = $state(false);
	let dirty = $derived(yaml !== saved);

	async function save() {
		if (busy) return;
		busy = true;
		error = '';
		try {
			await saveFlow(name, yaml);
			saved = yaml;
			if (name !== data.source) {
				await goto(resolve('/flows/edit/[...source]', { source: name }), { replaceState: true });
			}
		} catch (e) {
			error =
				e instanceof InvalidFlow
					? `Not saved, the flow is invalid:\n${e.message}`
					: e instanceof Error
						? e.message
						: String(e);
		} finally {
			busy = false;
		}
	}

	beforeNavigate((nav) => {
		if (dirty && !confirm('Discard unsaved changes?')) nav.cancel();
	});
</script>

<p><a href={resolve('/flows')}>← Flows</a></p>

{#if !data.user?.isAdmin}
	<p>Only Immich admins can manage flows.</p>
{:else}
	<div class="bar">
		{#if data.source}
			<h1>{data.source}</h1>
		{:else}
			<label>File <input bind:value={name} required /></label>
		{/if}
		<button onclick={save} disabled={busy || (!dirty && !!data.source)}>
			{busy ? 'Saving…' : 'Save'}
		</button>
		{#if dirty}<small>Unsaved changes</small>{/if}
	</div>

	{#if error}<pre role="alert" class="error">{error}</pre>{/if}

	<div class="tabs" role="tablist">
		<button role="tab" aria-selected={mode === 'yaml'} onclick={() => (mode = 'yaml')}>YAML</button>
		<button role="tab" aria-selected={mode === 'visual'} onclick={() => (mode = 'visual')}>
			Visual
		</button>
	</div>

	{#key data.source + mode}
		{#if mode === 'yaml'}
			<FlowEditor bind:value={yaml} source={name} schema={data.schema} onsave={save} />
			<p>
				<small
					>Ctrl+Space completes keys and values. Problems are checked on the server as you type;
					invalid flows are not saved.</small
				>
			</p>
		{:else if data.schema}
			{#await import('#lib/Designer.svelte') then { default: Designer }}
				<Designer bind:value={yaml} source={name} schema={data.schema} />
			{/await}
		{/if}
	{/key}
{/if}

<style>
	.bar {
		display: flex;
		gap: 1rem;
		align-items: center;
		margin-bottom: 0.75rem;
	}
	h1 {
		font-size: 1.25rem;
		margin: 0;
	}
	.tabs {
		display: flex;
		gap: 0.25rem;
		margin-bottom: 0.5rem;
	}
	.tabs [aria-selected='true'] {
		font-weight: 600;
		text-decoration: underline;
	}
	.error {
		color: #cf222e;
		white-space: pre-wrap;
	}
</style>
