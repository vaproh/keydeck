<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api';
	import { Card, CardContent, CardHeader, CardTitle } from '#lib/components/ui/card';
	import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#lib/components/ui/table';
	import { Badge } from '#lib/components/ui/badge';
	import { Activity, KeyRound, MonitorSmartphone, TriangleAlert } from '@lucide/svelte';

	type Stats = {
		products: number;
		licenses: number;
		active_licenses: number;
		activations: number;
		expiring_7: number;
		expiring_30: number;
	};
	type Expiring = { id: number; product_slug: string; plan_name: string; key_last4: string; expires_at: string };
	type Payload = { stats: Stats; expiring: Expiring[] };

	let data = $state<Payload | null>(null);
	let error = $state('');

	onMount(async () => {
		try {
			data = await api.get<Payload>('/admin/stats');
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load the control room.';
		}
	});

	function when(value: string) {
		return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
	}
</script>

<div class="space-y-8">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<p class="text-[11px] uppercase tracking-[0.2em] text-muted-foreground">Keydeck</p>
			<h1 class="mt-1 text-3xl font-semibold tracking-tight">Control room</h1>
			<p class="mt-1 max-w-xl text-sm text-muted-foreground">Keys, activations and releases for every product, served from one desk.</p>
		</div>
		<a href="/admin/keys" class="rounded-md bg-primary px-3.5 py-2 text-sm font-medium text-primary-foreground hover:opacity-90">Issue a key</a>
	</div>

	{#if error}
		<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>
	{:else if !data}
		<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
			{#each Array(4) as _}
				<div class="h-32 animate-pulse rounded-xl border border-border bg-card/60"></div>
			{/each}
		</div>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
			<Card class="desk-rise">
				<CardHeader class="flex-row items-center justify-between space-y-0">
					<CardTitle class="text-sm font-medium text-muted-foreground">Products</CardTitle>
					<Activity class="size-4 text-accent" />
				</CardHeader>
				<CardContent><p class="text-3xl font-semibold">{data.stats.products}</p><p class="mt-1 text-xs text-muted-foreground">active products</p></CardContent>
			</Card>
			<Card class="desk-rise">
				<CardHeader class="flex-row items-center justify-between space-y-0">
					<CardTitle class="text-sm font-medium text-muted-foreground">Active keys</CardTitle>
					<KeyRound class="size-4 text-accent" />
				</CardHeader>
				<CardContent><p class="text-3xl font-semibold">{data.stats.active_licenses}</p><p class="mt-1 text-xs text-muted-foreground">{data.stats.licenses} issued total</p></CardContent>
			</Card>
			<Card class="desk-rise">
				<CardHeader class="flex-row items-center justify-between space-y-0">
					<CardTitle class="text-sm font-medium text-muted-foreground">Activations</CardTitle>
					<MonitorSmartphone class="size-4 text-accent" />
				</CardHeader>
				<CardContent><p class="text-3xl font-semibold">{data.stats.activations}</p><p class="mt-1 text-xs text-muted-foreground">machines bound</p></CardContent>
			</Card>
			<Card class="desk-rise border-accent/30">
				<CardHeader class="flex-row items-center justify-between space-y-0">
					<CardTitle class="text-sm font-medium text-muted-foreground">Expiring in 7 days</CardTitle>
					<TriangleAlert class="size-4 text-accent" />
				</CardHeader>
				<CardContent><p class="text-3xl font-semibold">{data.stats.expiring_7}</p><p class="mt-1 text-xs text-muted-foreground">{data.stats.expiring_30} within 30 days</p></CardContent>
			</Card>
		</div>

		<section class="rounded-xl border border-border bg-card/80">
			<div class="flex items-center justify-between border-b border-border px-4 py-3 sm:px-5">
				<div>
					<h2 class="text-sm font-semibold">Expiring keys</h2>
					<p class="text-xs text-muted-foreground">Next 30 days, closest first.</p>
				</div>
				<a class="text-xs text-muted-foreground hover:text-foreground" href="/admin/keys">View all</a>
			</div>
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Product</TableHead>
						<TableHead>Plan</TableHead>
						<TableHead>Key</TableHead>
						<TableHead class="text-right">Expires</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each data.expiring as row (row.id)}
						<TableRow>
							<TableCell class="font-medium">{row.product_slug}</TableCell>
							<TableCell><Badge variant="secondary">{row.plan_name}</Badge></TableCell>
							<TableCell class="font-mono text-xs text-muted-foreground">•••• {row.key_last4}</TableCell>
							<TableCell class="text-right text-xs text-muted-foreground">{when(row.expires_at)}</TableCell>
						</TableRow>
					{:else}
						<TableRow><TableCell colspan={4} class="py-8 text-center text-sm text-muted-foreground">Nothing expires soon.</TableCell></TableRow>
					{/each}
				</TableBody>
			</Table>
		</section>
	{/if}
</div>
