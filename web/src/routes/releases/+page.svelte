<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Label } from '#lib/components/ui/label';
	import { Card, CardContent, CardHeader, CardTitle } from '#lib/components/ui/card';
	import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#lib/components/ui/table';
	import { Badge } from '#lib/components/ui/badge';
	import { Upload, RadioTower } from '@lucide/svelte';

	type Product = { id: number; slug: string; name: string };
	type Release = { id: number; product_id: number; version: string; channel: string; platform: string; status: string; sha256: string; notes: string; created_at: string };
	let products = $state<Product[]>([]);
	let releases = $state<Release[]>([]);
	let error = $state('');
	let message = $state('');
	let busy = $state(false);
	let file = $state<File | null>(null);
	let form = $state({ product_id: '', version: '', channel: 'stable', platform: 'windows-amd64', min_version: '', notes: '', mandatory: false, security: false });

	async function load() {
		try {
			const [p, r] = await Promise.all([
				api.get<{ products: Product[] }>('/admin/products'),
				api.get<{ releases: Release[] }>('/admin/releases')
			]);
			products = p.products;
			releases = r.releases;
			if (!form.product_id && products[0]) form.product_id = String(products[0].id);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load releases.';
		}
	}

	onMount(load);

	async function upload(event: SubmitEvent) {
		event.preventDefault();
		if (!file || busy) return;
		busy = true;
		error = '';
		message = '';
		try {
			const data = new FormData();
			for (const [key, value] of Object.entries(form)) data.set(key, String(value));
			data.set('artifact', file);
			await api.upload('/admin/releases', data);
			message = `Published ${form.version}.`;
			form.version = '';
			form.notes = '';
			file = null;
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not publish release.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<div>
		<p class="text-[11px] uppercase tracking-[0.2em] text-muted-foreground">Distribution</p>
		<h1 class="mt-1 text-3xl font-semibold tracking-tight">Releases</h1>
		<p class="mt-1 text-sm text-muted-foreground">Signed manifests for the stable and beta channels.</p>
	</div>

	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}
	{#if message}<div class="rounded-lg border border-primary/30 bg-primary/10 p-4 text-sm">{message}</div>{/if}

	<div class="grid gap-6 xl:grid-cols-[1fr_380px]">
		<section class="rounded-xl border border-border bg-card/80">
			<Table>
				<TableHeader><TableRow><TableHead>Version</TableHead><TableHead>Channel</TableHead><TableHead>Platform</TableHead><TableHead>Status</TableHead><TableHead>SHA-256</TableHead><TableHead class="text-right">Published</TableHead></TableRow></TableHeader>
				<TableBody>
					{#each releases as release (release.id)}
						<TableRow>
							<TableCell class="font-mono text-xs">{release.version}</TableCell>
							<TableCell><Badge variant="outline">{release.channel}</Badge></TableCell>
							<TableCell class="text-xs">{release.platform}</TableCell>
							<TableCell><Badge variant={release.status === 'published' ? 'secondary' : 'outline'}>{release.status}</Badge></TableCell>
							<TableCell class="max-w-32 truncate font-mono text-[11px] text-muted-foreground">{release.sha256.slice(0, 16)}…</TableCell>
							<TableCell class="text-right text-xs text-muted-foreground">{new Date(release.created_at).toLocaleDateString()}</TableCell>
						</TableRow>
					{:else}
						<TableRow><TableCell colspan={6} class="py-10 text-center text-sm text-muted-foreground">No releases published.</TableCell></TableRow>
					{/each}
				</TableBody>
			</Table>
		</section>

		<Card>
			<CardHeader><CardTitle class="flex items-center gap-2"><RadioTower class="size-4" /> Publish release</CardTitle></CardHeader>
			<CardContent>
				<form class="space-y-3" onsubmit={upload}>
					<div class="space-y-1.5"><Label for="product">Product</Label>
						<select id="product" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm" bind:value={form.product_id}>
							{#each products as product (product.id)}<option value={String(product.id)}>{product.name}</option>{/each}
						</select>
					</div>
					<div class="grid grid-cols-2 gap-2">
						<div class="space-y-1.5"><Label for="version">Version</Label><Input id="version" bind:value={form.version} placeholder="1.0.0" required /></div>
						<div class="space-y-1.5"><Label for="channel">Channel</Label>
							<select id="channel" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm" bind:value={form.channel}><option value="stable">Stable</option><option value="beta">Beta</option></select>
						</div>
					</div>
					<div class="space-y-1.5"><Label for="platform">Platform</Label><Input id="platform" bind:value={form.platform} /></div>
					<div class="space-y-1.5"><Label for="notes">Release notes</Label><textarea id="notes" class="min-h-20 w-full rounded-md border border-input bg-background p-2 text-sm" bind:value={form.notes}></textarea></div>
					<label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={form.mandatory} /> Mandatory update</label>
					<label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={form.security} /> Security update</label>
					<Input type="file" accept=".zip" onchange={(e: Event) => { const input = e.currentTarget as HTMLInputElement; file = input.files?.[0] ?? null; }} required />
					<Button class="w-full" type="submit" disabled={busy || !file}>{busy ? 'Uploading…' : 'Publish'}<Upload class="size-4" /></Button>
				</form>
			</CardContent>
		</Card>
	</div>
</div>
