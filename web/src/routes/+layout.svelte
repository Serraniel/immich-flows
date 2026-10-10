<script lang="ts">
	import { resolve } from '$app/paths';
	import favicon from '#lib/assets/favicon.svg';
	import type { LayoutProps } from './$types';

	let { children, data }: LayoutProps = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>immich-flows</title>
</svelte:head>

<!-- Links to "/" and "/auth/login" point to Immich itself, outside this app. -->
<header>
	<nav>
		<strong>immich-flows</strong>
		<a href={resolve('/')}>Overview</a>
		<a href={resolve('/runs')}>Runs</a>
	</nav>
	<span>
		{#if data.user}{data.user.name}{/if}
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<a href="/" data-sveltekit-reload>Back to Immich</a>
	</span>
</header>

<main>
	{#if data.user}
		{@render children()}
	{:else}
		<p>
			Not signed in.
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
			<a href="/auth/login?continue=%2F_flows%2F" data-sveltekit-reload>Sign in to Immich</a>
		</p>
	{/if}
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
	nav {
		display: flex;
		gap: 1rem;
		align-items: center;
	}
	header span {
		display: flex;
		gap: 1rem;
	}
	main {
		padding: 1rem;
		max-width: 72rem;
		margin: 0 auto;
	}
</style>
