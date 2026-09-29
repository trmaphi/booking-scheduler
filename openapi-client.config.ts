import { defineConfig } from "@hey-api/openapi-ts";

export default defineConfig({
  input: "./openapi/booking-api.yaml",
  output: {
    path: "src/features/booking/api/generated",
    postProcess: ["prettier"],
  },
  plugins: ["@hey-api/client-fetch", "@hey-api/typescript", "@hey-api/sdk"],
});
