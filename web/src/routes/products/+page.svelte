<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '#lib/api';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Label } from '#lib/components/ui/label';
	import { Card, CardContent, CardHeader, CardTitle } from '#lib/components/ui/card';
	import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#lib/components/ui/table';
	import { Badge } from '#lib/components/ui/badge';

	type Product = { id: number; slug: string; name: string; status: string; created_at: string };
	let products = $state<Product[]>([]);
	let error = $state('');
	let slug = $state('');
	let name = $state('');
	let prefix = $state('');
	let creating = $state(false);

	async function load() {
		try {
			const data = await api.get<{ products: Product[] }>('/admin/products');
			products = data.products;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load products.';
		}
	}

	onMount(load);

	async function create(event: SubmitEvent) {
		event.preventDefault();
		creating = true;
		error = '';
		try {
			await api.post('/admin/products', { slug, name, key_prefix: prefix });
			slug = '';
			name = '';
			prefix = '';
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not create product.';
		} finally {
			creating = false;
		}
	}
</script>

<div class="space-y-8">
	<div>
		<p class="text-[11px] uppercase tracking-[0.2em] text-muted-foreground">Inventory</p>
		<h1 class="mt-1 text-3xl font-semibold tracking-tight">Products</h1>
		<p class="mt-1 text-sm text-muted-foreground">Each product owns its keys, limits, presets and release channel.</p>
	</div>

	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}

	<div class="grid gap-6 lg:grid-cols-[1fr_340px]">
		<section class="rounded-xl border border-border bg-card/80">
			<Table>
				<TableHeader><TableRow><TableHead>Product</TableHead><TableHead>Slug</TableHead><TableHead>Status</TableHead><TableHead class="text-right">Created</TableHead></TableRow></TableHeader>
				<TableBody>
					{#each products as product (product.id)}
						<TableRow class="cursor-pointer" onclick={() => goto(`/products/${product.id}`)}>
							<TableCell class="font-medium">{product.name}</TableCell>
							<TableCell class="font-mono text-xs text-muted-foreground">{product.slug}</TableCell>
							<TableCell><Badge variant={product.status === 'active' ? 'secondary' : 'outline'}>{product.status}</Badge></TableCell>
							<TableCell class="text-right text-xs text-muted-foreground">{new Date(product.created_at).toLocaleDateString()}</TableCell>
						</TableRow>
					{:else}
						<TableRow><TableCell colspan={4} class="py-10 text-center text-sm text-muted-foreground">No products yet. Add the first one.</TableCell></TableRow>
					{/each}
				</TableBody>
			</Table>
		</section>

		<Card>
			<CardHeader><CardTitle>New product</CardTitle></CardHeader>
			<CardContent>
				<form class="space-y-3" onsubmit={create}>
					<div class="space-y-1.5"><Label for="name">Name</Label><Input id="name" bind:value={name} placeholder="ReOrchestra" required /></div>
					<div class="space-y-1.5"><Label for="slug">Slug</Label><Input id="slug" bind:value={slug} placeholder="reorchestra" required /></div>
					<div class="space-y-1.5"><Label for="prefix">Key prefix</Label><Input id="prefix" bind:value={prefix} placeholder="REO" /></div>
					<p class="text-xs text-muted-foreground">Signing public keys are derived from the product env vars. You can replace them on the product page.</p>
					<Button class="w-full" type="submit" disabled={creating}>{creating ? 'Creating…' : 'Create product'}</Button>
				</form>
			</CardContent>
		</Card>
	</div>
</div>
