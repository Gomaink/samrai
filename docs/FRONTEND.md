# Frontend development

The frontend is a React and TypeScript application built with Vite. The production files are written to `internal/webui/dist` and embedded by the Go server.

## Development server

Start the backend:

```powershell
.\run-windows.bat
```

In another terminal:

```powershell
Set-Location web
npm ci
npm run dev
```

Open `http://127.0.0.1:5173/`. Vite forwards API requests to the Go server according to `vite.config.ts`.

## Production build

```bash
cd web
npm ci
npm run typecheck
npm run build
```

Commit changes to `internal/webui/dist` when the source archive is expected to build and run without Node.js. Do not commit `web/node_modules`.

## Application flow

The top-level application checks setup and authentication status, then renders either setup, login, or the library shell. TanStack Query owns server state; reader components keep short-lived interaction state locally.

Routes are handled in the browser and fall back to the embedded `index.html` for direct navigation.

## Text and accessibility

User-facing interface text is written in English. Buttons that contain only an icon need an accessible label, dialogs need a labelled heading, and reader controls must remain usable from a keyboard.

## Security

Do not inject imported metadata as HTML. EPUB content is handled by the EPUB reader's isolated resource path and iframe rules rather than being mounted into the main application DOM.
