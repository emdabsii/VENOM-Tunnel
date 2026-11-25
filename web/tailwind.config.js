export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        fa: ['Vazirmatn', 'Tahoma', 'sans-serif'],
        en: ['Plus Jakarta Sans', 'Inter', 'sans-serif'],
        zh: ['Noto Sans SC', 'sans-serif'],
        mono: ['JetBrains Mono', 'monospace']
      }
    }
  },
  plugins: []
};
