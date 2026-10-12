import { useState } from "react";
import {
  Plus,
  Trash2,
  Copy,
  CheckCircle2,
  XCircle,
  Circle,
  LoaderCircle,
  FileJson,
  ChevronRight,
  X,
  Search,
} from "lucide-react";
import { pretty, methodTone, statusLabel, uid } from "./model.mjs";

export function IconButton({ icon: Icon, title, ...props }) {
  return (
    <button className="icon-button" title={title} aria-label={title} {...props}>
      <Icon size={16} />
    </button>
  );
}
export function Method({ value = "GET" }) {
  return <span className={`method ${methodTone(value)}`}>{value}</span>;
}
export function Status({ value }) {
  const Icon =
    value === "passed"
      ? CheckCircle2
      : value === "failed"
        ? XCircle
        : value === "running"
          ? LoaderCircle
          : Circle;
  return (
    <span className={`status ${value || ""}`}>
      <Icon size={14} className={value === "running" ? "spin" : ""} />
      {statusLabel(value)}
    </span>
  );
}
export function Empty({ title = "暂无内容", children, icon: Icon = FileJson }) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <Icon size={28} strokeWidth={1.3} />
      </div>
      <strong>{title}</strong>
      <p>{children}</p>
    </div>
  );
}
export function Tabs({ items, value, onChange, compact = false }) {
  return (
    <div className={`sub-tabs ${compact ? "compact" : ""}`} role="tablist">
      {items.map((item) => (
        <button
          key={item.id || item}
          role="tab"
          aria-selected={value === (item.id || item)}
          className={value === (item.id || item) ? "active" : ""}
          onClick={() => onChange(item.id || item)}
        >
          {item.label || item}
          {item.count != null && (
            <span className="tab-count">{item.count}</span>
          )}
        </button>
      ))}
    </div>
  );
}
export function Code({ value, label = "JSON 内容" }) {
  const raw = pretty(value) || "";
  const lines = raw.split("\n").slice(0, 1200);
  function tokenize(line) {
    return line
      .split(
        /("(?:\\.|[^"\\])*"(?:\s*:)?|\btrue\b|\bfalse\b|\bnull\b|\b-?\d+(?:\.\d+)?\b)/g,
      )
      .map((part, index) => (
        <span
          key={index}
          className={
            part.startsWith('"')
              ? part.trim().endsWith(":")
                ? "code-key"
                : "code-string"
              : /^(true|false|null|\d)/.test(part)
                ? "code-number"
                : ""
          }
        >
          {part}
        </span>
      ));
  }
  return (
    <div className="code-view" aria-label={label}>
      <div className="code-tools">
        <span>JSON</span>
        <IconButton
          icon={Copy}
          title="复制内容"
          onClick={() => window.testbench.copy(raw)}
        />
      </div>
      <pre>
        {lines.map((line, index) => (
          <div className="code-line" key={index}>
            <span className="line-number">{index + 1}</span>
            <code>{tokenize(line)}</code>
          </div>
        ))}
      </pre>
      {raw.split("\n").length > lines.length && (
        <p className="muted">显示前 1200 行，复制可获得完整内容。</p>
      )}
    </div>
  );
}
export function KeyValueEditor({
  rows,
  onChange,
  kind = "参数",
  disabled = false,
}) {
  const trailing = { id: "blank", key: "", value: "", enabled: true };
  function change(index, patch) {
    const next = rows.map((row) => ({ ...row }));
    if (index === rows.length) next.push({ ...trailing, id: uid("row") });
    next[index] = { ...next[index], ...patch };
    onChange(next);
  }
  return (
    <div className="kv-editor">
      <div className="kv-head">
        <span />
        <span>参数名</span>
        <span>参数值</span>
        <span />
      </div>
      {[...rows, trailing].map((row, index) => (
        <div className="kv-row" key={row.id}>
          <input
            aria-label={`启用${kind}${index + 1}`}
            type="checkbox"
            checked={row.enabled}
            disabled={disabled}
            onChange={(event) =>
              change(index, { enabled: event.target.checked })
            }
          />
          <input
            aria-label={`${kind}名称${index + 1}`}
            placeholder="添加参数"
            value={row.key}
            disabled={disabled}
            onChange={(event) => change(index, { key: event.target.value })}
          />
          <input
            aria-label={`${kind}值${index + 1}`}
            placeholder="参数值"
            value={row.value}
            disabled={disabled}
            onChange={(event) => change(index, { value: event.target.value })}
          />
          {index < rows.length && (
            <IconButton
              icon={Trash2}
              title={`删除${kind}${index + 1}`}
              disabled={disabled}
              onClick={() => onChange(rows.filter((_, i) => i !== index))}
            />
          )}
        </div>
      ))}
    </div>
  );
}
export function Modal({ title, children, onClose, wide = false }) {
  return (
    <div className="modal-shade" onClick={onClose}>
      <section
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={`modal ${wide ? "wide" : ""}`}
        onClick={(event) => event.stopPropagation()}
      >
        <header>
          <h3>{title}</h3>
          <IconButton icon={X} title="关闭弹窗" onClick={onClose} />
        </header>
        {children}
      </section>
    </div>
  );
}
export function SearchInput({
  value,
  onChange,
  placeholder = "搜索名称或路径",
}) {
  return (
    <label className="search-input">
      <Search size={14} />
      <input
        aria-label={placeholder}
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      {value && (
        <button aria-label="清除搜索" onClick={() => onChange("")}>
          <X size={12} />
        </button>
      )}
    </label>
  );
}
export function Breadcrumb({ items }) {
  return (
    <div className="breadcrumb">
      {items.map((item, index) => (
        <span key={index}>
          {index > 0 && <ChevronRight size={11} />}
          <span>{item}</span>
        </span>
      ))}
    </div>
  );
}
