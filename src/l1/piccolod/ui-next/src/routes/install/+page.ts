import type { PageLoad } from './$types';
import type { InstallTarget } from '$lib/api/install';

export const prerender = false;

type RawInstallTarget = {
	id?: unknown;
	model?: unknown;
	size_bytes?: unknown;
	contents?: unknown;
	erase_warning?: unknown;
};

const normalizeTarget = (raw: RawInstallTarget): InstallTarget | null => {
	const id = typeof raw.id === 'string' ? raw.id : '';
	if (!id) return null;
	const model = typeof raw.model === 'string' ? raw.model : 'Unknown device';
	const sizeValue = typeof raw.size_bytes === 'number' ? raw.size_bytes : Number(raw.size_bytes ?? 0);
	const contents = Array.isArray(raw.contents) ? raw.contents.map((entry) => String(entry)) : [];
	const eraseWarning = Boolean(raw.erase_warning);
	return {
		id,
		model,
		sizeBytes: Number.isFinite(sizeValue) ? sizeValue : 0,
		contents,
		eraseWarning
	};
};

export const load: PageLoad<{ targets: InstallTarget[] }> = async ({ fetch }) => {
	try {
		const res = await fetch('/api/v1/install/targets');
		if (!res.ok) {
			return { targets: [] };
		}
		const body = await res.json();
		const targetsInput = Array.isArray(body?.targets) ? (body.targets as RawInstallTarget[]) : [];
		const targets = targetsInput.map(normalizeTarget).filter((target): target is InstallTarget => Boolean(target));
		return { targets };
	} catch (err) {
		console.error('Failed to load install targets', err);
		return { targets: [] };
	}
};
