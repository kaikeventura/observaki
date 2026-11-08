import { writable, type Writable } from 'svelte/store';

export type ProjectEnvironment = 'production' | 'staging' | 'development';

export type Project = {
	id: string;
	name: string;
	organization: string;
	environment: ProjectEnvironment;
};

/**
 * Mock projects for the Phase 0 shell. A real project list comes from the
 * control plane (`GET /api/v1/projects`) once the UI is wired to the API.
 */
export const projects: readonly Project[] = [
	{
		id: 'project-prod',
		name: 'Production',
		organization: 'Acme',
		environment: 'production'
	},
	{
		id: 'project-staging',
		name: 'Staging',
		organization: 'Acme',
		environment: 'staging'
	}
];

export const PROJECT_STORAGE_KEY = 'observaki.project';

export function findProject(id: string): Project | undefined {
	return projects.find((project) => project.id === id);
}

function readStoredProjectId(): string | null {
	if (typeof localStorage === 'undefined') return null;
	try {
		return localStorage.getItem(PROJECT_STORAGE_KEY);
	} catch {
		return null;
	}
}

function persistProjectId(id: string): void {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(PROJECT_STORAGE_KEY, id);
	} catch {
		// Storage can be disabled (private mode); the in-memory selection still works.
	}
}

function initialProject(): Project {
	// Deterministic on the server and during hydration; the stored selection is
	// applied after mount so prerendered HTML and the first client render match.
	return projects[0];
}

const store = writable<Project>(initialProject());

export type ActiveProjectStore = Pick<Writable<Project>, 'subscribe'> & {
	/** Select a mock project by ID and remember it for the next visit. */
	select(id: string): void;
	/** Apply the selection persisted by a previous visit (client only). */
	restore(): void;
};

export const activeProject: ActiveProjectStore = {
	subscribe: store.subscribe,
	select(id: string) {
		const project = findProject(id);
		if (!project) return;
		store.set(project);
		persistProjectId(project.id);
	},
	restore() {
		const project = findProject(readStoredProjectId() ?? '');
		if (project) store.set(project);
	}
};
