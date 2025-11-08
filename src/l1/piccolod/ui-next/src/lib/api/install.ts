import { http } from './http';

export type InstallTarget = {
	id: string;
	model: string;
	sizeBytes: number;
	contents: string[];
	eraseWarning: boolean;
};

export type InstallPlan = {
	target: string;
	actions: string[];
	simulate: boolean;
};

export type InstallPlanResponse = {
	plan: InstallPlan;
};

export type FetchLatestImage = {
	version?: string;
	verified?: boolean;
	sizeBytes?: number;
};

type InstallTargetDTO = {
	id: string;
	model: string;
	size_bytes: number;
	contents?: string[];
	erase_warning?: boolean;
};

type InstallTargetsDTO = {
	targets?: InstallTargetDTO[];
};

type InstallPlanDTO = {
	plan: {
		target: string;
		actions?: string[];
		simulate?: boolean;
	};
};

type FetchLatestDTO = {
	version?: string;
	verified?: boolean;
	size_bytes?: number;
};

type InstallPlanParams = {
	targetId: string;
	fetchLatest?: boolean;
};

type RunInstallParams = {
	targetId: string;
	fetchLatest?: boolean;
	acknowledgeId: string;
};

const toInstallTarget = (target: InstallTargetDTO): InstallTarget => ({
	id: target.id,
	model: target.model,
	sizeBytes: target.size_bytes,
	contents: target.contents ?? [],
	eraseWarning: Boolean(target.erase_warning)
});

export async function fetchInstallTargets(): Promise<InstallTarget[]> {
	const data = await http<InstallTargetsDTO>('/install/targets');
	return (data.targets ?? []).map(toInstallTarget);
}

export async function requestInstallPlan(params: InstallPlanParams): Promise<InstallPlan> {
	const payload = {
		target_id: params.targetId,
		fetch_latest: params.fetchLatest ?? false
	};
	const data = await http<InstallPlanDTO>('/install/plan', {
		method: 'POST',
		json: payload
	});
	return {
		target: data.plan.target,
		actions: data.plan.actions ?? [],
		simulate: Boolean(data.plan.simulate)
	};
}

export async function fetchLatestImage(): Promise<FetchLatestImage> {
	const data = await http<FetchLatestDTO>('/install/fetch-latest', { method: 'POST' });
	return {
		version: data.version,
		verified: data.verified,
		sizeBytes: data.size_bytes
	};
}

export async function runInstall(params: RunInstallParams): Promise<void> {
	const payload = {
		target_id: params.targetId,
		fetch_latest: params.fetchLatest ?? false,
		acknowledge_id: params.acknowledgeId
	};
	await http('/install/run', { method: 'POST', json: payload });
}
