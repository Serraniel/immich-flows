<script lang="ts">
	import { api } from './api';

	type Info = { path: string; headerName: string; headerValue: string };
	let info = $state<Info>();
	let error = $state('');
	api<Info>('webhook')
		.then((i) => (info = i))
		.catch((e) => (error = e instanceof Error ? e.message : String(e)));
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

<style>
	dl {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 0.25rem 1rem;
	}
	dt {
		font-weight: 600;
	}
	.secret {
		word-break: break-all;
	}
</style>
