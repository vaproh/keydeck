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
	let presetLimitValues = $state<Record<string, number>>({});
	let presetFeatureValues = $state<string[]>([]);
	let presetValidity = $state(30);
	let presetActivations = $state(1);
	let editingPreset = $state<{ id: number; name: string; limits: Record<string, number>; features: string[]; validity_days: number; max_activations: number } | null>(null);
	const id = $derived(page.params.id);

	async function load() {
		try {
			data = await api.get<Payload>(`/admin/products/${id}/overview`);
			const nextLimits: Record<string, number> = {};
			for (const item of data.limits) nextLimits[item.key] = presetLimitValues[item.key] ?? 0;
			presetLimitValues = nextLimits;
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
			name: presetName,
			limits: Object.fromEntries(Object.entries(presetLimitValues).filter(([, value]) => value > 0)),
			features: presetFeatureValues,
			validity_days: presetValidity, max_activations: presetActivations
		});
		presetName = '';
		presetFeatureValues = [];
		presetLimitValues = Object.fromEntries((data?.limits ?? []).map((item) => [item.key, 0]));
		await load();
	}
	function startPresetEdit(preset: Preset) {
		const limits: Record<string, number> = {};
		for (const item of data?.limits ?? []) limits[item.key] = preset.limits[item.key] ?? 0;
		for (const [key, value] of Object.entries(preset.limits)) {
			if (!(key in limits)) limits[key] = value;
		}
		editingPreset = {
			id: preset.id, name: preset.name, limits,
			features: [...preset.features], validity_days: preset.validity_days ?? 30,
			max_activations: preset.max_activations
		};
	}
	async function savePreset(event: SubmitEvent) {
		event.preventDefault();
		if (!editingPreset) return;
		await api.patch(`/admin/products/${id}/presets/${editingPreset.id}`, {
			name: editingPreset.name,
			limits: Object.fromEntries(Object.entries(editingPreset.limits).filter(([, value]) => value > 0)),
			features: editingPreset.features, validity_days: editingPreset.validity_days,
			max_activations: editingPreset.max_activations
		});
		editingPreset = null;
		await load();
	}
	async function deletePreset(presetID: number) {
		if (!confirm('Delete this plan preset?')) return;
		await api.del(`/admin/products/${id}/presets/${presetID}`);
		await load();
	}
	async function removeLimit(limitID: number) {
		await api.del(`/admin/products/${id}/limits/${limitID}`);
		await load();
	}
	async function removeFeature(featureID: number) {
		await api.del(`/admin/products/${id}/features/${featureID}`);
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
			<Button variant="outline" onclick={() => goto(`/admin/keys?product=${id}`)}>Issue a key</Button>
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
								<TableRow class="cursor-pointer" onclick={() => goto(`/admin/keys/${key.id}`)}>
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
						<div class="flex flex-wrap gap-2">{#each data.limits as item (item.id)}<span class="inline-flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs"><span>{item.key}</span><button class="text-muted-foreground hover:text-destructive" onclick={() => removeLimit(item.id)} aria-label={`Delete ${item.key}`}>×</button></span>{:else}<span class="text-sm text-muted-foreground">No limits defined.</span>{/each}</div>
						<form class="flex gap-2" onsubmit={addLimit}><Input bind:value={limitKey} placeholder="max_accounts" required /><Button type="submit" variant="outline">Add</Button></form>
					</CardContent>
				</Card>
				<Card>
					<CardHeader><CardTitle>Features</CardTitle></CardHeader>
					<CardContent class="space-y-4">
						<div class="flex flex-wrap gap-2">{#each data.features as item (item.id)}<span class="inline-flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs"><span>{item.key}</span><button class="text-muted-foreground hover:text-destructive" onclick={() => removeFeature(item.id)} aria-label={`Delete ${item.key}`}>×</button></span>{:else}<span class="text-sm text-muted-foreground">No features defined.</span>{/each}</div>
						<form class="flex gap-2" onsubmit={addFeature}><Input bind:value={featureKey} placeholder="warmup" required /><Button type="submit" variant="outline">Add</Button></form>
					</CardContent>
				</Card>
			</div>
			<Card>
				<CardHeader><CardTitle>Plan presets</CardTitle></CardHeader>
				<CardContent class="space-y-4">
					<div class="grid gap-2 sm:grid-cols-3">
						{#each data.presets as preset (preset.id)}
							{#if editingPreset?.id === preset.id}
								<form class="space-y-2 rounded-lg border border-accent/40 p-3" onsubmit={savePreset}>
									<Input bind:value={editingPreset.name} placeholder="Preset name" required />
									<div class="grid grid-cols-2 gap-2">
										<Input type="number" min="1" bind:value={editingPreset.validity_days} placeholder="Days" />
										<Input type="number" min="1" bind:value={editingPreset.max_activations} placeholder="Activations" />
									</div>
									{#if data.limits.length}
										<div class="space-y-1">
											<p class="text-xs font-medium">Limits</p>
											<div class="grid grid-cols-2 gap-2">
												{#each data.limits as item (item.id)}
													<label class="space-y-1 text-[11px] text-muted-foreground">{item.key}<Input type="number" min="0" bind:value={editingPreset.limits[item.key]} /></label>
												{/each}
											</div>
										</div>
									{/if}
									{#if data.features.length}
										<div class="space-y-1">
											<p class="text-xs font-medium">Features</p>
											<div class="flex flex-wrap gap-2">
												{#each data.features as item (item.id)}
													<label class="inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-1 text-[11px]"><input type="checkbox" value={item.key} bind:group={editingPreset.features} /> {item.key}</label>
												{/each}
											</div>
										</div>
									{/if}
									<div class="flex gap-2"><Button type="submit" variant="outline">Save</Button><Button type="button" variant="ghost" onclick={() => (editingPreset = null)}>Cancel</Button></div>
								</form>
							{:else}
								<div class="rounded-lg border border-border p-3">
									<div class="flex items-start justify-between gap-2"><div><p class="text-sm font-medium">{preset.name}</p><p class="mt-1 text-xs text-muted-foreground">{preset.validity_days ?? '—'} days · {preset.max_activations} activation(s)</p></div><div class="flex gap-1"><button class="text-xs text-muted-foreground hover:text-foreground" onclick={() => startPresetEdit(preset)}>Edit</button><button class="text-xs text-destructive hover:underline" onclick={() => deletePreset(preset.id)}>Delete</button></div></div>
									<p class="mt-2 text-xs text-muted-foreground">Limits {Object.entries(preset.limits).map(([key, value]) => `${key}: ${value}`).join(' · ') || 'none'}</p>
									<p class="text-xs text-muted-foreground">Features {preset.features.join(', ') || 'none'}</p>
								</div>
							{/if}
						{:else}<p class="text-sm text-muted-foreground">No presets yet.</p>{/each}
					</div>
					<form class="space-y-3 rounded-lg border border-border p-3" onsubmit={addPreset}>
						<div class="grid gap-2 sm:grid-cols-3">
							<Input bind:value={presetName} placeholder="Preset name" required />
							<Input type="number" min="1" bind:value={presetValidity} placeholder="Days" />
							<Input type="number" min="1" bind:value={presetActivations} placeholder="Activations" />
						</div>
						{#if data.limits.length}
							<div class="space-y-1">
								<p class="text-xs font-medium">Limits</p>
								<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
									{#each data.limits as item (item.id)}
										<label class="space-y-1 text-[11px] text-muted-foreground">{item.key}<Input type="number" min="0" bind:value={presetLimitValues[item.key]} /></label>
									{/each}
								</div>
							</div>
						{/if}
						{#if data.features.length}
							<div class="space-y-1">
								<p class="text-xs font-medium">Features</p>
								<div class="flex flex-wrap gap-2">
									{#each data.features as item (item.id)}
										<label class="inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-1 text-[11px]"><input type="checkbox" value={item.key} bind:group={presetFeatureValues} /> {item.key}</label>
									{/each}
								</div>
							</div>
						{/if}
						<Button type="submit" variant="outline">Save preset</Button>
					</form>
				</CardContent>
			</Card>
			<Card>
				<CardHeader><CardTitle>Signing keys</CardTitle></CardHeader>
				<CardContent class="space-y-2 text-xs"><p class="text-muted-foreground">License public key</p><code class="block overflow-auto rounded bg-muted p-2">{data.key?.license_public_key ?? 'not set'}</code><p class="pt-2 text-muted-foreground">Update public key</p><code class="block overflow-auto rounded bg-muted p-2">{data.key?.update_public_key ?? 'not set'}</code></CardContent>
			</Card>
		{/if}
	{/if}
</div>
