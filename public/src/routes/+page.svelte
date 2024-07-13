<script lang="ts">
	import { onMount } from 'svelte';

	let users = {};

	const formatBytes = (totalBytes: number, space = true) => {
		if (!totalBytes) return `00.00${space ? ' ' : ''}KB`;
		const totalKilos = totalBytes / 1024;
		const totalMegas = totalKilos / 1000;
		const totalGigas = totalMegas / 1000;
		const totalTeras = totalGigas / 1000;
		if (totalKilos < 100)
			return `${totalKilos < 10 ? '0' : ''}${totalKilos.toFixed(2)}${space ? ' ' : ''}KB`;
		if (totalMegas < 100)
			return `${totalMegas < 10 ? '0' : ''}${totalMegas.toFixed(2)}${space ? ' ' : ''}MB`;
		if (totalGigas < 100)
			return `${totalGigas < 10 ? '0' : ''}${totalGigas.toFixed(2)}${space ? ' ' : ''}GB`;
		return `${totalTeras < 10 ? '0' : ''}${totalTeras.toFixed(2)}${space ? ' ' : ''}TB`;
	};

	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

	onMount(async () => {
		try {
			while (true) {
				const res = await fetch('http://87.236.215.219:10800/api/users');
				const data = await res.json();
				console.log(data);
				users = data;
				await sleep(1000);
			}
		} catch (error) {
			console.log(error);
		}
	});
</script>

<div class="bg-neutral-950 text-neutral-50 min-h-svh w-full p-4">
	<!-- svelte-ignore empty-block -->
	{#each Object.entries(users) as entries}
		<div class="flex rounded border border-neutral-800 my-4 px-4 py-2">
			<div>{entries[0]}</div>
			:
			<div>{formatBytes(Number(entries[1]))}</div>
		</div>
	{/each}
</div>
