<script lang="ts">
	import { api } from './api';

	type Info = { path: string; headerName: string; headerValue: string };
	type FlowHook = Info & { flowId: string; name?: string };
	let info = $state<Info>();
	let hooks = $state<FlowHook[]>([]);
	let error = $state('');
	const fail = (e: unknown) => (error = e instanceof Error ? e.message : String(e));
	api<Info>('webhook')
		.then((i) => (info = i))
		.catch(fail);
	api<FlowHook[]>('flow-webhooks')
		.then((h) => (hooks = h))
		.catch(fail);
	const origin = typeof location === 'undefined' ? '' : location.origin;
</script>

<section>
	<h2>Assets added outside immich-flows</h2>
	<p>
		External libraries and other imports that do not pass immich-flows reach flows through an Immich
		workflow: in Immich, create a workflow with trigger <code>AssetCreate</code> and the step
		<em>Trigger Webhook</em>:
	</p>
	{#if info}
		<dl>
			<dt>URL</dt>
			<dd>
				<code>http://immich-flows:2283{info.path}</code>
				<small>(as Immich reaches immich-flows)</small>
			</dd>
			<dt>Header name</dt>
			<dd><code>{info.headerName}</code></dd>
			<dt>Header value</dt>
			<dd>
				<code class="secret">{info.headerValue}</code> <small>(yours; keep it private)</small>
			</dd>
		</dl>
	{:else if error}
		<p role="alert">{error}</p>
	{/if}
</section>

{#if hooks.length}
	<section>
		<h2>Webhook flows</h2>
		<p>
			Start these flows with a JSON POST, e.g. from scripts or home automation. The body is
			<code>trigger.data.body</code>; <code>assetIds</code> starts one run per asset. Runs act as you.
		</p>
		{#each hooks as h (h.flowId)}
			<h3>{h.name ?? h.flowId}</h3>
			<pre>curl -X POST -H '{h.headerName}: {h.headerValue}' -H 'Content-Type: application/json' \
  -d '&#123;"assetIds": []&#125;' {origin}{h.path}</pre>
		{/each}
	</section>
{/if}

<style>
	dl {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 0.25rem 1rem;
	}
	dt {
		font-weight: 600;
	}
	pre {
		white-space: pre-wrap;
		word-break: break-all;
		background: color-mix(in srgb, currentColor 6%, transparent);
		padding: 0.5rem;
	}
	.secret {
		word-break: break-all;
	}
</style>
