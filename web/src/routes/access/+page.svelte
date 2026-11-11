<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Label } from '#lib/components/ui/label';
	import { Card, CardContent, CardHeader, CardTitle } from '#lib/components/ui/card';
	import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#lib/components/ui/table';
	import { Badge } from '#lib/components/ui/badge';
	import { Copy, ShieldCheck } from '@lucide/svelte';

	type Product = { id: number; slug: string; name: string; status: string };
	type Token = {
		id: number; name: string; hint: string; scopes: string[]; product_id: number;
		expires_at: string | null; last_used_at: string | null; revoked_at: string | null; created_at: string;
	};
	const scopes = [
		'products:read', 'products:write', 'licenses:read', 'licenses:write', 'releases:write', 'audit:read'
	];
	let products = $state<Product[]>([]);
	let tokens = $state<Token[]>([]);
	let error = $state('');
	let raw = $state('');
	let busy = $state(false);
	let form = $state({ name: '', product_id: 0, expires_days: 90, scopes: ['products:read', 'products:write'] });

	async function load() {
		error = '';
		try {
			const [p, t] = await Promise.all([
				api.get<{ products: Product[] }>('/admin/products'),
				api.get<{ tokens: Token[] }>('/admin/tokens')
			]);
			products = p.products;
			tokens = t.tokens;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load tokens.';
		}
	}
	onMount(load);

	async function create(event: SubmitEvent) {
		event.preventDefault();
		busy = true;
		error = '';
		try {
			const data = await api.post<{ token: string }>('/admin/tokens', {
				name: form.name, product_id: Number(form.product_id), expires_days: form.expires_days, scopes: form.scopes
			});
			raw = data.token;
			form.name = '';
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not create token.';
		} finally {
			busy = false;
		}
	}
	async function revoke(id: number) {
		if (!confirm('Revoke this token? Existing agent calls will stop immediately.')) return;
		try {
			await api.post(`/admin/tokens/${id}/revoke`);
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not revoke token.';
		}
	}
	async function copy() {
		await navigator.clipboard.writeText(raw);
	}
</script>

<div class="space-y-8">
	<div>
		<p class="text-[11px] uppercase tracking-[0.2em] text-muted-foreground">Machines</p>
		<h1 class="mt-1 text-3xl font-semibold tracking-tight">Agent access</h1>
		<p class="mt-1 text-sm text-muted-foreground">Scoped tokens for project agents. A token can only touch its product and scopes.</p>
	</div>

	{#if raw}
		<div class="rounded-xl border border-accent/40 bg-accent/10 p-4">
			<div class="flex items-center justify-between gap-3">
				<div><p class="text-sm font-semibold">Copy this token now</p><p class="text-xs text-muted-foreground">It is shown once and cannot be recovered.</p></div>
				<Button variant="outline" onclick={copy}><Copy class="size-4" /> Copy</Button>
			</div>
			<code class="mt-3 block break-all rounded bg-background/80 p-3 text-sm">{raw}</code>
		</div>
	{/if}
	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}

	<div class="grid gap-6 xl:grid-cols-[1fr_400px]">
		<section class="rounded-xl border border-border bg-card/80">
			<Table>
				<TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Product</TableHead><TableHead>Scopes</TableHead><TableHead>Last used</TableHead><TableHead></TableHead></TableRow></TableHeader>
				<TableBody>
					{#each tokens as token (token.id)}
						<TableRow>
							<TableCell class="font-medium">{token.name}<span class="ml-2 font-mono text-[11px] text-muted-foreground">•••• {token.hint}</span></TableCell>
							<TableCell class="text-xs">{token.product_id ? products.find((p) => p.id === token.product_id)?.name ?? token.product_id : 'All products'}</TableCell>
							<TableCell><div class="flex max-w-64 flex-wrap gap-1">{#each token.scopes as scope (scope)}<Badge variant="outline">{scope}</Badge>{/each}</div></TableCell>
							<TableCell class="text-xs text-muted-foreground">{token.last_used_at ? new Date(token.last_used_at).toLocaleString() : 'Never'}{#if token.revoked_at}<span class="text-destructive"> · revoked</span>{/if}</TableCell>
							<TableCell class="text-right">{#if !token.revoked_at}<button class="rounded px-2 py-1 text-xs text-destructive hover:bg-destructive/10" onclick={() => revoke(token.id)}>Revoke</button>{/if}</TableCell>
						</TableRow>
					{:else}
						<TableRow><TableCell colspan={5} class="py-10 text-center text-sm text-muted-foreground">No agent tokens yet.</TableCell></TableRow>
					{/each}
				</TableBody>
			</Table>
		</section>

		<Card>
			<CardHeader><CardTitle class="flex items-center gap-2"><ShieldCheck class="size-4" /> New token</CardTitle></CardHeader>
			<CardContent>
				<form class="space-y-3" onsubmit={create}>
					<div class="space-y-1.5"><Label for="name">Name</Label><Input id="name" bind:value={form.name} placeholder="reorchestra-agent" required /></div>
					<div class="space-y-1.5"><Label for="product">Product scope</Label>
						<select id="product" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm" bind:value={form.product_id}>
							<option value={0}>All products</option>
							{#each products as product (product.id)}<option value={product.id}>{product.name}</option>{/each}
						</select>
					</div>
					<div class="space-y-1.5"><Label for="days">Expires in days (0 = never)</Label><Input id="days" type="number" min="0" max="3650" bind:value={form.expires_days} /></div>
					<fieldset class="space-y-2">
						<legend class="text-sm font-medium">Scopes</legend>
						<div class="grid grid-cols-2 gap-2">
							{#each scopes as scope (scope)}
								<label class="flex items-center gap-2 rounded-md border border-border px-2 py-1.5 font-mono text-[11px]">
									<input type="checkbox" value={scope} bind:group={form.scopes} /> {scope}
								</label>
							{/each}
						</div>
					</fieldset>
					<Button class="w-full" type="submit" disabled={busy || !form.name || form.scopes.length === 0}>{busy ? 'Creating…' : 'Create token'}</Button>
				</form>
			</CardContent>
		</Card>
	</div>
</div>
