<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';

	type Product = { id: number; slug: string; name: string; status: string; created_at: string };
	type Item = { id: number; key: string; label: string };
	type Preset = { id: number; name: string; limits_json: unknown; features_json: unknown; validity_days: number | null; max_activations: number };
	type Payload = { product: Product; key: { key_prefix: string; license_public_key: string; update_public_key: string } | null; limits: Item[]; features: Item[]; presets: Preset[] };

	let data = $state<Payload | null>(null);
	let error = $state('');
	let limitKey = $state('');
	let featureKey = $state('');
	let presetName = $state('');
	let presetLimits = $state('{"max_accounts": 25}');
	let presetFeatures = $state('["warmup"]');

	const id = $derived(page.params.id);

	async function load() {
		try {
			data = await api.get<Payload>(`/admin/products/${id}`);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load product.';
		}
	}

	onMount(load);

	async function addLimit(event: SubmitEvent) {
		event.preventDefault();
		await api.post(`/admin/products/${id}/limits`, { key: limitKey, label: limitKey });
		limitKey = '';
		await load();
	}
	async function addFeature(event: SubmitEvent) {
		event.preventDefault();
		await api.post(`/admin/products/${id}/features`, { key: featureKey, label: featureKey });
		featureKey = '';
		await load();
	}
	async function addPreset(event: SubmitEvent) {
		event.preventDefault();
		await api.post(`/admin/products/${id}/presets`, {
			name: presetName, limits: JSON.parse(presetLimits), features: JSON.parse(presetFeatures),
			validity_days: 30, max_activations: 1
		});
		presetName = '';
		await load();
	}
</script>

<div class="space-y-8">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<a href="/admin/products" class="text-xs text-muted-foreground hover:text-foreground">← Products</a>
			<h1 class="mt-2 text-3xl font-semibold tracking-tight">{data?.product.name ?? 'Product'}</h1>
			<p class="mt-1 font-mono text-xs text-muted-foreground">{data?.product.slug ?? id} · {data?.key?.key_prefix ?? 'no prefix'}</p>
		</div>
		<Button onclick={() => goto(`/keys?product=${id}`)}>Issue a key</Button>
	</div>

	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}

	{#if data}
		<div class="grid gap-6 lg:grid-cols-2">
			<Card>
				<CardHeader><CardTitle>Limits</CardTitle></CardHeader>
				<CardContent class="space-y-4">
					<div class="flex flex-wrap gap-2">
						{#each data.limits as item (item.id)}<Badge variant="secondary">{item.key}</Badge>{:else}<span class="text-sm text-muted-foreground">No limits defined.</span>{/each}
					</div>
					<form class="flex gap-2" onsubmit={addLimit}>
						<Input bind:value={limitKey} placeholder="max_accounts" required />
						<Button type="submit" variant="outline">Add</Button>
					</form>
				</CardContent>
			</Card>
			<Card>
				<CardHeader><CardTitle>Features</CardTitle></CardHeader>
				<CardContent class="space-y-4">
					<div class="flex flex-wrap gap-2">
						{#each data.features as item (item.id)}<Badge variant="secondary">{item.key}</Badge>{:else}<span class="text-sm text-muted-foreground">No features defined.</span>{/each}
					</div>
					<form class="flex gap-2" onsubmit={addFeature}>
						<Input bind:value={featureKey} placeholder="warmup" required />
						<Button type="submit" variant="outline">Add</Button>
					</form>
				</CardContent>
			</Card>
		</div>

		<Card>
			<CardHeader><CardTitle>Plan presets</CardTitle></CardHeader>
			<CardContent class="space-y-4">
				<div class="grid gap-2 sm:grid-cols-3">
					{#each data.presets as preset (preset.id)}
						<div class="rounded-lg border border-border p-3"><p class="text-sm font-medium">{preset.name}</p><p class="mt-1 text-xs text-muted-foreground">{preset.validity_days ?? '—'} days · {preset.max_activations} activation(s)</p></div>
					{:else}<p class="text-sm text-muted-foreground">No presets yet.</p>{/each}
				</div>
				<form class="grid gap-2 sm:grid-cols-4" onsubmit={addPreset}>
					<Input bind:value={presetName} placeholder="Preset name" required />
					<Input bind:value={presetLimits} placeholder="max_accounts: 25" />
					<Input bind:value={presetFeatures} placeholder="warmup" />
					<Button type="submit" variant="outline">Save preset</Button>
				</form>
			</CardContent>
		</Card>

		<Card>
			<CardHeader><CardTitle>Signing keys</CardTitle></CardHeader>
			<CardContent class="space-y-2 text-xs">
				<p class="text-muted-foreground">License public key</p>
				<code class="block overflow-auto rounded bg-muted p-2">{data.key?.license_public_key ?? 'not set'}</code>
				<p class="pt-2 text-muted-foreground">Update public key</p>
				<code class="block overflow-auto rounded bg-muted p-2">{data.key?.update_public_key ?? 'not set'}</code>
			</CardContent>
		</Card>
	{/if}
</div>
