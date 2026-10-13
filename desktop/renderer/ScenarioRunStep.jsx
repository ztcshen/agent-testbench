import { useState } from "react";
import { ArrowUpRight, Braces } from "lucide-react";
import { Code, Status } from "./components.jsx";
import { evidenceFor } from "./api.mjs";

export default function ScenarioRunStep({
  item,
  index,
  step,
  openEvidence,
  locate,
}) {
  const [evidence, setEvidence] = useState(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  async function load(event) {
    if (!event.currentTarget.open || evidence || loading || !item.caseRunId)
      return;
    setLoading(true);
    try {
      setEvidence((await evidenceFor({ id: item.caseRunId })).data.evidence);
    } catch (error) {
      setError(error.message);
    } finally {
      setLoading(false);
    }
  }
  const group = evidence?.stepGroup;
  return (
    <details
      onToggle={load}
      open={item.status === "failed"}
      className="scenario-run-step"
    >
      <summary>
        <span>{index + 1}</span>
        <strong>
          {step?.group ? `${step.group.title} / ` : ""}
          {step?.title || item.displayName || item.caseId}
        </strong>
        <Status value={item.status} />
      </summary>
      <div className="scenario-run-actions">
        <button className="text-button" onClick={locate}>
          <ArrowUpRight size={13} />
          定位编排步骤
        </button>
        {item.caseRunId && (
          <button className="text-button" onClick={() => openEvidence?.(item)}>
            <Braces size={13} />
            查看运行证据
          </button>
        )}
      </div>
      {loading && <p className="muted">正在读取本次运行记录…</p>}
      {error && <div className="notice error">{error}</div>}
      {group && (
        <section className="scenario-run-snapshot">
          <strong>
            {group.title} · 本次引用版本 {group.revision?.slice(0, 8)}
          </strong>
          <table>
            <thead>
              <tr>
                <th>参数</th>
                <th>本次实际使用值</th>
              </tr>
            </thead>
            <tbody>
              {Object.entries(group.parameters || {}).map(([name, value]) => (
                <tr key={name}>
                  <td>
                    <code>{name}</code>
                  </td>
                  <td>
                    <code>
                      {typeof value === "string"
                        ? value
                        : JSON.stringify(value)}
                    </code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      {item.error && <div className="notice error">{item.error}</div>}
      {evidence && (
        <>
          <details>
            <summary>实际请求</summary>
            <Code value={evidence.request} />
          </details>
          <details>
            <summary>实际响应与断言</summary>
            <Code
              value={{
                response: evidence.response,
                assertions: evidence.assertions,
              }}
            />
          </details>
        </>
      )}
      <details>
        <summary>步骤运行记录</summary>
        <Code value={item} />
      </details>
    </details>
  );
}
