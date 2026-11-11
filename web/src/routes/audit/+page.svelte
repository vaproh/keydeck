<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api';
	import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#lib/components/ui/table';

	type Event = { id: number; actor: string; action: string; target: string; metadata: unknown; created_at: string };
	let events = $state<Event[]>([]);
	let error = $state('');

	onMount(async () => {
		try {
			const data = await api.get<{ events: Event[] }>('/admin/audit');
			events = data.events;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load audit.';
		}
	});
</script>

<div class="space-y-8">
	<div>
		<p class="text-[11px] uppercase tracking-[0.2em] text-muted-foreground">History</p>
		<h1 class="mt-1 text-3xl font-semibold tracking-tight">Audit</h1>
		<p class="mt-1 text-sm text-muted-foreground">Key issuance, resets, revocations and releases.</p>
	</div>
	{#if error}<div class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>{/if}
	<section class="rounded-xl border border-border bg-card/80">
		<Table>
			<TableHeader><TableRow><TableHead>When</TableHead><TableHead>Actor</TableHead><TableHead>Action</TableHead><TableHead>Target</TableHead></TableRow></TableHeader>
			<TableBody>
				{#each events as event (event.id)}
					<TableRow>
						<TableCell class="whitespace-nowrap text-xs text-muted-foreground">{new Date(event.created_at).toLocaleString()}</TableCell>
						<TableCell class="text-xs">{event.actor}</TableCell>
						<TableCell class="font-mono text-xs">{event.action}</TableCell>
						<TableCell class="max-w-64 truncate text-xs text-muted-foreground">{event.target || '—'}</TableCell>
					</TableRow>
				{:else}
					<TableRow><TableCell colspan={4} class="py-10 text-center text-sm text-muted-foreground">No audit events yet.</TableCell></TableRow>
				{/each}
			</TableBody>
		</Table>
	</section>
</div>
