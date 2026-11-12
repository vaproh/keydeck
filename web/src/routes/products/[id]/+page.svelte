<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '#lib/api';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Label } from '#lib/components/ui/label';
	import { Card, CardContent, CardHeader, CardTitle } from '#lib/components/ui/card';
	import { Badge } from '#lib/components/ui/badge';
	import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#lib/components/ui/table';

	type Product = { id: number; slug: string; name: string; status: string; created_at: string };
	type Item = { id: number; key: string; label: string };
	type Preset = { id: number; name: string; limits: Record<string, number>; features: string[]; validity_days: number | null; max_activations: number };
	type RecentKey = { id: number; plan_name: string; holder_name: string; key_last4: string; display_status: string; activation_count: number; max_activations: number };
	type Release = { id: number; version: string; channel: string; platform: string; status: string; created_at: string };
	type Payload = {
		product: Product;
		key: { key_prefix: string; license_public_key: string; update_public_key: string } | null;
		limits: Item[]; features: Item[]; presets: Preset[];
		counts: { keys: number; active_keys: number; activations: number; releases: number };
		recent_keys: RecentKey[]; recent_releases: Release[];
	};

	let data = $state<Payload | null>(null);
	let error = $state('');
	let tab = $state('overview');
	let limitKey = $state('');
	let featureKey = $state('');
	let presetName = $state('');
	let presetLimits = $state('{"max_accounts": 25}');
	let presetFeatures = $state('["warmup"]');
	const id = $derived(page.params.id);

	async function load() {
		try {
			data = await api.get<Payload>(`/admin/products/${id}/overview`);
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
		<div class="flex gap-2">
			<Button variant="outline" onclick={() => goto(`/keys?product=${id}`)}>Issue a key</Button>
			<Badge variant={data?.product.status === 'active' ? 'secondary' : 'outline'}>{data?.product.status ?? '—'}</Badge>
		</div>
	</div>

	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}

	{#if data}
		<div class="flex flex-wrap gap-1 rounded-lg border border-border bg-card/60 p-1">
			{#each [['overview', 'Overview'], ['keys', 'Keys'], ['releases', 'Releases'], ['settings', 'Settings']] as [value, label] (value)}
				<button class="rounded-md px-3 py-1.5 text-sm transition-colors {tab === value ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-muted hover:text-foreground'}" onclick={() => (tab = value)}>{label}</button>
			{/each}
		</div>

		{#if tab === 'overview'}
			<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
				<Card><CardHeader><CardTitle class="text-xs font-medium text-muted-foreground">Total keys</CardTitle></CardHeader><CardContent><p class="text-2xl font-semibold">{data.counts.keys}</p></CardContent></Card>
				<Card><CardHeader><CardTitle class="text-xs font-medium text-muted-foreground">Active keys</CardTitle></CardHeader><CardContent><p class="text-2xl font-semibold">{data.counts.active_keys}</p></CardContent></Card>
				<Card><CardHeader><CardTitle class="text-xs font-medium text-muted-foreground">Activations</CardTitle></CardHeader><CardContent><p class="text-2xl font-semibold">{data.counts.activations}</p></CardContent></Card>
				<Card><CardHeader><CardTitle class="text-xs font-medium text-muted-foreground">Releases</CardTitle></CardHeader><CardContent><p class="text-2xl font-semibold">{data.counts.releases}</p></CardContent></Card>
			</div>
			<div class="grid gap-6 xl:grid-cols-2">
				<Card>
					<CardHeader><CardTitle>Recent keys</CardTitle></CardHeader>
					<CardContent>
						<Table>
							<TableHeader><TableRow><TableHead>Plan</TableHead><TableHead>Holder</TableHead><TableHead>Status</TableHead></TableRow></TableHeader>
							<TableBody>
								{#each data.recent_keys as key (key.id)}
									<TableRow><TableCell><a class="hover:underline" href={`/admin/keys/${key.id}`}>{key.plan_name}</a></TableCell><TableCell class="text-xs">{key.holder_name || '—'}</TableCell><TableCell><Badge variant={key.display_status === 'active' ? 'secondary' : 'outline'}>{key.display_status}</Badge></TableCell></TableRow>
								{:else}<TableRow><TableCell colspan={3} class="py-8 text-center text-sm text-muted-foreground">No keys yet.</TableCell></TableRow>{/each}
							</TableBody>
						</Table>
					</CardContent>
				</Card>
				<Card>
					<CardHeader><CardTitle>Recent releases</CardTitle></CardHeader>
					<CardContent>
						<Table>
							<TableHeader><TableRow><TableHead>Version</TableHead><TableHead>Channel</TableHead><TableHead>Status</TableHead></TableRow></TableHeader>
							<TableBody>
								{#each data.recent_releases as release (release.id)}
									<TableRow><TableCell class="font-mono text-xs">{release.version}</TableCell><TableCell><Badge variant="outline">{release.channel}</Badge></TableCell><TableCell class="text-xs">{release.status}</TableCell></TableRow>
								{:else}<TableRow><TableCell colspan={3} class="py-8 text-center text-sm text-muted-foreground">No releases yet.</TableCell></TableRow>{/each}
							</TableBody>
						</Table>
					</CardContent>
				</Card>
			</div>
		{:else if tab === 'keys'}
			<Card>
				<CardHeader><CardTitle>Keys</CardTitle></CardHeader>
				<CardContent>
					<Table>
						<TableHeader><TableRow><TableHead>Plan</TableHead><TableHead>Holder</TableHead><TableHead>Key</TableHead><TableHead>Activations</TableHead><TableHead>Status</TableHead></TableRow></TableHeader>
						<TableBody>
							{#each data.recent_keys as key (key.id)}
								<TableRow class="cursor-pointer" onclick={() => goto(`/keys/${key.id}`)}>
									<TableCell class="font-medium">{key.plan_name}</TableCell><TableCell class="text-xs">{key.holder_name || '—'}</TableCell>
									<TableCell class="font-mono text-xs text-muted-foreground">•••• {key.key_last4}</TableCell>
									<TableCell class="text-xs">{key.activation_count}/{key.max_activations}</TableCell>
									<TableCell><Badge variant={key.display_status === 'active' ? 'secondary' : 'outline'}>{key.display_status}</Badge></TableCell>
								</TableRow>
							{:else}<TableRow><TableCell colspan={5} class="py-8 text-center text-sm text-muted-foreground">No keys yet.</TableCell></TableRow>{/each}
						</TableBody>
					</Table>
				</CardContent>
			</Card>
		{:else if tab === 'releases'}
			<Card>
				<CardHeader><CardTitle>Releases</CardTitle></CardHeader>
				<CardContent>
					<Table>
						<TableHeader><TableRow><TableHead>Version</TableHead><TableHead>Channel</TableHead><TableHead>Platform</TableHead><TableHead>Status</TableHead></TableRow></TableHeader>
						<TableBody>
							{#each data.recent_releases as release (release.id)}
								<TableRow><TableCell class="font-mono text-xs">{release.version}</TableCell><TableCell><Badge variant="outline">{release.channel}</Badge></TableCell><TableCell class="text-xs">{release.platform}</TableCell><TableCell class="text-xs">{release.status}</TableCell></TableRow>
							{:else}<TableRow><TableCell colspan={4} class="py-8 text-center text-sm text-muted-foreground">No releases yet.</TableCell></TableRow>{/each}
						</TableBody>
					</Table>
				</CardContent>
			</Card>
		{:else}
			<div class="grid gap-6 lg:grid-cols-2">
				<Card>
					<CardHeader><CardTitle>Limits</CardTitle></CardHeader>
					<CardContent class="space-y-4">
						<div class="flex flex-wrap gap-2">{#each data.limits as item (item.id)}<Badge variant="secondary">{item.key}</Badge>{:else}<span class="text-sm text-muted-foreground">No limits defined.</span>{/each}</div>
						<form class="flex gap-2" onsubmit={addLimit}><Input bind:value={limitKey} placeholder="max_accounts" required /><Button type="submit" variant="outline">Add</Button></form>
					</CardContent>
				</Card>
				<Card>
					<CardHeader><CardTitle>Features</CardTitle></CardHeader>
					<CardContent class="space-y-4">
						<div class="flex flex-wrap gap-2">{#each data.features as item (item.id)}<Badge variant="secondary">{item.key}</Badge>{:else}<span class="text-sm text-muted-foreground">No features defined.</span>{/each}</div>
						<form class="flex gap-2" onsubmit={addFeature}><Input bind:value={featureKey} placeholder="warmup" required /><Button type="submit" variant="outline">Add</Button></form>
					</CardContent>
				</Card>
			</div>
			<Card>
				<CardHeader><CardTitle>Plan presets</CardTitle></CardHeader>
				<CardContent class="space-y-4">
					<div class="grid gap-2 sm:grid-cols-3">
						{#each data.presets as preset (preset.id)}
							<div class="rounded-lg border border-border p-3"><p class="text-sm font-medium">{preset.name}</p><p class="mt-1 text-xs text-muted-foreground">{preset.validity_days ?? '—'} days · {preset.max_activations} activation(s)</p><p class="mt-2 text-xs text-muted-foreground">Limits {JSON.stringify(preset.limits)}</p><p class="text-xs text-muted-foreground">Features {preset.features.join(', ') || 'none'}</p></div>
						{:else}<p class="text-sm text-muted-foreground">No presets yet.</p>{/each}
					</div>
					<form class="grid gap-2 sm:grid-cols-4" onsubmit={addPreset}><Input bind:value={presetName} placeholder="Preset name" required /><Input bind:value={presetLimits} placeholder="max_accounts: 25" /><Input bind:value={presetFeatures} placeholder="warmup" /><Button type="submit" variant="outline">Save preset</Button></form>
				</CardContent>
			</Card>
			<Card>
				<CardHeader><CardTitle>Signing keys</CardTitle></CardHeader>
				<CardContent class="space-y-2 text-xs"><p class="text-muted-foreground">License public key</p><code class="block overflow-auto rounded bg-muted p-2">{data.key?.license_public_key ?? 'not set'}</code><p class="pt-2 text-muted-foreground">Update public key</p><code class="block overflow-auto rounded bg-muted p-2">{data.key?.update_public_key ?? 'not set'}</code></CardContent>
			</Card>
		{/if}
	{/if}
</div>
