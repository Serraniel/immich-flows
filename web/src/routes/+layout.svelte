<script lang="ts">
	import favicon from '#lib/assets/favicon.svg';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>immich-flows</title>
</svelte:head>

<header>
	<strong>immich-flows</strong>
	<!-- Immich itself, outside this app. -->
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
	<a href="/" data-sveltekit-reload>Back to Immich</a>
</header>

<main>
	{@render children()}
</main>

<style>
	:global(body) {
		margin: 0;
		font-family: system-ui, sans-serif;
		color-scheme: light dark;
	}
	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid color-mix(in srgb, currentColor 15%, transparent);
	}
	main {
		padding: 1rem;
		max-width: 72rem;
		margin: 0 auto;
	}
</style>
