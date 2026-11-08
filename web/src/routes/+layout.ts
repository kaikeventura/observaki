// Routes are prerendered into web/build so the Go binary can serve the real
// markup (title, tokens, content) for every URL; the client router takes over
// after hydration, and the adapter fallback covers unknown routes.
export const prerender = true;
