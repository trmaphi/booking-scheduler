import { access, readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";

const requiredRoots = [
  "src/features/booking/domain",
  "src/features/booking/application",
  "src/features/booking/infrastructure/postgres",
  "src/features/booking/ui",
  "src/app/api/v1",
];

const forbiddenDomainImports = [
  "next",
  "next/",
  "postgres",
  "pg",
  "@neondatabase/",
  "@/app/",
  "../application",
  "../infrastructure",
  "../api",
  "../ui",
];

async function sourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map((entry) => {
      const entryPath = path.join(directory, entry.name);
      return entry.isDirectory()
        ? sourceFiles(entryPath)
        : /\.[cm]?[jt]sx?$/.test(entry.name)
          ? [entryPath]
          : [];
    }),
  );
  return nested.flat();
}

function importedModules(source) {
  const imports = [];
  const expression = /(?:from\s+|import\s*\(|require\s*\()\s*["']([^"']+)["']/g;
  for (const match of source.matchAll(expression)) imports.push(match[1]);
  return imports;
}

function isForbiddenDomainImport(moduleName) {
  return forbiddenDomainImports.some((forbidden) =>
    forbidden.endsWith("/")
      ? moduleName.startsWith(forbidden)
      : moduleName === forbidden || moduleName.startsWith(`${forbidden}/`),
  );
}

export async function findBoundaryViolations(
  root,
  { requireModuleRoots = false } = {},
) {
  const violations = [];

  if (requireModuleRoots) {
    for (const moduleRoot of requiredRoots) {
      try {
        await access(path.join(root, moduleRoot));
      } catch {
        violations.push(`missing required module root ${moduleRoot}`);
      }
    }
  }

  const domainRoot = path.join(root, "src/features/booking/domain");
  try {
    for (const file of await sourceFiles(domainRoot)) {
      const source = await readFile(file, "utf8");
      for (const moduleName of importedModules(source)) {
        if (isForbiddenDomainImport(moduleName)) {
          violations.push(
            `${path.relative(root, file)} imports forbidden module "${moduleName}"`,
          );
        }
      }
    }
  } catch (error) {
    if (error?.code !== "ENOENT") throw error;
  }

  return violations.sort();
}

async function main() {
  const violations = await findBoundaryViolations(process.cwd(), {
    requireModuleRoots: true,
  });
  if (violations.length > 0) {
    console.error(violations.join("\n"));
    process.exitCode = 1;
  } else {
    console.log("Booking module boundaries are valid.");
  }
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) await main();
