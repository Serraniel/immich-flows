<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';
	import type { Kind } from './flowgraph';

	type Data = { kind: Kind; label: string; detail?: string; error?: string };
	let { data, selected }: NodeProps & { data: Data } = $props();
</script>

{#if data.kind !== 'trigger'}
	<Handle type="target" position={Position.Top} isConnectable={false} />
{/if}
<div
	class="node {data.kind}"
	class:selected
	class:error={!!data.error}
	title={data.error ?? [data.label, data.detail].filter(Boolean).join('\n')}
>
	<small>{data.kind === 'case' ? 'case' : data.kind === 'step' ? '' : data.kind}</small>
	<strong>{data.label}</strong>
	{#if data.detail && data.detail !== data.label}<span>{data.detail}</span>{/if}
</div>
<Handle type="source" position={Position.Bottom} isConnectable={false} />

<style>
	.node {
		width: 200px;
		box-sizing: border-box;
		padding: 0.4rem 0.6rem;
		border: 1px solid color-mix(in srgb, currentColor 30%, transparent);
		border-radius: 0.4rem;
		background: Canvas;
		color: CanvasText;
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
		font-size: 0.8rem;
	}
	.node strong,
	.node span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.node span {
		opacity: 0.7;
		font-family: ui-monospace, monospace;
	}
	small {
		text-transform: uppercase;
		font-size: 0.65rem;
		opacity: 0.6;
	}
	small:empty {
		display: none;
	}
	.trigger {
		border-color: #1f883d;
	}
	.switch,
	.case {
		border-style: dashed;
	}
	.case {
		width: 160px;
		border-radius: 1rem;
	}
	.selected {
		outline: 2px solid Highlight;
	}
	.error {
		border-color: #cf222e;
		border-width: 2px;
	}
</style>
