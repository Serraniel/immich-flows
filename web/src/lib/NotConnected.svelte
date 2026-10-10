<script lang="ts">
	import { connect, getConnection } from './connection';

	let connected = $state(true);
	let error = $state('');
	let busy = $state(false);

	getConnection()
		.then((c) => (connected = c.connected))
		.catch(() => {});

	async function run() {
		busy = true;
		error = '';
		try {
			connected = (await connect()).connected;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
</script>

{#if !connected}
	<div class="banner" role="alert">
		<strong>Not connected:</strong> flows do not run for your assets until you connect immich-flows.
		<button disabled={busy} onclick={run}>Connect</button>
		{#if error}<span>{error}</span>{/if}
	</div>
{/if}

<style>
	.banner {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: center;
		padding: 0.6rem 0.8rem;
		margin-bottom: 1rem;
		border: 1px solid #bf8700;
		border-radius: 0.3rem;
		background: color-mix(in srgb, #bf8700 12%, transparent);
	}
</style>
