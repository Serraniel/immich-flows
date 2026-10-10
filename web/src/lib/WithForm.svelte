<script lang="ts">
	import { parse as parseYAML, stringify } from 'yaml';
	import { field, listText, parseList } from './stepschema';
	import type { Schema } from './yamlschema';

	type Props = {
		schema: Schema | undefined;
		value: Record<string, unknown> | undefined;
		// key undefined: replace the whole with: block.
		onchange: (key: string | undefined, value: unknown) => void;
	};
	let { schema, value, onchange }: Props = $props();

	let fields = $derived(Object.entries(schema?.properties ?? {}));
	let required = $derived(new Set((schema as { required?: string[] })?.required ?? []));
	let yamlError = $state('');

	const toYAML = (v: unknown) => (v === undefined ? '' : stringify(v).trimEnd());

	function fromYAML(key: string | undefined, text: string) {
		try {
			yamlError = '';
			onchange(key, text.trim() ? parseYAML(text) : undefined);
		} catch (e) {
			yamlError = e instanceof Error ? e.message : String(e);
		}
	}
</script>

{#if fields.length === 0}
	<label>
		with <small>(YAML)</small>
		<textarea
			rows="6"
			value={toYAML(value)}
			onchange={(e) => fromYAML(undefined, e.currentTarget.value)}></textarea>
	</label>
{/if}

{#each fields as [key, prop] (key)}
	{@const f = field(prop)}
	{@const v = value?.[key]}
	<label class:inline={f.kind === 'boolean'}>
		<span>{key}{required.has(key) ? ' *' : ''}</span>
		{#if f.kind === 'select'}
			<select value={v ?? ''} onchange={(e) => onchange(key, e.currentTarget.value || undefined)}>
				<option value="">–</option>
				{#each f.choices as c (c.value)}<option value={c.value}>{c.value}</option>{/each}
			</select>
		{:else if f.kind === 'boolean'}
			<input
				type="checkbox"
				checked={v === true}
				onchange={(e) => onchange(key, e.currentTarget.checked || undefined)}
			/>
		{:else if f.kind === 'number'}
			<input
				type="number"
				value={v ?? ''}
				onchange={(e) =>
					onchange(key, e.currentTarget.value === '' ? undefined : Number(e.currentTarget.value))}
			/>
		{:else if f.kind === 'checks'}
			<span class="checks">
				{#each f.choices as c (c.value)}
					{@const list = Array.isArray(v) ? v.map(String) : []}
					<label class="inline">
						<input
							type="checkbox"
							checked={list.includes(c.value)}
							onchange={(e) =>
								onchange(
									key,
									e.currentTarget.checked ? [...list, c.value] : list.filter((x) => x !== c.value)
								)}
						/>
						{c.value}
					</label>
				{/each}
			</span>
		{:else if f.kind === 'list'}
			<textarea
				rows="3"
				placeholder="one per line"
				value={listText(v)}
				onchange={(e) => onchange(key, parseList(e.currentTarget.value, f.allowString))}></textarea>
		{:else if f.kind === 'yaml'}
			<textarea rows="4" value={toYAML(v)} onchange={(e) => fromYAML(key, e.currentTarget.value)}
			></textarea>
		{:else}
			<input
				value={v === undefined ? '' : String(v)}
				onchange={(e) => onchange(key, e.currentTarget.value)}
			/>
		{/if}
		{#if prop.description}<small>{prop.description}</small>{/if}
	</label>
{/each}
{#if yamlError}<p class="error">{yamlError}</p>{/if}

<style>
	label {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		margin-bottom: 0.6rem;
		font-size: 0.85rem;
	}
	label.inline {
		flex-direction: row;
		align-items: center;
		gap: 0.4rem;
		margin-bottom: 0.3rem;
	}
	.checks {
		display: flex;
		flex-wrap: wrap;
		gap: 0 0.75rem;
	}
	small {
		opacity: 0.7;
	}
	textarea,
	input:not([type='checkbox']),
	select {
		font-family: ui-monospace, monospace;
		font-size: 0.85rem;
	}
	.error {
		color: #cf222e;
		font-size: 0.85rem;
	}
</style>
