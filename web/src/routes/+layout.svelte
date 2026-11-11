<script lang="ts">
	import './layout.css';
	import favicon from '#lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Toaster } from '#lib/components/ui/sonner';
	import { me, logout, type Session } from '#lib/api';
	import { LayoutDashboard, Boxes, KeyRound, RadioTower, ScrollText, LogOut, ArrowUpRight } from '@lucide/svelte';

	let { children } = $props();
	let session = $state<Session | null>(null);
	let ready = $state(false);

	const nav = [
		{ href: '/', label: 'Overview', icon: LayoutDashboard },
		{ href: '/products', label: 'Products', icon: Boxes },
		{ href: '/keys', label: 'Keys', icon: KeyRound },
		{ href: '/releases', label: 'Releases', icon: RadioTower },
		{ href: '/audit', label: 'Audit', icon: ScrollText }
	];

	const isLogin = $derived(page.url.pathname.endsWith('/login'));

	onMount(async () => {
		if (isLogin) {
			ready = true;
			return;
		}
		try {
			session = await me();
		} catch {
			await goto('/login');
		} finally {
			ready = true;
		}
	});

	async function signOut() {
		try {
			await logout();
		} finally {
			await goto('/login');
		}
	}
</script>

<svelte:head>
	<title>Keydeck · license control</title>
	<link rel="icon" href={favicon} />
</svelte:head>

{#if !ready}
	<div class="grid min-h-screen place-items-center text-sm text-muted-foreground">Opening the desk…</div>
{:else if isLogin}
	{@render children()}
{:else if session}
	<div class="min-h-screen lg:grid lg:grid-cols-[232px_1fr]">
		<aside class="border-b border-border bg-card/80 backdrop-blur lg:min-h-screen lg:border-r lg:border-b-0">
			<div class="flex h-16 items-center gap-3 px-5">
				<img src={favicon} alt="" class="size-8 rounded-md" />
				<div>
					<p class="text-sm font-semibold tracking-tight">Keydeck</p>
					<p class="text-[11px] text-muted-foreground">license + release control</p>
				</div>
			</div>
			<nav class="flex gap-1 overflow-x-auto px-3 pb-3 lg:flex-col lg:overflow-visible">
				{#each nav as item (item.href)}
					{@const Icon = item.icon}
					<a
						href="/admin{item.href}"
						class="flex shrink-0 items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors {page.url.pathname === '/admin' + item.href || (item.href !== '/' && page.url.pathname.startsWith('/admin' + item.href))
							? 'bg-primary text-primary-foreground'
							: 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
					>
						<Icon class="size-4" />
						{item.label}
					</a>
				{/each}
			</nav>
			<div class="hidden px-4 py-5 lg:block">
				<div class="rounded-lg border border-border bg-background/60 p-3">
					<p class="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">Signed in</p>
					<p class="mt-1 text-sm font-medium">{session.actor}</p>
					<button onclick={signOut} class="mt-3 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
						<LogOut class="size-3.5" /> Sign out
					</button>
				</div>
			</div>
		</aside>

		<div class="min-w-0">
			<header class="flex h-16 items-center justify-between border-b border-border px-5 lg:px-8">
				<a href="/admin" class="text-sm font-medium text-muted-foreground hover:text-foreground">Control room</a>
				<div class="flex items-center gap-3 text-xs text-muted-foreground">
					<span class="hidden sm:inline">server clock · UTC</span>
					<a href="/admin/keys" class="inline-flex items-center gap-1 rounded-md border border-border px-2.5 py-1.5 text-foreground hover:bg-muted">
						Issue a key <ArrowUpRight class="size-3.5" />
					</a>
				</div>
			</header>
			<main class="desk-rise mx-auto w-full max-w-7xl px-5 py-8 lg:px-8">
				{@render children()}
			</main>
		</div>
	</div>
	<Toaster />
{/if}
