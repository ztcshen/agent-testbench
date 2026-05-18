export function parseEvidenceBody(value) {
  if (value === undefined || value === null || value === "") return {};
  if (typeof value === "object") return value;
  if (typeof value !== "string") return {};
  try {
    return JSON.parse(quoteUnsafeJSONIntegers(value));
  } catch {
    return {};
  }
}

export function exportedValues(step, result) {
  const out = {};
  for (const item of step?.exports || []) {
    const name = item?.name;
    const value = valueAtPath(exportRoot(result, item?.from), item?.path);
    if (name && value !== undefined && value !== null && value !== "") {
      out[name] = normalizeExportValue(value);
    }
  }
  return out;
}

function exportRoot(result, source) {
  const request = requestEvidence(result);
  const response = responseEvidence(result);
  const responseBody = parseEvidenceBody(response.body);
  switch (source) {
    case "request":
    case "requestBody":
      return request.body || {};
    case "requestQuery":
      return request.query || {};
    case "response":
    case "responseBody":
      return responseBody;
    case "responseHeaders":
      return response.headers || {};
    default:
      return responseBody;
  }
}

function valueAtPath(root, path) {
  if (!path) return undefined;
  return String(path).split(".").reduce((current, part) => {
    if (current === undefined || current === null) return undefined;
    if (Array.isArray(current) && /^\d+$/.test(part)) return current[Number(part)];
    return current[part];
  }, root);
}

function requestEvidence(result) {
  return result?.result?.request || {};
}

function responseEvidence(result) {
  return result?.result?.response || {};
}

function quoteUnsafeJSONIntegers(value) {
  return String(value).replace(/([:[,]\s*)(-?\d{16,})(?=\s*[,}\]])/g, "$1\"$2\"");
}

function normalizeExportValue(value) {
  if (typeof value === "number" && Number.isInteger(value) && Math.abs(value) >= Number.MAX_SAFE_INTEGER) {
    return String(value);
  }
  return value;
}
