<script lang="ts">
	import { connect, disconnect, getConnection, type Connection } from './connection';

	let conn = $state<Connection | null>(null);
	let error = $state('');
	let busy = $state(false);

	getConnection()
		.then((c) => (conn = c))
		.catch((e) => (error = String(e.message ?? e)));

	async function run(fn: () => Promise<Connection | void>) {
		busy = true;
		error = '';
		try {
			conn = (await fn()) ?? { connected: false };
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
</script>

<section>
	<h2>Connection</h2>
	{#if conn === null && !error}
		<p>Loading…</p>
	{:else if conn?.connected}
		<p>
			Connected{#if conn.createdAt}
				since {new Date(conn.createdAt).toLocaleString()}{/if}. Flows run with your Immich account.
		</p>
		<button disabled={busy} onclick={() => run(disconnect)}>Disconnect</button>
	{:else}
		<p>
			Not connected. Connecting creates an Immich API key for immich-flows (assets: read, update,
			download; tags; duplicates: read). Flows only run for connected users.
		</p>
		<button disabled={busy} onclick={() => run(connect)}>Connect</button>
	{/if}
	{#if error}<p role="alert">{error}</p>{/if}
</section>
