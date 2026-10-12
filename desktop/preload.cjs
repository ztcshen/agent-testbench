const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld(
  "testbench",
  Object.freeze({
    connection: () => ipcRenderer.invoke("atb:connection"),
    request: (input) => ipcRenderer.invoke("atb:api", input),
    settings: () => ipcRenderer.invoke("atb:settings"),
    assistantSettings: () => ipcRenderer.invoke("atb:assistant-settings"),
    assistantSaveSettings: (input) =>
      ipcRenderer.invoke("atb:assistant-save-settings", input),
    assistantProbe: () => ipcRenderer.invoke("atb:assistant-probe"),
    assistantSessions: (mapId) =>
      ipcRenderer.invoke("atb:assistant-sessions", { mapId }),
    assistantSession: (id) =>
      ipcRenderer.invoke("atb:assistant-session", { id }),
    assistantStart: (input) => ipcRenderer.invoke("atb:assistant-start", input),
    assistantCancel: (id) => ipcRenderer.invoke("atb:assistant-cancel", { id }),
    onAssistantEvent: (callback) => {
      const listener = (_event, session) => callback(session);
      ipcRenderer.on("atb:assistant-event", listener);
      return () => ipcRenderer.removeListener("atb:assistant-event", listener);
    },
    connect: (input) => ipcRenderer.invoke("atb:connect", input),
    copy: (text) => ipcRenderer.invoke("atb:copy", String(text)),
    exportJSON: (name, value) =>
      ipcRenderer.invoke("atb:export", { name, value }),
    importJSON: () => ipcRenderer.invoke("atb:import"),
  }),
);
