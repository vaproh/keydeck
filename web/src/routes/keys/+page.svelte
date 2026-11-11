<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api } from '#lib/api';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Label } from '#lib/components/ui/label';
	import { Card, CardContent, CardHeader, CardTitle } from '#lib/components/ui/card';
	import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#lib/components/ui/table';
	import { Badge } from '#lib/components/ui/badge';
	import { Copy, KeyRound } from '@lucide/svelte';

	type Product = { id: number; slug: string; name: string; status: string };
	type License = {
		id: number; product_id: number; key_last4: string; plan_name: string; key_type: string;
		expires_at: string; activation_expires_at: string | null; status: string; max_activations: number;
		activation_count: number; product_slug: string;
	};
	let products = $state<Product[]>([]);
	let licenses = $state<License[]>([]);
	let productId = $state('');
	let error = $state('');
	let rawKey = $state('');
	let busy = $state(false);
	let form = $state({
		plan_name: 'Personal', key_type: 'timed', trial_minutes: 15, validity_days: 30,
		starts_on_activation: true, one_time_activation: false, grace_seconds: 86400,
		max_activations: 1, limits: '{"max_accounts": 25}', features: '["warmup"]', notes: ''
	});

	async function loadProducts() {
		const data = await api.get<{ products: Product[] }>('/admin/products');
		products = data.products;
		if (!productId && products[0]) productId = String(products[0].id);
	}
	async function loadLicenses() {
		const query = productId ? `?product_id=${productId}` : '';
		const data = await api.get<{ licenses: License[] }>(`/admin/licenses${query}`);
		licenses = data.licenses;
	}
	async function load() {
		error = '';
		try {
			await loadProducts();
			await loadLicenses();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load keys.';
		}
	}

	onMount(() => {
		productId = page.url.searchParams.get('product') ?? '';
		load();
	});

	async function issue(event: SubmitEvent) {
		event.preventDefault();
		if (!productId || busy) return;
		busy = true;
		error = '';
		try {
			const body: Record<string, unknown> = {
				plan_name: form.plan_name, key_type: form.key_type, starts_on_activation: form.starts_on_activation,
				one_time_activation: form.one_time_activation, grace_seconds: form.grace_seconds,
				max_activations: form.max_activations, limits: JSON.parse(form.limits),
				features: JSON.parse(form.features), notes: form.notes
			};
			if (form.key_type === 'trial') body.trial_minutes = form.trial_minutes;
			if (form.key_type === 'timed') body.validity_days = form.validity_days;
			const data = await api.post<{ key: string }>(`/admin/products/${productId}/licenses`, body);
			rawKey = data.key;
			await loadLicenses();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not issue key.';
		} finally {
			busy = false;
		}
	}

	async function action(id: number, kind: string) {
		try {
			await api.post(`/admin/licenses/${id}/${kind}`, kind === 'extend' ? { days: 30 } : undefined);
			await loadLicenses();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Action failed.';
		}
	}

	async function copy() {
		await navigator.clipboard.writeText(rawKey);
	}
</script>

<div class="space-y-8">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<p class="text-[11px] uppercase tracking-[0.2em] text-muted-foreground">Inventory</p>
			<h1 class="mt-1 text-3xl font-semibold tracking-tight">Keys</h1>
			<p class="mt-1 text-sm text-muted-foreground">Issue trial, timed or lifetime keys. The server owns the clock.</p>
		</div>
		<select class="h-9 rounded-md border border-input bg-background px-3 text-sm" bind:value={productId} onchange={loadLicenses}>
			{#each products as product (product.id)}<option value={String(product.id)}>{product.name}</option>{/each}
		</select>
	</div>

	{#if rawKey}
		<div class="rounded-xl border border-accent/40 bg-accent/10 p-4">
			<div class="flex items-center justify-between gap-3">
				<div><p class="text-sm font-semibold">Copy this key now</p><p class="text-xs text-muted-foreground">It is shown once.</p></div>
				<Button variant="outline" onclick={copy}><Copy class="size-4" /> Copy</Button>
			</div>
			<code class="mt-3 block break-all rounded bg-background/80 p-3 text-sm">{rawKey}</code>
		</div>
	{/if}
	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}

	<div class="grid gap-6 xl:grid-cols-[1fr_380px]">
		<section class="rounded-xl border border-border bg-card/80">
			<Table>
				<TableHeader><TableRow><TableHead>Plan</TableHead><TableHead>Type</TableHead><TableHead>Key</TableHead><TableHead>Activations</TableHead><TableHead>Expires</TableHead><TableHead></TableHead></TableRow></TableHeader>
				<TableBody>
					{#each licenses as row (row.id)}
						<TableRow>
							<TableCell class="font-medium">{row.plan_name}</TableCell>
							<TableCell><Badge variant="outline">{row.key_type}</Badge></TableCell>
							<TableCell class="font-mono text-xs text-muted-foreground">•••• {row.key_last4}</TableCell>
							<TableCell class="text-xs">{row.activation_count}/{row.max_activations}</TableCell>
							<TableCell class="text-xs text-muted-foreground">{row.key_type === 'lifetime' ? 'Never' : new Date(row.activation_expires_at ?? row.expires_at).toLocaleString()}</TableCell>
							<TableCell class="text-right">
								<div class="flex justify-end gap-1">
									<button class="rounded px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground" onclick={() => action(row.id, 'extend')}>+30d</button>
									<button class="rounded px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground" onclick={() => action(row.id, 'reset')}>Reset</button>
									<button class="rounded px-2 py-1 text-xs text-destructive hover:bg-destructive/10" onclick={() => action(row.id, 'revoke')}>Revoke</button>
								</div>
							</TableCell>
						</TableRow>
					{:else}
						<TableRow><TableCell colspan={6} class="py-10 text-center text-sm text-muted-foreground">No keys for this product yet.</TableCell></TableRow>
					{/each}
				</TableBody>
			</Table>
		</section>

		<Card>
			<CardHeader><CardTitle class="flex items-center gap-2"><KeyRound class="size-4" /> Issue key</CardTitle></CardHeader>
			<CardContent>
				<form class="space-y-3" onsubmit={issue}>
					<div class="space-y-1.5"><Label for="plan">Plan name</Label><Input id="plan" bind:value={form.plan_name} required /></div>
					<div class="space-y-1.5"><Label for="type">Type</Label>
						<select id="type" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm" bind:value={form.key_type}>
							<option value="timed">Timed</option><option value="trial">Trial</option><option value="lifetime">Lifetime</option>
						</select>
					</div>
					{#if form.key_type === 'trial'}
						<div class="space-y-1.5"><Label for="trial">Trial minutes</Label><Input id="trial" type="number" min="1" bind:value={form.trial_minutes} /></div>
					{:else if form.key_type === 'timed'}
						<div class="space-y-1.5"><Label for="days">Validity days</Label><Input id="days" type="number" min="1" bind:value={form.validity_days} /></div>
						<label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={form.starts_on_activation} /> Start clock on activation</label>
					{/if}
					<div class="grid grid-cols-2 gap-2">
						<div class="space-y-1.5"><Label for="acts">Activations</Label><Input id="acts" type="number" min="1" bind:value={form.max_activations} /></div>
						<div class="space-y-1.5"><Label for="grace">Grace seconds</Label><Input id="grace" type="number" min="0" bind:value={form.grace_seconds} /></div>
					</div>
					<div class="space-y-1.5"><Label for="limits">Limits JSON</Label><textarea id="limits" class="min-h-16 w-full rounded-md border border-input bg-background p-2 font-mono text-xs" bind:value={form.limits}></textarea></div>
					<div class="space-y-1.5"><Label for="features">Features JSON</Label><textarea id="features" class="min-h-14 w-full rounded-md border border-input bg-background p-2 font-mono text-xs" bind:value={form.features}></textarea></div>
					<Button class="w-full" type="submit" disabled={busy || !productId}>{busy ? 'Issuing…' : 'Issue key'}</Button>
				</form>
			</CardContent>
		</Card>
	</div>
</div>
