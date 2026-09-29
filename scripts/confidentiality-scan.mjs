import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml } from "yaml";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const fixtureAllowlist = JSON.parse(
  readFileSync(join(scriptDirectory, "confidentiality-allowlist.json"), "utf8"),
);
const allowedNames = new Set(fixtureAllowlist.fixtureNames);
const allowedAddresses = new Set(fixtureAllowlist.fixtureAddresses);
const allowedRegistrations = new Set(fixtureAllowlist.fixtureRegistrations);
const allowedEmailDomains = new Set(fixtureAllowlist.fixtureEmailDomains);
const allowedPhones = new Set(
  fixtureAllowlist.fixturePhoneNumbers.map(normalizePhone),
);

const builtInTermDigests = [
  {
    length: 7,
    digest: "ac4d3763c7c8118b0f852d3cb4ddd14bd0034ddef7b5a8a054193b42c732cd60",
  },
  {
    length: 12,
    digest: "ec20da19ff4b9b401461e9e03a7c3f437577eb97d8efcf55cf6a66352379768e",
  },
];
const configuredDigests = (process.env.CONFIDENTIALITY_PROHIBITED_DIGESTS ?? "")
  .split(/[\n,]/)
  .map((value) => value.trim())
  .filter(Boolean)
  .map((value) => {
    const [length, digest] = value.split(":", 2);
    return { length: Number(length), digest: digest?.toLowerCase() };
  })
  .filter(
    ({ length, digest }) =>
      Number.isSafeInteger(length) &&
      length > 0 &&
      length <= 256 &&
      /^[a-f0-9]{64}$/.test(digest ?? ""),
  );
const prohibitedTermDigests = [...builtInTermDigests, ...configuredDigests];
const configuredTerms = (process.env.CONFIDENTIALITY_PROHIBITED_TERMS ?? "")
  .split(/[\n,]/)
  .map((value) => value.trim())
  .filter(Boolean);
const prohibitedTerms = [...new Set(configuredTerms)];
const providerTokens = [
  "aws",
  "amazon web services",
  "google",
  "google cloud",
  "gcp",
  "azure",
  "azurerm",
  "vercel",
  "netlify",
  "fly.io",
  "render",
  "railway",
  "grafana",
  "cloudflare",
  "supabase",
  "neon",
  "neondatabase",
  ...(process.env.CONFIDENTIALITY_PROVIDER_TOKENS ?? "")
    .split(/[\n,]/)
    .map((value) => value.trim().toLowerCase())
    .filter(Boolean),
];
const findings = new Map();

function git(args, encoding = "buffer") {
  return execFileSync("git", args, {
    encoding,
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function asText(bytes) {
  return Buffer.isBuffer(bytes) ? bytes.toString("utf8") : String(bytes);
}

function normalizePhone(value) {
  const trimmed = value.trim();
  return `${trimmed.startsWith("+") ? "+" : ""}${trimmed.replace(/\D/g, "")}`;
}

function locationID(scope, object, path) {
  return createHash("sha256")
    .update(scope)
    .update("\0")
    .update(object)
    .update("\0")
    .update(path)
    .digest("hex")
    .slice(0, 16);
}

function add(category, scope, object, path) {
  const locator = locationID(scope, object, path);
  findings.set(`${category}\0${scope}\0${locator}`, {
    category,
    scope,
    locator,
  });
}

function containsProhibitedTerm(pathText, text) {
  const candidate = `${pathText}\n${text}`.toLowerCase();
  if (prohibitedTerms.some((term) => candidate.includes(term.toLowerCase()))) {
    return true;
  }
  const tokens = candidate.match(/[a-z0-9][a-z0-9.-]{0,255}/g) ?? [];
  return tokens.some((token) =>
    prohibitedTermDigests.some(({ length, digest }) => {
      for (let index = 0; index + length <= token.length; index += 1) {
        const value = token.slice(index, index + length);
        if (createHash("sha256").update(value).digest("hex") === digest) {
          return true;
        }
      }
      return false;
    }),
  );
}

function hasSourceFingerprint(pathText, text) {
  const decode = (encoded) => Buffer.from(encoded, "base64").toString("utf8");
  const contexts = new Set(["Y29kaW5n", "dGVjaG5pY2Fs"].map(decode));
  const exercises = new Set(
    ["YXNzZXNzbWVudA==", "Y2hhbGxlbmdl", "ZXhlcmNpc2U="].map(decode),
  );
  const documents = new Set(
    [
      "YnJpZWY=",
      "ZG9jdW1lbnQ=",
      "c291cmNl",
      "c3BlYw==",
      "c3BlY2lmaWNhdGlvbg==",
    ].map(decode),
  );
  const restrictedMarker = decode("Y29uZmlkZW50aWFs");
  const domainMarker = decode("dGVjaG5pY2Fs");
  const evaluationMarker = decode("YXNzZXNzbWVudA==");
  const activityMarker = decode("ZXhlcmNpc2U=");
  const documentMarker = decode("YnJpZWY=");
  const tokens = `${pathText}\n${text}`
    .toLowerCase()
    .split(/[^a-z]+/)
    .filter(Boolean);

  for (let start = 0; start < tokens.length; start += 1) {
    const window = new Set(tokens.slice(start, start + 7));
    const hasContext = [...contexts].some((token) => window.has(token));
    const hasExercise = [...exercises].some((token) => window.has(token));
    const hasDocument = [...documents].some((token) => window.has(token));
    if (hasContext && hasExercise && hasDocument) return true;
    if (window.has(restrictedMarker) && hasContext && hasExercise) return true;
    if (
      window.has(restrictedMarker) &&
      window.has(domainMarker) &&
      window.has(evaluationMarker)
    )
      return true;
    if (
      window.has(restrictedMarker) &&
      window.has(activityMarker) &&
      window.has(documentMarker)
    )
      return true;
  }
  return false;
}

function hasCredential(pathText, text) {
  const candidate = `${pathText}\n${text}`;
  return [
    /\bgh[pousr]_[A-Za-z0-9]{20,}\b/,
    /\b(?:AKIA|ASIA)[A-Z0-9]{16}\b/,
    /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/,
    /\b(?:api[_-]?key|secret|token|password)\s*[:=]\s*["']?(?!scheduler\b)[A-Za-z0-9_./+=-]{16,}/i,
  ].some((pattern) => pattern.test(candidate));
}

function parseDelimitedRow(line, delimiter = ",") {
  const values = [];
  let value = "";
  let quote = "";
  for (let index = 0; index < line.length; index += 1) {
    const character = line[index];
    if (quote) {
      if (character === quote && line[index + 1] === quote) {
        value += character;
        index += 1;
      } else if (character === quote) quote = "";
      else value += character;
    } else if (character === '"' || character === "'") quote = character;
    else if (character === delimiter) {
      values.push(value.trim());
      value = "";
    } else value += character;
  }
  values.push(value.trim());
  return values;
}

function canonicalFixtureField(key) {
  const normalized = key.toLowerCase().replace(/[^a-z0-9]+/g, "_");
  if (/^(?:name|full_name|customer_name|technician_name)$/.test(normalized))
    return "name";
  if (/^(?:phone|phone_number|telephone|mobile)$/.test(normalized))
    return "phone";
  if (/^(?:address|postal_address|street_address)$/.test(normalized))
    return "address";
  if (
    /^(?:registration|registration_number|vehicle_registration|license_plate)$/.test(
      normalized,
    )
  )
    return "registration";
  if (/^(?:email|email_address)$/.test(normalized)) return "email";
  return "";
}

function fixtureFieldViolation(key, rawValue) {
  const field = canonicalFixtureField(key);
  if (!field || rawValue === null || rawValue === undefined) return false;
  const value = String(rawValue).trim();
  if (!value) return false;
  if (field === "name") return !allowedNames.has(value);
  if (field === "phone") return !allowedPhones.has(normalizePhone(value));
  if (field === "address") return !allowedAddresses.has(value);
  if (field === "registration") return !allowedRegistrations.has(value);
  if (field === "email") {
    const separator = value.lastIndexOf("@");
    return (
      separator < 1 ||
      !allowedEmailDomains.has(value.slice(separator + 1).toLowerCase())
    );
  }
  return false;
}

function jsonFixtureViolation(value) {
  if (Array.isArray(value)) return value.some(jsonFixtureViolation);
  if (!value || typeof value !== "object") return false;
  return Object.entries(value).some(
    ([key, fieldValue]) =>
      fixtureFieldViolation(key, fieldValue) ||
      jsonFixtureViolation(fieldValue),
  );
}

function yamlFixtureViolation(text) {
  try {
    return jsonFixtureViolation(parseYaml(text));
  } catch {
    return false;
  }
}

function structuredFixtureViolation(pathText, text) {
  if (/\.json$/i.test(pathText) || /^[\s]*[\[{]/.test(text)) {
    try {
      if (jsonFixtureViolation(JSON.parse(text))) return true;
    } catch {
      // Continue with conservative field-aware text parsing.
    }
  }

  if (/\.ya?ml$/i.test(pathText) && yamlFixtureViolation(text)) return true;

  for (const match of text.matchAll(
    /(?:^|\n)\s*["']?([A-Za-z][A-Za-z0-9 _-]*)["']?\s*:\s*["']?([^\n"']*)["']?/g,
  )) {
    if (fixtureFieldViolation(match[1], match[2])) return true;
  }

  if (/\.csv$/i.test(pathText)) {
    const lines = text.split(/\r?\n/).filter(Boolean);
    const headers = parseDelimitedRow(lines[0] ?? "");
    for (const line of lines.slice(1)) {
      const values = parseDelimitedRow(line);
      if (
        headers.some((header, index) =>
          fixtureFieldViolation(header, values[index]),
        )
      )
        return true;
    }
  }

  for (const statement of text.matchAll(
    /insert\s+into\s+[^\s(]+\s*\(([^)]+)\)\s*values\s*([\s\S]*?);/gi,
  )) {
    const columns = parseDelimitedRow(statement[1]).map((column) =>
      column.replace(/["'`]/g, "").trim(),
    );
    for (const row of statement[2].matchAll(/\(([^()]*)\)/g)) {
      const values = parseDelimitedRow(row[1]);
      if (
        columns.some((column, index) =>
          fixtureFieldViolation(column, values[index]),
        )
      )
        return true;
    }
  }
  return false;
}

function fixtureViolatesPolicy(pathText, text) {
  if (!/(?:^|\/)(?:fixtures?|[^/]*seed[^/]*)\b/i.test(pathText)) return false;

  if (structuredFixtureViolation(pathText, text)) return true;

  const emails = text.match(/[A-Z0-9._%+-]+@([A-Z0-9.-]+\.[A-Z]{2,})/gi) ?? [];
  if (
    emails.some((email) => {
      const domain = email.slice(email.lastIndexOf("@") + 1).toLowerCase();
      return !allowedEmailDomains.has(domain);
    })
  )
    return true;

  const phones = [
    ...text.matchAll(
      /(?:^|\n)\s*(?:phone|telephone|mobile)\s*:\s*["']?([^\n"']+)/gi,
    ),
  ].map((match) => match[1]);
  if (
    phones.some((phone) => {
      const digits = phone.replace(/\D/g, "");
      return digits.length >= 10 && !allowedPhones.has(normalizePhone(phone));
    })
  )
    return true;

  const addresses =
    text.match(
      /\b\d{1,5}[A-Z]?\s+[A-Z][A-Za-z.'-]+(?:\s+[A-Z][A-Za-z.'-]+){0,4}\s+(?:Street|St|Road|Rd|Avenue|Ave|Lane|Ln|Drive|Dr|Way|Boulevard|Blvd)\b/g,
    ) ?? [];
  if (addresses.some((address) => !allowedAddresses.has(address))) return true;

  const registrations = text.match(/\b[A-Z]{2}\d{2}\s?[A-Z]{3}\b/g) ?? [];
  if (
    registrations.some(
      (registration) => !allowedRegistrations.has(registration),
    )
  )
    return true;

  const structuredNames = [
    ...text.matchAll(
      /(?:^|\n)\s*(?:name|customer(?:_name)?|technician(?:_name)?)\s*:\s*["']?([A-Z][A-Za-z.'-]+(?:\s+[A-Z][A-Za-z.'-]+){1,3})/g,
    ),
  ].map((match) => match[1]);
  return structuredNames.some((name) => !allowedNames.has(name));
}

function hasProviderCommitment(pathText, text) {
  const approvedProductionPath =
    /^(?:infra\/ansible\/(?:group_vars|roles|playbooks)\/|openspec\/changes\/deploy-production-vps\/|docs\/operations\/)/i.test(
      pathText,
    );
  const approvedMonitoringPath =
    /^(?:infra\/ansible\/(?:playbooks\/monitoring\.yml|group_vars\/monitoring(?:\.vault)?\.example\.yml|roles\/monitoring\/)|docs\/operations\/)/i.test(
      pathText,
    );
  const approvedProductionProvider = (value) =>
    (approvedProductionPath && /^(?:cloudflare)$/i.test(value.trim())) ||
    (approvedMonitoringPath && /^(?:grafana)$/i.test(value.trim()));
  if (
    /(?:^|\/)(?:vercel\.json|netlify\.toml|fly\.toml|render\.ya?ml|railway\.json|serverless\.ya?ml|supabase\/config\.toml)$/i.test(
      pathText,
    )
  )
    return true;

  const providerNames = `(?:${providerTokens
    .map((token) => token.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))
    .join("|")})`;

  if (/\.tf$/i.test(pathText)) {
    const localProviders = new Set([
      "docker",
      "local",
      "null",
      "postgresql",
      "random",
      "template",
      "time",
      "tls",
    ]);
    const providers = [...text.matchAll(/\bprovider\s+"([^"]+)"/gi)].map(
      (match) => match[1].toLowerCase(),
    );
    if (providers.some((provider) => !localProviders.has(provider)))
      return true;

    const backends = [...text.matchAll(/\bbackend\s+"([^"]+)"/gi)].map(
      (match) => match[1].toLowerCase(),
    );
    if (backends.some((backend) => backend !== "local")) return true;

    const moduleSources = [...text.matchAll(/\bsource\s*=\s*"([^"]+)"/gi)].map(
      (match) => match[1],
    );
    if (
      moduleSources.some(
        (source) =>
          !source.startsWith("./") &&
          !source.startsWith("../") &&
          !source.startsWith("/"),
      )
    )
      return true;
  }

  if (/\.(?:json|ya?ml)$/i.test(pathText)) {
    try {
      const document = parseYaml(text);
      const containsNonlocalProvider = (value) => {
        if (Array.isArray(value)) return value.some(containsNonlocalProvider);
        if (!value || typeof value !== "object") return false;
        return Object.entries(value).some(([key, child]) => {
          const normalized = key.toLowerCase().replace(/[^a-z0-9]+/g, "_");
          if (
            /^(?:provider|deploy_provider|deployment_provider|hosting_provider)$/.test(
              normalized,
            ) &&
            typeof child === "string"
          ) {
            return (
              !/^(?:docker|local|postgres|postgresql)$/i.test(child.trim()) &&
              !approvedProductionProvider(child)
            );
          }
          return containsNonlocalProvider(child);
        });
      };
      if (containsNonlocalProvider(document)) return true;
    } catch {
      // Fall through to conservative text patterns.
    }
  }
  const providerPolicyText = approvedProductionPath || approvedMonitoringPath
    ? text.replace(
        /(^|\n)(\s*(?:provider|deploy_provider|deployment_provider|hosting_provider)\s*[:=]\s*["']?)(?:cloudflare|grafana)\b/gi,
        "$1$2approved-production-provider",
      )
    : text;
  const patterns = [
    new RegExp(`\\bprovider\\s*[:=]\\s*["']?${providerNames}\\b`, "i"),
    new RegExp(
      `\\b(?:production )?hosting\\s*[:=]\\s*["']?${providerNames}\\b`,
      "i",
    ),
    new RegExp(`provider\\s+"${providerNames}"`, "i"),
    new RegExp(`resource\\s+"(?:${providerNames})[_-][^"\\s]+"`, "i"),
    new RegExp(
      `uses:\\s*(?:${providerNames})[^/\\s]*/[^\\s]*(?:deploy|branch|preview|environment|database)[^\\s]*@`,
      "i",
    ),
    /uses:\s*(?:cloudflare\/pages-action|vercel\/action|google-github-actions\/deploy|azure\/(?:webapps|functions)-deploy|aws-actions\/)/i,
    /https?:\/\/(?:[^/]+\.)?(?:vercel\.com|neon\.tech|supabase\.com|netlify\.app|render\.com|railway\.app|fly\.io)\b/i,
  ];
  return patterns.some((pattern) => pattern.test(providerPolicyText));
}

function inspect(path, bytes, scope, object = Buffer.alloc(0)) {
  const pathBuffer = Buffer.isBuffer(path) ? path : Buffer.from(path);
  const objectBuffer = Buffer.isBuffer(object) ? object : Buffer.from(object);
  const pathText = asText(pathBuffer);
  const text = asText(bytes);
  const lowerPath = pathText.toLowerCase();

  if (
    lowerPath.endsWith(".pdf") ||
    bytes.subarray(0, 5).toString("ascii") === "%PDF-"
  )
    add("pdf_artifact", scope, objectBuffer, pathBuffer);
  if (containsProhibitedTerm(pathText, text))
    add("prohibited_term", scope, objectBuffer, pathBuffer);
  if (hasSourceFingerprint(pathText, text))
    add("source_fingerprint", scope, objectBuffer, pathBuffer);
  if (hasCredential(pathText, text))
    add("credential", scope, objectBuffer, pathBuffer);
  if (fixtureViolatesPolicy(pathText, text))
    add("fixture_policy", scope, objectBuffer, pathBuffer);
  if (hasProviderCommitment(pathText, text))
    add("provider_commitment", scope, objectBuffer, pathBuffer);
}

function splitNul(buffer) {
  const records = [];
  let start = 0;
  for (let index = 0; index < buffer.length; index += 1) {
    if (buffer[index] !== 0) continue;
    records.push(buffer.subarray(start, index));
    start = index + 1;
  }
  if (start < buffer.length) records.push(buffer.subarray(start));
  return records.filter((record) => record.length > 0);
}

function scanTrackedFiles() {
  for (const path of splitNul(git(["ls-files", "-z"]))) {
    inspect(path, readFileSync(path), "tracked_file");
  }
}

function scanCommitMetadata() {
  const commits = asText(
    git(["rev-list", "--branches", "--tags"], "utf8"),
  )
    .split(/\r?\n/)
    .filter(Boolean);
  for (const object of commits) {
    inspect(
      Buffer.alloc(0),
      git(["cat-file", "commit", object]),
      "commit_metadata",
      Buffer.from(object),
    );
  }
}

function scanTagRefsAndAnnotatedMetadata() {
  const records = splitNul(
    git([
      "for-each-ref",
      "--format=%(objectname)%00%(refname)%00",
      "refs/tags",
    ]),
  );
  for (let index = 0; index + 1 < records.length; index += 2) {
    const tag = records[index];
    const refName = records[index + 1];
    const object = tag.toString("ascii").trim();
    inspect(refName, refName, "tag_ref", Buffer.from(object));
    if (
      !object ||
      asText(git(["cat-file", "-t", object], "utf8")).trim() !== "tag"
    )
      continue;
    inspect(
      Buffer.alloc(0),
      git(["cat-file", "tag", object]),
      "tag_metadata",
      tag,
    );
  }
}

function scanHistoryObjects() {
  const seen = new Set();
  const records = splitNul(
    git(["rev-list", "--objects", "-z", "--branches", "--tags"]),
  );
  for (let index = 0; index < records.length; index += 1) {
    const object = records[index];
    if (!/^[0-9a-f]{40}$/.test(object.toString("ascii"))) continue;
    let path = Buffer.alloc(0);
    const possiblePath = records[index + 1];
    if (possiblePath?.subarray(0, 5).equals(Buffer.from("path="))) {
      path = possiblePath.subarray(5);
      index += 1;
    }
    const objectText = object.toString("ascii");
    const key = `${objectText}\0${path.toString("hex")}`;
    if (seen.has(key)) continue;
    seen.add(key);
    if (asText(git(["cat-file", "-t", objectText], "utf8")).trim() !== "blob")
      continue;
    const size = Number(
      asText(git(["cat-file", "-s", objectText], "utf8")).trim(),
    );
    if (size > 8 * 1024 * 1024) {
      add("oversized_unscanned_blob", "history_blob", object, path);
      continue;
    }
    inspect(
      path,
      git(["cat-file", "blob", objectText]),
      "history_blob",
      object,
    );
  }
}

try {
  git(["rev-parse", "--is-inside-work-tree"]);
  scanTrackedFiles();
  scanCommitMetadata();
  scanTagRefsAndAnnotatedMetadata();
  scanHistoryObjects();
} catch {
  console.error("Confidentiality scan could not inspect the repository.");
  process.exit(2);
}

if (findings.size) {
  const values = [...findings.values()].sort((a, b) =>
    `${a.category}:${a.scope}:${a.locator}`.localeCompare(
      `${b.category}:${b.scope}:${b.locator}`,
    ),
  );
  console.log(`Confidentiality scan failed with ${values.length} finding(s).`);
  for (const finding of values.slice(0, 100)) {
    console.log(
      `category=${finding.category} scope=${finding.scope} location=${finding.locator}`,
    );
  }
  if (values.length > 100)
    console.log(`additional_findings=${values.length - 100}`);
  process.exit(1);
}

const fingerprint = createHash("sha256")
  .update(asText(git(["rev-parse", "HEAD"], "utf8")).trim())
  .digest("hex")
  .slice(0, 12);
console.log(
  `Confidentiality scan passed (revision fingerprint ${fingerprint}).`,
);
