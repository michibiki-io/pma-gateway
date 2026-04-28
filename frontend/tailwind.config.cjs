module.exports = {
  content: ['./index.html', './src/**/*.{svelte,js}', './node_modules/flowbite-svelte/**/*.{html,js,svelte,ts}'],
  theme: {
    extend: {}
  },
  plugins: [require('flowbite/plugin')]
};
