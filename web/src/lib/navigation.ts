export type NavIconName = 'explore' | 'dashboards' | 'alerts' | 'settings' | 'states' | 'health';

export type NavItem = {
	href: string;
	label: string;
	icon: NavIconName;
};

/** Phase 1 primary surfaces required by the app shell (Story 6.1, AC1). */
export const primaryNav: readonly NavItem[] = [
	{ href: '/explore', label: 'Explore', icon: 'explore' },
	{ href: '/dashboards', label: 'Dashboards', icon: 'dashboards' },
	{ href: '/alerts', label: 'Alerts', icon: 'alerts' },
	{ href: '/settings', label: 'Settings', icon: 'settings' }
];

/**
 * Reference surfaces. The state gallery is part of the Phase 1 information
 * architecture (EXPERIENCE.md) and keeps every UI state reviewable.
 */
export const secondaryNav: readonly NavItem[] = [
	{ href: '/states', label: 'State gallery', icon: 'states' },
	{ href: '/health', label: 'System health', icon: 'health' }
];

/** A nav item is current on its own route and on nested routes. */
export function isActivePath(activePath: string, href: string): boolean {
	if (href === '/') return activePath === '/';
	return activePath === href || activePath.startsWith(`${href}/`);
}
