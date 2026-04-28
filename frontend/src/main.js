import "@fortawesome/fontawesome-free/css/all.min.css";
import "flowbite/dist/flowbite.css";
import "./styles.css";
import App from "./App.svelte";
import { mount } from "svelte";
import { setupI18n } from "./lib/i18n/index.js";
import { setupTheme } from "./lib/theme.js";

setupI18n();
setupTheme();

const app = mount(App, {
  target: document.getElementById("app"),
});

export default app;
