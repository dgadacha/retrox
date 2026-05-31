import type { Config } from "tailwindcss"

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // surfaces — deep blue-gray, Steam-like with a colder hue
        ink: {
          950: "#0a0e14", // page background
          900: "#10151c", // sidebar, large surfaces
          850: "#131923",
          800: "#171e2a", // cards
          700: "#1f2735", // raised
          600: "#2a3441", // borders
          500: "#3c4858", // muted strokes
        },
        // primary accent — violet
        accent: {
          300: "#c4b5fd",
          400: "#a78bfa",
          500: "#8b5cf6",
          600: "#7c3aed",
          700: "#6d28d9",
        },
        // secondary accent — cyan
        cyan2: {
          300: "#67e8f9",
          400: "#22d3ee",
          500: "#06b6d4",
        },
        success: "#10b981",
        warn: "#f59e0b",
        danger: "#ef4444",
        text: {
          100: "#f1f3f7",
          300: "#c2c7d0",
          500: "#8a93a0",
          700: "#4b5360",
        },
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "-apple-system", "Segoe UI", "Roboto", "sans-serif"],
      },
      boxShadow: {
        card: "0 10px 30px rgba(0,0,0,0.45)",
        glow: "0 0 0 1px rgba(139,92,246,0.4), 0 8px 30px rgba(139,92,246,0.25)",
      },
      backgroundImage: {
        "accent-gradient": "linear-gradient(135deg, #8b5cf6 0%, #22d3ee 100%)",
        "hero-fade":
          "linear-gradient(180deg, rgba(10,14,20,0) 0%, rgba(10,14,20,0.5) 60%, rgba(10,14,20,1) 100%)",
      },
    },
  },
  plugins: [],
} satisfies Config
