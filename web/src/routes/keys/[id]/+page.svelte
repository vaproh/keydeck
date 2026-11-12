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
	import { ArrowLeft, Clock3, KeyRound, RotateCcw, ShieldOff, Trash2 } from '@lucide/svelte';

	type License = {
		id: number; product_id: number; product_slug?: string; key_last4: string; plan_name: string;
		holder_name: string; key_type: string; status: string; display_status: string;
		expires_at: string; activation_expires_at: string | null; first_activated_at: string | null;
		max_activations: number; activation_count: number; lease_ttl_seconds: number;
		grace_seconds: number; limits: Record<string, number>; features: string[];
		notes: string; created_at: string;
	};
	type Activation = {
		id: number; installation_hash: string; machine_hash: string; app_version: string;
		ip: string; last_seen_at: string; created_at: string; revoked_at: string | null;
	};
	type Payload = { license: License; activations: Activation[] };

	let data = $state<Payload | null>(null);
	let error = $state('');
	let saving = $state(false);
	let form = $state({ holder_name: '', plan_name: '', max_activations: 1, lease_ttl_hours: 24, limits: '{}', features: '[]' });
	const id = $derived(page.params.id);

	async function load() {
		try {
			data = await api.get<Payload>(`/admin/licenses/${id}`);
			form = {
				holder_name: data.license.holder_name,
				plan_name: data.license.plan_name,
				max_activations: data.license.max_activations,
				lease_ttl_hours: Math.round((data.license.lease_ttl_seconds / 3600) * 10) / 10,
				limits: JSON.stringify(data.license.limits ?? {}, null, 2),
				features: JSON.stringify(data.license.features ?? [], null, 2)
			};
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load key.';
		}
	}
	onMount(load);

	async function save(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		error = '';
		try {
			await api.patch(`/admin/licenses/${id}`, {
				holder_name: form.holder_name, plan_name: form.plan_name,
				max_activations: form.max_activations,
				lease_ttl_seconds: Math.max(60, Math.round(form.lease_ttl_hours * 3600)),
				limits: JSON.parse(form.limits), features: JSON.parse(form.features)
			});
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not save key.';
		} finally {
			saving = false;
		}
	}

	async function action(kind: string) {
		if (kind === 'revoke' && !confirm('Revoke this key? Clients will be locked out on the next heartbeat.')) return;
		try {
			await api.post(`/admin/licenses/${id}/${kind}`, kind === 'extend' ? { days: 30 } : undefined);
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Action failed.';
		}
	}

	async function revokeActivation(activationID: number) {
		if (!confirm('Revoke this machine activation?')) return;
		try {
			await api.post(`/admin/licenses/${id}/activations/${activationID}/revoke`);
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not revoke activation.';
		}
	}

	function when(value: string | null) {
		return value ? new Date(value).toLocaleString() : '—';
	}
	function remaining() {
		const l = data?.license;
		if (!l) return '—';
		if (l.key_type === 'lifetime') return 'Never';
		const end = new Date(l.activation_expires_at ?? l.expires_at).getTime();
		const diff = Math.max(0, end - Date.now());
		const hours = Math.floor(diff / 3_600_000);
		return hours >= 48 ? `${Math.floor(hours / 24)}d ${hours % 24}h` : `${hours}h`;
	}
	function short(value: string) {
		return value ? `${value.slice(0, 12)}…` : '—';
	}
</script>

<div class="space-y-8">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<a href="/admin/keys" class="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"><ArrowLeft class="size-3.5" /> Keys</a>
			<h1 class="mt-2 text-3xl font-semibold tracking-tight">{data?.license.plan_name ?? 'Key'}</h1>
			<p class="mt-1 text-sm text-muted-foreground">
				{data?.license.holder_name || 'Unassigned'} · <span class="font-mono">•••• {data?.license.key_last4 ?? ''}</span>
			</p>
		</div>
		{#if data}
			<div class="flex items-center gap-2">
				<Badge variant={data.license.display_status === 'active' ? 'secondary' : 'outline'}>{data.license.display_status}</Badge>
				<Badge variant="outline">{data.license.key_type}</Badge>
			</div>
		{/if}
	</div>

	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}

	{#if data}
		<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
			<Card><CardHeader><CardTitle class="text-xs font-medium text-muted-foreground">Remaining</CardTitle></CardHeader><CardContent><p class="text-2xl font-semibold">{remaining()}</p></CardContent></Card>
			<Card><CardHeader><CardTitle class="text-xs font-medium text-muted-foreground">Activations</CardTitle></CardHeader><CardContent><p class="text-2xl font-semibold">{data.license.activation_count}/{data.license.max_activations}</p></CardContent></Card>
			<Card><CardHeader><CardTitle class="text-xs font-medium text-muted-foreground">Lease TTL</CardTitle></CardHeader><CardContent><p class="text-2xl font-semibold">{Math.round(data.license.lease_ttl_seconds / 360) / 10}h</p></CardContent></Card>
			<Card><CardHeader><CardTitle class="text-xs font-medium text-muted-foreground">Product</CardTitle></CardHeader><CardContent><p class="text-lg font-semibold">{data.license.product_slug ?? data.license.product_id}</p></CardContent></Card>
		</div>

		<div class="grid gap-6 xl:grid-cols-[1fr_420px]">
			<div class="space-y-6">
				<Card>
					<CardHeader><CardTitle class="flex items-center gap-2"><Clock3 class="size-4" /> Clock</CardTitle></CardHeader>
					<CardContent class="grid gap-3 text-sm sm:grid-cols-2">
						<div><p class="text-xs text-muted-foreground">Issued</p><p>{when(data.license.created_at)}</p></div>
						<div><p class="text-xs text-muted-foreground">First activated</p><p>{when(data.license.first_activated_at)}</p></div>
						<div><p class="text-xs text-muted-foreground">Activation expiry</p><p>{when(data.license.activation_expires_at)}</p></div>
						<div><p class="text-xs text-muted-foreground">Absolute expiry</p><p>{data.license.key_type === 'lifetime' ? 'Never' : when(data.license.expires_at)}</p></div>
						<div><p class="text-xs text-muted-foreground">Grace</p><p>{data.license.grace_seconds}s</p></div>
						<div><p class="text-xs text-muted-foreground">Stored status</p><p>{data.license.status}</p></div>
					</CardContent>
				</Card>

				<Card>
					<CardHeader><CardTitle>Activations</CardTitle></CardHeader>
					<CardContent>
						<Table>
							<TableHeader><TableRow><TableHead>Installation</TableHead><TableHead>Version</TableHead><TableHead>Last seen</TableHead><TableHead></TableHead></TableRow></TableHeader>
							<TableBody>
								{#each data.activations as activation (activation.id)}
									<TableRow>
										<TableCell class="font-mono text-xs">{short(activation.installation_hash)}</TableCell>
										<TableCell class="text-xs">{activation.app_version || '—'}</TableCell>
										<TableCell class="text-xs text-muted-foreground">{when(activation.last_seen_at)}</TableCell>
										<TableCell class="text-right">
											{#if activation.revoked_at}<Badge variant="outline">revoked</Badge>
											{:else}<button class="rounded px-2 py-1 text-xs text-destructive hover:bg-destructive/10" onclick={() => revokeActivation(activation.id)}>Revoke</button>{/if}
										</TableCell>
									</TableRow>
								{:else}<TableRow><TableCell colspan={4} class="py-8 text-center text-sm text-muted-foreground">Never activated.</TableCell></TableRow>{/each}
							</TableBody>
						</Table>
					</CardContent>
				</Card>

				<Card>
					<CardHeader><CardTitle>Limits and features</CardTitle></CardHeader>
					<CardContent class="grid gap-4 sm:grid-cols-2">
						<div><p class="mb-2 text-xs uppercase tracking-wider text-muted-foreground">Limits</p><pre class="overflow-auto rounded bg-muted p-3 text-xs">{JSON.stringify(data.license.limits, null, 2)}</pre></div>
						<div><p class="mb-2 text-xs uppercase tracking-wider text-muted-foreground">Features</p><pre class="overflow-auto rounded bg-muted p-3 text-xs">{JSON.stringify(data.license.features, null, 2)}</pre></div>
					</CardContent>
				</Card>
			</div>

			<div class="space-y-6">
				<Card>
					<CardHeader><CardTitle class="flex items-center gap-2"><KeyRound class="size-4" /> Edit key</CardTitle></CardHeader>
					<CardContent>
						<form class="space-y-3" onsubmit={save}>
							<div class="space-y-1.5"><Label for="holder">Assigned to</Label><Input id="holder" bind:value={form.holder_name} maxlength={120} /></div>
							<div class="space-y-1.5"><Label for="plan">Plan name</Label><Input id="plan" bind:value={form.plan_name} /></div>
							<div class="grid grid-cols-2 gap-2">
								<div class="space-y-1.5"><Label for="acts">Activations</Label><Input id="acts" type="number" min="1" bind:value={form.max_activations} /></div>
								<div class="space-y-1.5"><Label for="ttl">Lease TTL (hours)</Label><Input id="ttl" type="number" min="0.1" max="720" step="0.1" bind:value={form.lease_ttl_hours} /></div>
							</div>
							<div class="space-y-1.5"><Label for="limits">Limits JSON</Label><textarea id="limits" class="min-h-24 w-full rounded-md border border-input bg-background p-2 font-mono text-xs" bind:value={form.limits}></textarea></div>
							<div class="space-y-1.5"><Label for="features">Features JSON</Label><textarea id="features" class="min-h-20 w-full rounded-md border border-input bg-background p-2 font-mono text-xs" bind:value={form.features}></textarea></div>
							<Button class="w-full" type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save changes'}</Button>
						</form>
					</CardContent>
				</Card>

				<Card>
					<CardHeader><CardTitle>Actions</CardTitle></CardHeader>
					<CardContent class="flex flex-wrap gap-2">
						<Button variant="outline" onclick={() => action('extend')}><Clock3 class="size-4" /> +30 days</Button>
						<Button variant="outline" onclick={() => action('reset')}><RotateCcw class="size-4" /> Reset</Button>
						<Button variant="destructive" onclick={() => action('revoke')}><ShieldOff class="size-4" /> Revoke key</Button>
					</CardContent>
				</Card>

				<Card>
					<CardHeader><CardTitle class="flex items-center gap-2"><Trash2 class="size-4" /> Notes</CardTitle></CardHeader>
					<CardContent><p class="whitespace-pre-wrap text-sm text-muted-foreground">{data.license.notes || 'No notes.'}</p></CardContent>
				</Card>
			</div>
		</div>
	{/if}
</div>
