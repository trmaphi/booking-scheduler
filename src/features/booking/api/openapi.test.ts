// @vitest-environment node
import SwaggerParser from "@apidevtools/swagger-parser";
import path from "node:path";
import { describe, expect, it } from "vitest";

describe("booking OpenAPI contract", () => {
  it("is a valid OpenAPI document with every booking operation", async () => {
    const api = (await SwaggerParser.validate(
      path.resolve("api/openapi/booking-api.yaml"),
    )) as unknown as {
      openapi: string;
      paths?: Record<string, unknown>;
    };

    expect(api.openapi).toBe("3.1.0");
    expect(Object.keys(api.paths ?? {})).toEqual([
      "/api/v1/booking-options",
      "/api/v1/availability",
      "/api/v1/appointments",
      "/api/v1/appointments/{appointmentId}",
      "/api/v1/health/live",
      "/api/v1/health/ready",
    ]);
  });

  it("keeps booking, availability, and error examples compatible with their schemas", async () => {
    const api = (await SwaggerParser.dereference(
      path.resolve("api/openapi/booking-api.yaml"),
    )) as unknown as OpenApiDocument;

    const examples = [
      responseContent(api, "/api/v1/booking-options", "200"),
      responseContent(api, "/api/v1/availability", "200"),
      responseContent(api, "/api/v1/availability", "400"),
      responseContent(api, "/api/v1/availability", "405"),
      responseContent(api, "/api/v1/availability", "500"),
      responseContent(api, "/api/v1/appointments", "200"),
      responseContent(api, "/api/v1/appointments", "201", "post"),
      responseContent(api, "/api/v1/appointments", "200", "post"),
      responseContent(api, "/api/v1/appointments", "409", "post"),
      responseContent(api, "/api/v1/appointments/{appointmentId}", "200"),
      responseContent(api, "/api/v1/appointments/{appointmentId}", "404"),
    ];

    for (const { example, schema } of examples) {
      expect(example).toBeDefined();
      expect(() => validateExample(example, schema, "$")).not.toThrow();
    }
  });

  it("rejects non-UUID identifiers and undeclared response properties", async () => {
    const api = (await SwaggerParser.dereference(
      path.resolve("api/openapi/booking-api.yaml"),
    )) as unknown as OpenApiDocument;
    const { example, schema } = responseContent(
      api,
      "/api/v1/booking-options",
      "200",
    );
    const valid = structuredClone(example) as {
      vehicles: Array<Record<string, unknown>>;
    };
    const invalidIdentifier = structuredClone(valid);
    invalidIdentifier.vehicles[0].id = "not-a-uuid";
    expect(() => validateExample(invalidIdentifier, schema, "$")).toThrow(
      /UUID/,
    );

    const extraProperty = structuredClone(valid);
    extraProperty.vehicles[0].internalNote = "must not cross the contract";
    expect(() => validateExample(extraProperty, schema, "$")).toThrow(
      /undeclared property/,
    );
  });
});

type Schema = {
  type?: string;
  format?: string;
  const?: unknown;
  enum?: unknown[];
  required?: string[];
  properties?: Record<string, Schema>;
  items?: Schema;
  minLength?: number;
  maxLength?: number;
  minimum?: number;
  pattern?: string;
  additionalProperties?: boolean | Schema;
};

type Media = { schema: Schema; example?: unknown };
type OpenApiDocument = {
  paths: Record<
    string,
    {
      get?: {
        responses: Record<string, { content: { "application/json": Media } }>;
      };
      post?: {
        responses: Record<string, { content: { "application/json": Media } }>;
      };
    }
  >;
};

function responseContent(
  api: OpenApiDocument,
  route: string,
  status: string,
  method: "get" | "post" = "get",
): Media {
  const operation = api.paths[route][method];
  if (!operation)
    throw new Error(`${method.toUpperCase()} ${route} is missing`);
  return operation.responses[status].content["application/json"];
}

function validateExample(
  value: unknown,
  schema: Schema,
  location: string,
): void {
  if (schema.const !== undefined && value !== schema.const)
    throw new Error(`${location} must equal its const`);
  if (schema.enum && !schema.enum.includes(value))
    throw new Error(`${location} must be an enum member`);
  switch (schema.type) {
    case "object": {
      if (typeof value !== "object" || value === null || Array.isArray(value))
        throw new Error(`${location} must be an object`);
      const record = value as Record<string, unknown>;
      const declared = schema.properties ?? {};
      for (const key of schema.required ?? [])
        if (!(key in record)) throw new Error(`${location}.${key} is required`);
      for (const [key, childValue] of Object.entries(record)) {
        if (!(key in declared)) {
          if (schema.additionalProperties === false)
            throw new Error(`${location}.${key} is an undeclared property`);
          if (
            typeof schema.additionalProperties === "object" &&
            schema.additionalProperties !== null
          )
            validateExample(
              childValue,
              schema.additionalProperties,
              `${location}.${key}`,
            );
        }
      }
      for (const [key, child] of Object.entries(declared))
        if (key in record)
          validateExample(record[key], child, `${location}.${key}`);
      break;
    }
    case "array":
      if (!Array.isArray(value))
        throw new Error(`${location} must be an array`);
      for (const [index, item] of value.entries())
        validateExample(item, schema.items ?? {}, `${location}[${index}]`);
      break;
    case "string":
      if (typeof value !== "string")
        throw new Error(`${location} must be a string`);
      if (schema.format === "uuid" && !isCanonicalUUID(value))
        throw new Error(`${location} must be a canonical UUID`);
      if (schema.minLength !== undefined && value.length < schema.minLength)
        throw new Error(`${location} is too short`);
      if (schema.maxLength !== undefined && value.length > schema.maxLength)
        throw new Error(`${location} is too long`);
      if (schema.pattern && !new RegExp(schema.pattern).test(value))
        throw new Error(`${location} does not match its pattern`);
      break;
    case "integer":
      if (
        !Number.isInteger(value) ||
        (schema.minimum !== undefined && (value as number) < schema.minimum)
      )
        throw new Error(`${location} must be a valid integer`);
      break;
  }
}

function isCanonicalUUID(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    value,
  );
}
