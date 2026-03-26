/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./web/templates/**/*.html", "./web/static/**/*.js", "./internal/pipeline/pipeline.go"],
  theme: {
    fontFamily: {
      sans: ['-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', 'system-ui', 'sans-serif'],
    },
    extend: {
      colors: {
        sage: '#8BA888',
      },
    },
  },
  plugins: [require("daisyui")],
  daisyui: {
    themes: [
      {
        "press-out": {
          "primary": "#8BA888",
          "secondary": "#C4BFAE",
          "accent": "#8BA888",
          "neutral": "#EDEDEA",
          "base-100": "#FAFAF8",
          "base-content": "#2D2D2D",
          "info": "#9BB0BA",
          "success": "#7DA67D",
        },
      },
    ],
  },
};
