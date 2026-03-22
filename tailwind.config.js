/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./web/templates/**/*.html", "./internal/**/*.go"],
  theme: {
    fontFamily: {
      sans: ['-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', 'system-ui', 'sans-serif'],
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
