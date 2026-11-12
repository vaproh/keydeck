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
	type Preset = { id: number; name: string; limits: Record<string, number>; features: string[]; validity_days: number | null; max_activations: number };
	type LimitDef = { id: number; key: string; label: string };
	type FeatureDef = { id: number; key: string; label: string };
	type License = {
		id: number; product_id: number; key_last4: string; plan_name: string; holder_name: string; key_type: string; display_status: string;
		expires_at: string; activation_expires_at: string | null; status: string; max_activations: number;
		activation_count: number; product_slug: string;
	};
	let products = $state<Product[]>([]);
	let licenses = $state<License[]>([]);
	let presets = $state<Preset[]>([]);
	let limitDefs = $state<LimitDef[]>([]);
	let featureDefs = $state<FeatureDef[]>([]);
	let limitValues = $state<Record<string, number>>({});
	let featureValues = $state<Record<string, boolean>>({});
	let productId = $state('');
	let error = $state('');
	let rawKey = $state('');
	let busy = $state(false);
	let form = $state({
		preset_id: 0, plan_name: 'Personal', holder_name: '', key_type: 'timed', trial_minutes: 15, validity_days: 30,
		starts_on_activation: true, one_time_activation: false, grace_seconds: 86400, lease_ttl_hours: 24,
		max_activations: 1, notes: ''
	});

	async function loadProducts() {
		const data = await api.get<{ products: Product[] }>('/admin/products');
		products = data.products;
		if (!productId && products[0]) productId = String(products[0].id);
	}
	async function loadPresets() {
		if (!productId) {
			presets = [];
			limitDefs = [];
			featureDefs = [];
			return;
		}
		const data = await api.get<{ presets: Preset[]; limits: LimitDef[]; features: FeatureDef[] }>(`/admin/products/${productId}`);
		presets = data.presets;
		limitDefs = data.limits;
		featureDefs = data.features;
		limitValues = Object.fromEntries(limitDefs.map((def) => [def.key, 0]));
		featureValues = Object.fromEntries(featureDefs.map((def) => [def.key, false]));
	}
	async function loadLicenses() {
		const query = productId ? `?product_id=${productId}` : '';
		const data = await api.get<{ licenses: License[] }>(`/admin/licenses${query}`);
		licenses = data.licenses;
	}
	function applyPreset() {
		const preset = presets.find((p) => p.id === Number(form.preset_id));
		if (!preset) {
			for (const def of limitDefs) limitValues[def.key] = 0;
			for (const def of featureDefs) featureValues[def.key] = false;
			return;
		}
		form.plan_name = preset.name;
		form.max_activations = preset.max_activations;
		if (preset.validity_days) form.validity_days = preset.validity_days;
		for (const def of limitDefs) limitValues[def.key] = Number(preset.limits?.[def.key] ?? 0);
		for (const def of featureDefs) featureValues[def.key] = (preset.features ?? []).includes(def.key);
	}
	async function changeProduct() {
		form.preset_id = 0;
		await loadPresets();
		await loadLicenses();
	}
	async function load() {
		error = '';
		try {
			await loadProducts();
			await loadPresets();
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
				preset_id: form.preset_id, plan_name: form.plan_name, holder_name: form.holder_name, key_type: form.key_type, starts_on_activation: form.starts_on_activation,
				one_time_activation: form.one_time_activation, grace_seconds: form.grace_seconds,
				lease_ttl_seconds: Math.max(60, Math.round(form.lease_ttl_hours * 3600)),
				max_activations: form.max_activations,
				limits: Object.fromEntries(limitDefs.map((def) => [def.key, Number(limitValues[def.key]) || 0])),
				features: featureDefs.filter((def) => featureValues[def.key]).map((def) => def.key),
				notes: form.notes
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
		<select class="h-9 rounded-md border border-input bg-background px-3 text-sm" bind:value={productId} onchange={changeProduct}>
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
				<TableHeader><TableRow><TableHead>Plan</TableHead><TableHead>Assigned to</TableHead><TableHead>Type</TableHead><TableHead>Key</TableHead><TableHead>Activations</TableHead><TableHead>Expires</TableHead><TableHead></TableHead></TableRow></TableHeader>
				<TableBody>
					{#each licenses as row (row.id)}
						<TableRow>
							<TableCell class="font-medium"><a href={`/admin/keys/${row.id}`} class="hover:underline">{row.plan_name}</a></TableCell>
							<TableCell class="text-xs">{#if row.holder_name}{row.holder_name}{:else}<span class="text-muted-foreground">Unassigned</span>{/if}</TableCell>
							<TableCell><div class="flex gap-1"><Badge variant="outline">{row.key_type}</Badge><Badge variant={row.display_status === 'active' ? 'secondary' : 'outline'}>{row.display_status}</Badge></div></TableCell>
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
						<TableRow><TableCell colspan={7} class="py-10 text-center text-sm text-muted-foreground">No keys for this product yet.</TableCell></TableRow>
					{/each}
				</TableBody>
			</Table>
		</section>

		<Card>
			<CardHeader><CardTitle class="flex items-center gap-2"><KeyRound class="size-4" /> Issue key</CardTitle></CardHeader>
			<CardContent>
				<form class="space-y-3" onsubmit={issue}>
					{#if presets.length}
						<div class="space-y-1.5"><Label for="preset">Plan preset</Label>
							<select id="preset" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm" bind:value={form.preset_id} onchange={applyPreset}>
								<option value={0}>Custom</option>
								{#each presets as preset (preset.id)}<option value={preset.id}>{preset.name}</option>{/each}
							</select>
						</div>
					{/if}
					{#if !form.preset_id}
						<div class="space-y-1.5"><Label for="plan">Plan name</Label><Input id="plan" bind:value={form.plan_name} required /></div>
					{:else}
						<div class="rounded-md border border-border bg-muted/40 px-3 py-2 text-sm"><span class="text-muted-foreground">Plan: </span>{form.plan_name}</div>
					{/if}
					<div class="space-y-1.5"><Label for="holder">Assigned to</Label><Input id="holder" bind:value={form.holder_name} placeholder="Customer or machine name" maxlength={120} /></div>
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
					<div class="space-y-1.5"><Label for="lease">Lease TTL (hours)</Label><Input id="lease" type="number" min="0.1" max="720" step="0.1" bind:value={form.lease_ttl_hours} /></div>
					{#if limitDefs.length}
						<fieldset class="space-y-2">
							<legend class="text-sm font-medium">Limits</legend>
							<div class="grid grid-cols-2 gap-2">
								{#each limitDefs as def (def.id)}
									<div class="space-y-1.5"><Label for={`limit-${def.key}`}>{def.label || def.key}</Label><Input id={`limit-${def.key}`} type="number" min="0" bind:value={limitValues[def.key]} /></div>
								{/each}
							</div>
						</fieldset>
					{/if}
					{#if featureDefs.length}
						<fieldset class="space-y-2">
							<legend class="text-sm font-medium">Features</legend>
							<div class="grid grid-cols-2 gap-2">
								{#each featureDefs as def (def.id)}
									<label class="flex items-center gap-2 rounded-md border border-border px-2 py-1.5 text-sm"><input type="checkbox" bind:checked={featureValues[def.key]} /> {def.label || def.key}</label>
								{/each}
							</div>
						</fieldset>
					{/if}
					<Button class="w-full" type="submit" disabled={busy || !productId}>{busy ? 'Issuing…' : 'Issue key'}</Button>
				</form>
			</CardContent>
		</Card>
	</div>
</div>
