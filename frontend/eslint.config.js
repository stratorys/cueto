import pluginVue from "eslint-plugin-vue";
import { defineConfigWithVueTs, vueTsConfigs } from "@vue/eslint-config-typescript";
import eslintConfigPrettier from "eslint-config-prettier/flat";

// Vue 3 + TS recommended rules, with Prettier owning all formatting (its config is
// applied last so it disables any stylistic ESLint rules that would conflict).
export default defineConfigWithVueTs(
  { ignores: ["dist/**", "node_modules/**"] },
  pluginVue.configs["flat/recommended"],
  vueTsConfigs.recommended,
  {
    rules: {
      // Page and app-shell components have intentional single-word names
      // (Editor, Onboarding, Toolbar); the multi-word convention does not apply.
      "vue/multi-word-component-names": "off",
      // Catch dead code (the reason this tooling exists); allow intentionally
      // unused bindings when prefixed with an underscore.
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
        },
      ],
    },
  },
  eslintConfigPrettier,
);
