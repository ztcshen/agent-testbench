export async function fetchJSON(path) {
  return requestJSON(path);
}

export async function requestJSON(path, options = {}) {
  const response = await fetch(path, {
    cache: "no-store",
    ...options,
    headers: {
      Accept: "application/json",
      ...(options.body ? { "content-type": "application/json" } : {}),
      ...(options.headers || {}),
    },
  });
  const body = await response.json();
  if (!response.ok) {
    const error = new Error(body.error || response.statusText);
    error.payload = body;
    error.status = response.status;
    throw error;
  }
  return body;
}

export function fetchCurrentStore() {
  return fetchJSON("/api/store/current");
}

export function listEnvironments({ all = false } = {}) {
  return fetchJSON(`/api/environments${all ? "?all=true" : ""}`);
}

export function registerEnvironment(environment) {
  return requestJSON("/api/environments", {
    method: "POST",
    body: JSON.stringify(environment || {}),
  });
}

export function inspectEnvironment(id) {
  return fetchJSON(`/api/environments/${encodeURIComponent(id)}`);
}

export function bootstrapEnvironment(id) {
  return fetchJSON(`/api/environments/${encodeURIComponent(id)}/bootstrap`);
}

export function verifyEnvironment(id, verification) {
  return requestJSON(`/api/environments/${encodeURIComponent(id)}/verify`, {
    method: "POST",
    body: JSON.stringify(verification || {}),
  });
}

export function publishVerifiedEnvironment(id) {
  return requestJSON(`/api/environments/${encodeURIComponent(id)}/publish-verified`, {
    method: "POST",
    body: JSON.stringify({}),
  });
}

export function classNames(...values) {
  return values.filter(Boolean).join(" ");
}

export function unique(values) {
  return [...new Set(values.filter(Boolean))];
}

export function compactText(value, defaultValue = "-") {
  const text = String(value || "").replace(/\s+/g, " ").trim();
  return text || defaultValue;
}
