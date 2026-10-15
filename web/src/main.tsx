import "@fontsource-variable/ibm-plex-sans/wght.css";
import "@fontsource/ibm-plex-mono/latin-400.css";
import "@fontsource/ibm-plex-mono/latin-500.css";
import { createRoot } from "react-dom/client";
import App from "./App";
import { initializeLocale } from "./i18n";
import "./styles.css";
void initializeLocale().then(() =>
  createRoot(document.getElementById("app")!).render(<App />),
);
