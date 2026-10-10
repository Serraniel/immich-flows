<script lang="ts">
	let { status }: { status: string } = $props();
</script>

<span class="status {status}">{status}</span>

<style>
	.status {
		padding: 0.1rem 0.5rem;
		border-radius: 1rem;
		font-size: 0.85em;
		background: color-mix(in srgb, currentColor 12%, transparent);
	}
	.success {
		color: #1a7f37;
	}
	.failed {
		color: #cf222e;
	}
	.running,
	.skipped {
		color: #9a6700;
	}
</style>
