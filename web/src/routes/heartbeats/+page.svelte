<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api';
	import { Badge } from '#lib/components/ui/badge';
	import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#lib/components/ui/table';

	type Product = { id: number; slug: string; name: string; status: string };
	type Heartbeat = {
		id: number; license_id: number; activation_id: number | null; matched: boolean;
		installation_hash: string; app_version: string; ip: string; device_name: string;
		os: string; os_version: string; arch: string; user_agent: string; created_at: string;
		holder_name: string; plan_name: string; key_last4: string; product_slug: string; product_name: string;
	};
	let products = $state<Product[]>([]);
	let beats = $state<Heartbeat[]>([]);
	let total = $state(0);
	let productId = $state('');
	let error = $state('');

	async function load() {
		error = '';
		try {
			const query = productId ? `?product_id=${productId}` : '';
			const data = await api.get<{ heartbeats: Heartbeat[]; total: number }>(`/admin/heartbeats${query}`);
			beats = data.heartbeats;
			total = data.total;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load heartbeats.';
		}
	}
	async function init() {
		try {
			const data = await api.get<{ products: Product[] }>('/admin/products');
			products = data.products;
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load products.';
		}
	}
	onMount(init);
	function when(value: string) {
		return new Date(value).toLocaleString();
	}
	function short(value: string) {
		return value ? `${value.slice(0, 10)}…` : '—';
	}
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<p class="text-[11px] uppercase tracking-[0.2em] text-muted-foreground">Live traffic</p>
			<h1 class="mt-1 text-3xl font-semibold tracking-tight">Heartbeats</h1>
			<p class="mt-1 text-sm text-muted-foreground">Latest 7 days. Unknown means no activation log could be matched.</p>
		</div>
		<select class="h-9 rounded-md border border-input bg-background px-3 text-sm" bind:value={productId} onchange={load}>
			<option value="">All products</option>
			{#each products as product (product.id)}<option value={String(product.id)}>{product.name}</option>{/each}
		</select>
	</div>

	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}

	<div class="text-xs text-muted-foreground">{total.toLocaleString()} heartbeats retained</div>
	<section class="rounded-xl border border-border bg-card/80">
		<Table>
			<TableHeader><TableRow><TableHead>Time</TableHead><TableHead>Key</TableHead><TableHead>Match</TableHead><TableHead>Device</TableHead><TableHead>OS</TableHead><TableHead>App</TableHead><TableHead>IP</TableHead></TableRow></TableHeader>
			<TableBody>
				{#each beats as beat (beat.id)}
					<TableRow>
						<TableCell class="text-xs text-muted-foreground">{when(beat.created_at)}</TableCell>
						<TableCell class="text-xs"><a class="font-medium hover:underline" href="/admin/keys/{beat.license_id}">{beat.holder_name || 'Unassigned'}</a><p class="text-muted-foreground">{beat.plan_name} · •••• {beat.key_last4}</p></TableCell>
						<TableCell><Badge variant={beat.matched ? 'secondary' : 'outline'}>{beat.matched ? 'matched' : 'unknown'}</Badge></TableCell>
						<TableCell class="text-xs">{beat.device_name || 'Unknown device'}<p class="font-mono text-[10px] text-muted-foreground">{short(beat.installation_hash)}</p></TableCell>
						<TableCell class="text-xs">{beat.os || '—'} {beat.os_version}{#if beat.arch}<span class="text-muted-foreground"> · {beat.arch}</span>{/if}</TableCell>
						<TableCell class="text-xs">{beat.app_version || '—'}</TableCell>
						<TableCell class="font-mono text-xs">{beat.ip || '—'}</TableCell>
					</TableRow>
				{:else}
					<TableRow><TableCell colspan={7} class="py-12 text-center text-sm text-muted-foreground">No heartbeats in the last 7 days.</TableCell></TableRow>
				{/each}
			</TableBody>
		</Table>
	</section>
</div>
