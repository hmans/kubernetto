Kubernetto chart components live here as internal Web Components.

This is not a published package. The goal is to keep chart rendering,
data loading, batching, and component registration separate from the
application shell in `app.ts`.

Add new chart elements through `index.js` so the application can import
one registry entrypoint.
