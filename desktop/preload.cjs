const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld(
  "testbench",
  Object.freeze({
    connection: () => ipcRenderer.invoke("atb:connection"),
    request: (input) => ipcRenderer.invoke("atb:api", input),
    settings: () => ipcRenderer.invoke("atb:settings"),
    connect: (input) => ipcRenderer.invoke("atb:connect", input),
    copy: (text) => ipcRenderer.invoke("atb:copy", String(text)),
    exportJSON: (name, value) =>
      ipcRenderer.invoke("atb:export", { name, value }),
    importJSON: () => ipcRenderer.invoke("atb:import"),
  }),
);
