/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./public/**/*.{html,js}'],
  theme: {
    extend: {
      colors: {
        base: '#111114',     // page background
        panel: '#17171B',    // header, sidebar, cards
        raised: '#1F1F24',   // inputs, hover states
        line: '#2B2B32',     // borders
        fg: '#ECECF1',       // main text
        muted: '#8D8D98',    // secondary text
        faint: '#5E5E69',    // disabled text
        accent: { DEFAULT: '#D13B42', hover: '#E04A51', soft: '#2A1618' },
      },
      fontFamily: {
        sans: ['"Schibsted Grotesk"', 'system-ui', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'monospace'],
      },
    },
  },
  plugins: [],
};
