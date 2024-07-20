<script lang="ts">
	import { onMount } from 'svelte';

	let currentUser: User | null = null;
	let users: User[] = [];
	let publicAddress = '';
	let editing = false;

	interface User {
		username: string;
		password: string;
		totalUsage: number;
		allowedUsage: number;
		expiresAt: number;
		locked: boolean;
	}

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

	const formatExpiry = (expiresAt: number, noPrefix = false) => {
		if (!expiresAt) return 'unknown';
		let totalSeconds = Math.trunc(expiresAt - Date.now() / 1000);
		const prefix = totalSeconds < 0 && !noPrefix ? '-' : '';
		totalSeconds = Math.abs(totalSeconds);
		if (totalSeconds / 60 < 1) return `${prefix}${totalSeconds} seconds`;
		const totalMinutes = Math.trunc(totalSeconds / 60);
		if (totalMinutes / 60 < 1) return `${prefix}${totalMinutes} minutes`;
		const totalHours = Math.trunc(totalMinutes / 60);
		if (totalHours / 24 < 1) return `${prefix}${totalHours} hours`;
		return `${prefix}${Math.trunc(totalHours / 24)} days`;
	};

	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

	onMount(async () => {
		try {
			const publicAddressRes = await fetch('/api/public-address');
			publicAddress = await publicAddressRes.text();
			while (true) {
				if (currentUser === null) {
					const res = await fetch('/api/users');
					const data: { string: User } = await res.json();
					users = Object.values(data);
				} else {
					const res = await fetch('/api/users/' + currentUser.username);
					currentUser = await res.json();
				}
				await sleep(1000);
			}
		} catch (error) {
			console.log(error);
		}
	});

	async function deleteUser(username: string) {
		try {
			if (window.confirm(`Delete ${username}?`)) {
				const res = await fetch('/api/users', {
					method: 'DELETE',
					headers: { 'content-type': 'application/json' },
					body: JSON.stringify({ username })
				});
				console.log(res.status);
			}
		} catch (error) {
			console.log(error);
		}
	}

	async function editUser(
		expiresAt: number | undefined,
		allowedUsage: number | undefined,
		totalUsage: number | undefined
	) {
		try {
			const res = await fetch('/api/users', {
				method: 'PATCH',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ expiresAt, allowedUsage, totalUsage })
			});
			console.log(res.status);
		} catch (error) {
			console.log(error);
		}
	}
</script>

<div class="min-h-svh w-full p-4">
	{#if currentUser !== null}
		<div class="text-lg w-full flex flex-col h-full bg-neutral-950 absolute top-0 left-0 p-4">
			{#if editing}
				<div>
					<input type="text" placeholder="Allowed Usage (GB)" />
					<input type="text" placeholder="Expires At (Days)" />
				</div>
			{:else}
				<div>{currentUser.username}</div>
				<div>{formatBytes(currentUser.totalUsage)}/{formatBytes(currentUser.allowedUsage)}</div>
				<div>{formatExpiry(currentUser.expiresAt)}</div>
				<div>{currentUser.locked ? 'locked' : 'not locked'}</div>
				<div>
					socks://{btoa(
						currentUser.username + ':' + currentUser.password
					)}@{publicAddress}#{currentUser.username}
				</div>
				<div class="flex items-center mt-4">
					<button
						class="px-4 py-2 mr-4 border border-neutral-800 rounded"
						on:click={() => (currentUser = null)}>BACK</button
					>
					<button
						class="px-4 py-2 border border-neutral-800 rounded"
						on:click={() => deleteUser(currentUser?.username || '')}>DELETE</button
					>
				</div>
			{/if}
		</div>
	{/if}
	{#each users as user, i}
		<div
			class="grid grid-cols-3 rounded border border-neutral-800 px-4 py-2 mb-4 {user.locked &&
				'bg-red-500'}"
		>
			<div class="flex items-center">#{i + 1} {user.username}</div>
			<div class="flex">
				<div class="border-r border-neutral-800 pr-2 mr-2">
					{formatBytes(user.totalUsage)}/{formatBytes(user.allowedUsage)}
				</div>
				<div>{formatExpiry(user.expiresAt)}</div>
			</div>
			<div class="flex items-center justify-end">
				<button on:click={() => (currentUser = user)}>DETAILS</button>
				<button class="ml-4" on:click={() => deleteUser(user.username)}>DELETE</button>
			</div>
		</div>
	{/each}
</div>
