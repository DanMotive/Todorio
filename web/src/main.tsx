import React from "react"
import ReactDOM from "react-dom/client"
import App from "./App"
import { PublicListPage } from "./extras"
import "./theme.css"

// The PWA install prompt is offered from Settings, but the browser fires
// `beforeinstallprompt` once during initial load — typically before the Settings page has
// ever been mounted. Capture it here at module scope and stash it on window so the Settings
// row can pick it up whenever the user navigates there.
window.addEventListener("beforeinstallprompt", (e: Event) => {
  e.preventDefault()
  ;(window as any).todorioInstallEvent = e
})

// Public read-only links /s/{token} render without authentication.
const share = window.location.pathname.match(/^\/s\/([A-Za-z0-9]+)\/?$/)

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    {share ? <PublicListPage token={share[1]} /> : <App />}
  </React.StrictMode>,
)
