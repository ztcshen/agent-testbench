import { fetchJSON } from "./api.js";

export { fetchJSON };

export function queryParam(name) {
  return new URLSearchParams(window.location.search).get(name) || "";
}

export function workflowIdFromURL() {
  return queryParam("workflow") || queryParam("id");
}

export function serviceName(services, serviceId) {
  return (services || []).find((service) => service.id === serviceId)?.displayName || serviceId || "-";
}

export function selectedWorkflow(catalog, id) {
  const workflows = catalog?.workflows || [];
  return workflows.find((workflow) => workflow.id === id) || workflows.find((workflow) => (workflow.steps || []).length) || workflows[0] || null;
}

export function selectedStep(workflow, stepId) {
  const steps = workflow?.steps || [];
  return steps.find((step) => step.id === stepId) || steps[0] || null;
}

export function statusTone(status) {
  const value = String(status || "").toLowerCase();
  if (["pass", "passed", "success", "ok"].includes(value)) return "passed";
  if (["fail", "failed", "error"].includes(value)) return "failed";
  return value || "unknown";
}

export function Panel({ title, summary, action, className = "", children }) {
  return (
    <section className={`workflow-run-panel ${className}`}>
      <div className="section-head">
        <div>
          <h2>{title}</h2>
          {summary ? <p>{summary}</p> : null}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

export function Stat({ label, value }) {
  return (
    <article>
      <span>{label}</span>
      <strong>{value}</strong>
    </article>
  );
}

export function Chip({ children }) {
  return <span className="workflow-detail-chip">{children}</span>;
}
