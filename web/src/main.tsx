import React from "react"
import ReactDOM from "react-dom/client"
import App from "./App"
import { PublicListPage } from "./extras"
import "./theme.css"

// Публичные read-only ссылки /s/{token} рендерятся без авторизации.
const share = window.location.pathname.match(/^\/s\/([A-Za-z0-9]+)\/?$/)

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    {share ? <PublicListPage token={share[1]} /> : <App />}
  </React.StrictMode>,
)
