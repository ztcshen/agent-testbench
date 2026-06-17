package main

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
)

func renderMapReviewHTML(document mapReviewDocument) (string, error) {
	raw, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	data := strings.ReplaceAll(string(raw), "</", "<\\/")
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>`)
	b.WriteString(html.EscapeString(document.Map.DisplayName))
	b.WriteString(` Map Review</title><style>`)
	b.WriteString(mapReviewCSS())
	b.WriteString(`</style></head><body><script type="application/json" id="map-review-data">`)
	b.WriteString(data)
	b.WriteString(`</script><div class="app"><header><div><h1>`)
	b.WriteString(html.EscapeString(stringDefault(document.Map.DisplayName, document.Map.ID)))
	b.WriteString(`</h1><p>`)
	b.WriteString(html.EscapeString(document.Map.ID))
	b.WriteString(` &middot; profile `)
	b.WriteString(html.EscapeString(document.Map.ProfileID))
	b.WriteString(`</p></div><div class="metrics"><span>nodes `)
	b.WriteString(strconv.Itoa(document.Counts.Nodes))
	b.WriteString(`</span><span>paths `)
	b.WriteString(strconv.Itoa(document.Counts.Paths))
	b.WriteString(`</span><span>steps `)
	b.WriteString(strconv.Itoa(document.Counts.PathSteps))
	b.WriteString(`</span><span>warnings `)
	b.WriteString(strconv.Itoa(document.Counts.Warnings))
	b.WriteString(`</span></div></header><section class="toolbar"><button id="back-node" type="button">Back</button><label>Search <input id="case-search" type="search" placeholder="case, node, template, workflow"></label><label>Workflow <select id="workflow-filter"><option value="">All workflows</option></select></label><button id="focus-node" type="button">Focus</button><button id="fit-selected" type="button">Fit</button><button id="path-finder-open" type="button">Path Finder</button><button id="reset-view" type="button">Reset</button></section><main><section class="graph-shell"><svg id="edge-layer" aria-hidden="true"></svg><div id="node-layer"></div><svg id="map-review-minimap" aria-label="Map minimap"></svg></section><aside><div id="node-history"></div><div id="details"></div></aside></main><div id="path-finder" class="modal hidden"><div class="modal-card"><div class="modal-head"><h2>Path Finder</h2><button id="path-finder-close" type="button">Close</button></div><div class="path-form"><label>From <select id="path-from"></select></label><label>To <select id="path-to"></select></label><button id="path-find-run" type="button">Find Path</button></div><div id="path-finder-result"></div></div></div></div><script>`)
	b.WriteString(mapReviewJS())
	b.WriteString(`</script></body></html>`)
	return b.String(), nil
}

func mapReviewCSS() string {
	return `:root{color-scheme:light;--bg:#f7f8fb;--panel:#fff;--ink:#172033;--muted:#65708a;--line:#d8deea;--soft:#eef2f8;--accent:#2563eb}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.app{height:100vh;display:flex;flex-direction:column}header{height:76px;display:flex;align-items:center;justify-content:space-between;gap:20px;padding:14px 20px;background:#111827;color:#fff;border-bottom:1px solid #0b1220}h1{font-size:20px;margin:0 0 4px}header p{margin:0;color:#aeb8cf;font-size:12px}.metrics{display:flex;gap:8px;flex-wrap:wrap;justify-content:flex-end}.metrics span{font-size:12px;background:rgba(255,255,255,.08);border:1px solid rgba(255,255,255,.14);border-radius:999px;padding:5px 9px}.toolbar{min-height:52px;display:flex;align-items:center;gap:9px;padding:8px 16px;background:var(--panel);border-bottom:1px solid var(--line);flex-wrap:wrap}label{font-size:12px;color:var(--muted);display:flex;align-items:center;gap:7px}input,select,button{font:inherit;border:1px solid var(--line);background:#fff;border-radius:7px;padding:7px 9px;color:var(--ink)}input{width:300px}button{cursor:pointer}button.active{border-color:#2563eb;background:#eff6ff;color:#1d4ed8}main{flex:1;display:grid;grid-template-columns:minmax(0,1fr) 410px;min-height:0}.graph-shell{position:relative;overflow:auto;background:linear-gradient(#f9fbff,#f4f7fc);min-height:0}#edge-layer{position:absolute;inset:0;min-width:1400px;min-height:780px;pointer-events:none}.edge{fill:none;stroke:#9aa8bd;stroke-width:2}.edge.path{stroke-width:3}.edge.fixture{stroke:#b45309;stroke-dasharray:7 5}.edge.dim{opacity:.12}#node-layer{position:relative;min-width:1400px;min-height:780px}.node{position:absolute;width:230px;min-height:96px;background:var(--panel);border:1px solid var(--line);border-left:5px solid var(--accent);border-radius:8px;box-shadow:0 7px 20px rgba(17,24,39,.08);padding:10px 11px;text-align:left;cursor:pointer;transition:.16s transform,.16s box-shadow,.16s opacity}.node:hover{transform:translateY(-1px);box-shadow:0 10px 25px rgba(17,24,39,.14)}.node.selected{outline:3px solid rgba(37,99,235,.24)}.node.focused{box-shadow:0 0 0 4px rgba(22,163,74,.18),0 10px 25px rgba(17,24,39,.14)}.node.dim{opacity:.18}.node-title{font-weight:700;font-size:13px;line-height:1.25;margin-bottom:6px}.node-meta{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:10px;color:var(--muted);word-break:break-all}.badges{display:flex;gap:5px;flex-wrap:wrap;margin-top:7px}.badge{font-size:10px;border-radius:999px;background:var(--soft);color:#475569;padding:3px 6px}.badge.warn{background:#fff7ed;color:#9a3412}.badge.ok{background:#ecfdf5;color:#047857}#map-review-minimap{position:absolute;right:14px;bottom:14px;width:178px;height:124px;border:1px solid var(--line);border-radius:8px;background:rgba(255,255,255,.9);box-shadow:0 8px 24px rgba(15,23,42,.14);z-index:4}.mini-node{cursor:pointer;fill:#94a3b8;stroke:#fff;stroke-width:10}.mini-node.selected{fill:#2563eb}.mini-node.dim{opacity:.2}aside{background:var(--panel);border-left:1px solid var(--line);overflow:auto}.empty{padding:28px;color:var(--muted)}#node-history{padding:9px 14px;border-bottom:1px solid var(--line);display:flex;gap:6px;align-items:center;flex-wrap:wrap;background:#fbfdff}.history-chip{font-size:11px;max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.detail{padding:18px}.detail h2{font-size:18px;margin:0 0 4px}.detail .sub{color:var(--muted);font-size:12px;margin-bottom:14px;word-break:break-all}.detail-actions{display:flex;gap:7px;flex-wrap:wrap;margin:8px 0 12px}.section{border-top:1px solid var(--line);padding-top:13px;margin-top:13px}.section h3{font-size:12px;text-transform:uppercase;letter-spacing:.08em;color:#475569;margin:0 0 9px}.kv{display:grid;grid-template-columns:120px minmax(0,1fr);gap:5px 10px;font-size:12px}.kv div:nth-child(odd){color:var(--muted)}.mini-link{border:0;background:transparent;color:#2563eb;padding:0;text-align:left;font-size:12px;word-break:break-all}.mini-link:hover{text-decoration:underline}pre{white-space:pre-wrap;word-break:break-word;background:#0f172a;color:#dbeafe;border-radius:7px;padding:10px;font-size:11px;line-height:1.45;max-height:240px;overflow:auto}.list{display:flex;flex-direction:column;gap:6px}.link-row{border:1px solid var(--line);border-radius:7px;background:#fbfdff;padding:8px;text-align:left}.link-row strong{font-size:12px}.link-row span{display:block;color:var(--muted);font-size:11px;margin-top:2px}.warning{background:#fff7ed;border:1px solid #fed7aa;color:#9a3412;border-radius:7px;padding:9px;font-size:12px;margin-bottom:8px}.modal{position:fixed;inset:0;background:rgba(15,23,42,.48);display:flex;align-items:center;justify-content:center;z-index:20}.hidden{display:none}.modal-card{width:min(720px,calc(100vw - 32px));max-height:82vh;overflow:auto;background:#fff;border-radius:10px;border:1px solid var(--line);box-shadow:0 22px 60px rgba(15,23,42,.25)}.modal-head{display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--line);padding:14px 16px}.modal-head h2{margin:0;font-size:18px}.path-form{display:grid;grid-template-columns:1fr 1fr auto;gap:10px;padding:14px 16px;border-bottom:1px solid var(--line)}#path-finder-result{padding:14px 16px}@media(max-width:900px){main{grid-template-columns:1fr}aside{height:42vh;border-left:0;border-top:1px solid var(--line)}input{width:170px}#map-review-minimap{display:none}.path-form{grid-template-columns:1fr}}`
}

func mapReviewJS() string {
	return strings.Join([]string{
		mapReviewStateJS(),
		mapReviewVisibilityJS(),
		mapReviewGraphJS(),
		mapReviewDetailJS(),
		mapReviewNavigationJS(),
		mapReviewBootJS(),
	}, "\n")
}

func mapReviewStateJS() string {
	return `const reviewData=JSON.parse(document.getElementById("map-review-data").textContent);
let selectedNodeId=reviewData.nodes[0]?.id||"";
let activePathId="";
let activeInterfaceKey="";
let searchText="";
let focusNodeId="";
let nodeHistory=[];
	const nodeById=new Map(reviewData.nodes.map(function(n){return [n.id,n]}));
	const pathById=new Map(reviewData.paths.map(function(p){return [p.id,p]}));
	const esc=function(v){return String(v??"").replace(/[&<>"']/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]})};
	const jsArg=function(v){return JSON.stringify(String(v??""))};
	const jsCall=function(name){return name+"("+Array.prototype.slice.call(arguments,1).map(jsArg).join(",")+")"};
	const jsHandler=function(){return esc(Array.prototype.slice.call(arguments).join(";"))};
	function pretty(v){if(!v||v==="{}"||v==="[]")return "";try{return JSON.stringify(JSON.parse(v),null,2)}catch{return String(v)}}
function hashState(){
  const params=new URLSearchParams(window.location.hash.replace(/^#/,""));
  return {node:params.get("node")||"",path:params.get("path")||"",interfaceKey:params.get("interface")||"",search:params.get("search")||"",focus:params.get("focus")||""};
}
function updateHash(replace){
  const params=new URLSearchParams();
  if(selectedNodeId)params.set("node",selectedNodeId);
  if(activePathId)params.set("path",activePathId);
  if(activeInterfaceKey)params.set("interface",activeInterfaceKey);
  if(searchText)params.set("search",searchText);
  if(focusNodeId)params.set("focus",focusNodeId);
  const url=window.location.pathname+window.location.search+(params.toString()?"#"+params.toString():"");
  (replace?history.replaceState:history.pushState).call(history,null,"",url);
}
function hydrateFromHash(){
  const state=hashState();
  if(state.node&&nodeById.has(state.node))selectedNodeId=state.node;
  activePathId=state.path&&pathById.has(state.path)?state.path:"";
  activeInterfaceKey=state.interfaceKey;
  searchText=state.search;
  focusNodeId=state.focus&&nodeById.has(state.focus)?state.focus:"";
  syncControls();
}
function navigateToState(next, push){
  if(Object.prototype.hasOwnProperty.call(next,"node")&&nodeById.has(next.node))selectedNodeId=next.node;
  if(Object.prototype.hasOwnProperty.call(next,"path"))activePathId=next.path&&pathById.has(next.path)?next.path:"";
  if(Object.prototype.hasOwnProperty.call(next,"interfaceKey"))activeInterfaceKey=next.interfaceKey||"";
  if(Object.prototype.hasOwnProperty.call(next,"search"))searchText=next.search||"";
  if(Object.prototype.hasOwnProperty.call(next,"focus"))focusNodeId=next.focus&&nodeById.has(next.focus)?next.focus:"";
  syncControls();renderAll();updateHash(!push);if(next.node)fitSelectedNode();
}
function syncControls(){
  document.getElementById("workflow-filter").value=activePathId;
  document.getElementById("case-search").value=searchText;
  document.getElementById("focus-node").classList.toggle("active",!!focusNodeId);
}`
}

func mapReviewVisibilityJS() string {
	return `function interfaceKey(n){return n.interfaceNodeId||n.requestTemplateId||""}
function interfaceCases(n){
  const key=interfaceKey(n);if(!key)return [];
  return reviewData.nodes.filter(function(item){return interfaceKey(item)===key}).sort(function(a,b){return (a.displayName||a.id).localeCompare(b.displayName||b.id)})
}
function isReverseCase(n){
  const role=String(n.role||"").toLowerCase(),caseType=String(n.caseType||"").toLowerCase(),state=String(n.stateEffect||"").toLowerCase(),mode=String(n.renderMode||"").toLowerCase();
  return role==="validation"||role==="negative"||caseType.includes("negative")||caseType.includes("validation")||state==="unchanged"||mode==="template_patch"||!!n.baseCaseId||!!n.anchorNodeId;
}
function interfaceReverseCases(n){return interfaceCases(n).filter(isReverseCase)}
function nodeTouchesPath(n,pathId){
  if(!pathId)return true;
  if((n.paths||[]).some(function(p){return p.pathId===pathId}))return true;
  return reviewData.edges.some(function(e){return e.pathId===pathId&&(e.fromNodeId===n.id||e.toNodeId===n.id)});
}
function nodeTouchesFocus(n){
  if(!focusNodeId)return true;
  if(n.id===focusNodeId)return true;
  return reviewData.edges.some(function(e){return (e.fromNodeId===focusNodeId&&e.toNodeId===n.id)||(e.toNodeId===focusNodeId&&e.fromNodeId===n.id)});
}
function nodeVisible(n){
  const pathText=(n.paths||[]).map(function(p){return p.pathId+" "+p.workflowId+" "+p.displayName}).join(" ");
  const hay=[n.id,n.caseId,n.displayName,n.interfaceNodeId,n.requestTemplateId,n.role,n.stateEffect,n.caseType,(n.tags||[]).join(" "),pathText].join(" ").toLowerCase();
  const textOK=!searchText||hay.includes(searchText.toLowerCase());
  const pathOK=nodeTouchesPath(n,activePathId);
  const interfaceOK=!activeInterfaceKey||interfaceKey(n)===activeInterfaceKey;
  return textOK&&pathOK&&interfaceOK&&nodeTouchesFocus(n);
}
function edgeVisible(e){
  if(!nodeById.has(e.fromNodeId)||!nodeById.has(e.toNodeId))return false;
  if(activePathId&&e.pathId&&e.pathId!==activePathId)return false;
  return nodeVisible(nodeById.get(e.fromNodeId))&&nodeVisible(nodeById.get(e.toNodeId));
}
function edgeColor(e){if(e.pathId&&pathById.has(e.pathId))return pathById.get(e.pathId).color;if(e.kind==="fixture")return "#b45309";return "#94a3b8"}`
}

func mapReviewGraphJS() string {
	return `function renderWorkflowFilter(){
  const select=document.getElementById("workflow-filter");
  if(select.options.length===1){
    for(const p of reviewData.paths){const o=document.createElement("option");o.value=p.id;o.textContent=(p.displayName||p.id)+" ("+p.steps.length+")";select.appendChild(o)}
  }
  select.onchange=function(){navigateToState({path:select.value,interfaceKey:""},true)};
  document.getElementById("case-search").oninput=function(e){navigateToState({search:e.target.value},false)};
  document.getElementById("reset-view").onclick=function(){navigateToState({path:"",interfaceKey:"",search:"",focus:""},true)};
  document.getElementById("back-node").onclick=goBackNode;
  document.getElementById("focus-node").onclick=toggleFocusNode;
  document.getElementById("fit-selected").onclick=fitSelectedNode;
  document.getElementById("path-finder-open").onclick=openPathFinder;
  document.getElementById("path-finder-close").onclick=closePathFinder;
  document.getElementById("path-find-run").onclick=findPath;
}
function renderGraph(){
  const svg=document.getElementById("edge-layer");
  const nodes=document.getElementById("node-layer");
  svg.innerHTML="";nodes.innerHTML="";
  let maxX=1200,maxY=680;
  for(const n of reviewData.nodes){maxX=Math.max(maxX,n.layout.x+320);maxY=Math.max(maxY,n.layout.y+180)}
  svg.setAttribute("width",maxX);svg.setAttribute("height",maxY);nodes.style.minWidth=maxX+"px";nodes.style.minHeight=maxY+"px";
  for(const e of reviewData.edges){
    const from=nodeById.get(e.fromNodeId),to=nodeById.get(e.toNodeId);if(!from||!to)continue;
    const hidden=!edgeVisible(e);const x1=from.layout.x+230,y1=from.layout.y+48,x2=to.layout.x,y2=to.layout.y+48;const mid=(x1+x2)/2;
    const p=document.createElementNS("http://www.w3.org/2000/svg","path");
    p.setAttribute("d","M "+x1+" "+y1+" C "+mid+" "+y1+", "+mid+" "+y2+", "+x2+" "+y2);
    p.setAttribute("class","edge "+(e.kind||"")+" "+(hidden?"dim":""));p.setAttribute("stroke",edgeColor(e));svg.appendChild(p);
  }
  for(const n of reviewData.nodes){
    const el=document.createElement("button");const visible=nodeVisible(n);el.type="button";
    el.className="node "+(n.id===selectedNodeId?"selected":"")+" "+(n.id===focusNodeId?"focused":"")+" "+(visible?"":"dim");
    el.style.left=n.layout.x+"px";el.style.top=n.layout.y+"px";
    const firstPath=n.paths&&n.paths[0];el.style.borderLeftColor=(firstPath&&pathById.get(firstPath.pathId)?.color)||"#2563eb";
    el.onclick=function(){selectNode(n.id)};
    const shared=n.sharedCount>1?'<span class="badge">shared x'+n.sharedCount+'</span>':"";
    el.innerHTML='<div class="node-title">'+esc(n.displayName)+'</div><div class="node-meta">'+esc(n.caseId||n.id)+'</div><div class="badges"><span class="badge">'+esc(n.role||"case")+'</span><span class="badge '+(n.stateEffect==="unchanged"?"warn":"ok")+'">'+esc(n.stateEffect||"")+'</span>'+shared+'</div>';
    nodes.appendChild(el);
  }
  renderMinimap(maxX,maxY);
}
function renderMinimap(maxX,maxY){
  const svg=document.getElementById("map-review-minimap");svg.innerHTML="";
  svg.setAttribute("viewBox","0 0 "+maxX+" "+maxY);
  for(const n of reviewData.nodes){
    const c=document.createElementNS("http://www.w3.org/2000/svg","circle");
    c.setAttribute("cx",n.layout.x+115);c.setAttribute("cy",n.layout.y+48);c.setAttribute("r",38);
    c.setAttribute("class","mini-node "+(n.id===selectedNodeId?"selected":"")+" "+(nodeVisible(n)?"":"dim"));
    c.addEventListener("click",function(){selectNode(n.id)});
    svg.appendChild(c);
  }
}`
}

func mapReviewDetailJS() string {
	return `function renderHistory(){
  const box=document.getElementById("node-history");
  const chips=nodeHistory.slice(-4).map(function(id,idx){
    const n=nodeById.get(id);if(!n)return "";
    const fullIndex=nodeHistory.length-Math.min(nodeHistory.length,4)+idx;
    return '<button class="history-chip" onclick="navigateToHistoryIndex('+fullIndex+')" title="'+esc(n.displayName)+'">'+esc(n.displayName)+'</button>';
  }).join("");
  box.innerHTML='<button type="button" onclick="goBackNode()">Back</button>'+chips+'<span class="sub">'+esc(activePathId||activeInterfaceKey||focusNodeId||"Map")+'</span>';
}
function renderDetails(){
  const box=document.getElementById("details");const n=nodeById.get(selectedNodeId);
  if(!n){box.innerHTML='<div class="empty">Select a case node.</div>';return}
  const incoming=reviewData.edges.filter(function(e){return e.toNodeId===n.id&&nodeById.has(e.fromNodeId)});
  const outgoing=reviewData.edges.filter(function(e){return e.fromNodeId===n.id&&nodeById.has(e.toNodeId)});
  const ops=n.explanation?.operations||[];const template=n.requestTemplate;
  const reverseCases=interfaceReverseCases(n),sameInterface=interfaceCases(n),key=interfaceKey(n);
  const jsonBlocks=[["Patch",n.patchJson],["Expected",n.expectedJson],["Required properties",n.requiredPropertyJson],["Provided properties",n.providedPropertyJson],["Request template",template?.templateJson]].filter(function(x){return pretty(x[1])});
  let html='<div class="detail"><h2>'+esc(n.displayName)+'</h2><div class="sub">'+esc(n.id)+'</div>';
  html+='<div class="detail-actions"><button onclick="toggleFocusNode()">Focus</button><button onclick="fitSelectedNode()">Fit</button><button onclick="copyCurrentLink()">Copy link</button><button onclick="openPathFinder()">Path Finder</button></div>';
  html+=reviewData.warnings.filter(function(w){return w.includes(n.id)}).map(function(w){return '<div class="warning">'+esc(w)+'</div>'}).join("");
  html+='<div class="section"><h3>Case</h3><div class="kv"><div>case id</div><div>'+esc(n.caseId)+'</div><div>type</div><div>'+esc(n.caseType||n.role)+'</div><div>state effect</div><div>'+esc(n.stateEffect)+'</div><div>interface</div><div>'+(key?'<button class="mini-link" onclick="'+jsHandler(jsCall("showInterfaceCases",n.id))+'">'+esc(key)+'</button>':"")+'</div><div>template</div><div>'+esc(n.requestTemplateId)+'</div><div>base case</div><div>'+esc(n.baseCaseId)+'</div><div>anchor</div><div>'+esc(n.anchorNodeId)+'</div></div></div>';
  if(template){html+='<div class="section"><h3>Request</h3><div class="kv"><div>method</div><div>'+esc(template.method)+'</div><div>path</div><div>'+esc(template.path)+'</div><div>template</div><div>'+esc(template.id)+'</div></div></div>'}
  html+='<div class="section"><h3>Interface reverse cases</h3><div class="sub">'+esc(key||"No interface key")+' - '+reverseCases.length+' reverse / '+sameInterface.length+' total</div><div class="list">'+(reverseCases.map(interfaceCaseRow).join("")||"<span class='sub'>No reverse cases in this review data.</span>")+'</div></div>';
  html+='<div class="section"><h3>Workflow usage</h3><div class="list">'+((n.paths||[]).map(function(p){return '<button class="link-row" onclick="'+jsHandler(jsCall("highlightPath",p.pathId))+'"><strong>'+esc(p.displayName||p.pathId)+'</strong><span>'+esc(p.workflowId)+' - step '+p.stepIndex+' - '+esc(p.stepId)+'</span></button>'}).join("")||"<span class='sub'>No workflow path usage.</span>")+'</div></div>';
  html+='<div class="section"><h3>Planner operations</h3><div class="list">'+(ops.map(function(o){return '<div class="link-row"><strong>'+esc(o.kind)+'</strong><span>'+esc(o.reason||"")+" "+esc(o.pathId||"")+" "+esc(o.untilNodeId||"")+" "+esc(o.caseId||"")+'</span></div>'}).join("")||"<span class='sub'>No operations.</span>")+'</div></div>';
  html+='<div class="section"><h3>Connections</h3><div class="list">'+incoming.map(function(e){return edgeRow(e,true)}).join("")+outgoing.map(function(e){return edgeRow(e,false)}).join("")+'</div></div>';
  html+=jsonBlocks.map(function(x){return '<div class="section"><h3>'+esc(x[0])+'</h3><pre>'+esc(pretty(x[1]))+'</pre></div>'}).join("")+'</div>';
  box.innerHTML=html;
}
function interfaceCaseRow(item){return '<button class="link-row" onclick="'+jsHandler(jsCall("selectNode",item.id))+'"><strong>'+esc(item.displayName)+'</strong><span>'+esc(item.caseId||item.id)+' - '+esc(item.caseType||item.role||"case")+' - '+esc(item.stateEffect||"")+'</span></button>'}
function edgeRow(e,incoming){const other=nodeById.get(incoming?e.fromNodeId:e.toNodeId);return '<button class="link-row" onclick="'+jsHandler(jsCall("selectNode",other.id))+'"><strong>'+(incoming?"in":"out")+' '+esc(e.kind)+'</strong><span>'+esc(other.displayName)+' - '+esc(e.pathId||e.materializationId||"")+'</span></button>'}`
}

func mapReviewNavigationJS() string {
	return `function selectNode(id){if(!nodeById.has(id))return;if(selectedNodeId&&selectedNodeId!==id)nodeHistory.push(selectedNodeId);navigateToState({node:id},true)}
function navigateToHistoryIndex(index){const id=nodeHistory[index];nodeHistory=nodeHistory.slice(0,index);if(id)navigateToState({node:id},true)}
function goBackNode(){const id=nodeHistory.pop();if(id)navigateToState({node:id},true)}
function highlightPath(id){navigateToState({path:id,interfaceKey:""},true)}
function showInterfaceCases(id){const n=nodeById.get(id);if(!n)return;navigateToState({interfaceKey:interfaceKey(n),path:"",search:""},true)}
function toggleFocusNode(){navigateToState({focus:focusNodeId?"":selectedNodeId},true)}
function fitSelectedNode(){
  const n=nodeById.get(selectedNodeId);const shell=document.querySelector(".graph-shell");if(!n||!shell)return;
  shell.scrollTo({left:Math.max(n.layout.x-120,0),top:Math.max(n.layout.y-90,0),behavior:"smooth"});
}
function copyCurrentLink(){updateHash(true);if(navigator.clipboard)navigator.clipboard.writeText(window.location.href).catch(function(){})}
function renderPathFinderOptions(){
  const opts='<option value="">Select a case...</option>'+reviewData.nodes.map(function(n){return '<option value="'+esc(n.id)+'">'+esc(n.displayName)+' ('+esc(n.caseId||n.id)+')</option>'}).join("");
  document.getElementById("path-from").innerHTML=opts;document.getElementById("path-to").innerHTML=opts;
}
function openPathFinder(){renderPathFinderOptions();document.getElementById("path-from").value=selectedNodeId;document.getElementById("path-finder-result").innerHTML="";document.getElementById("path-finder").classList.remove("hidden")}
function closePathFinder(){document.getElementById("path-finder").classList.add("hidden")}
function findPath(){
  const from=document.getElementById("path-from").value,to=document.getElementById("path-to").value,result=document.getElementById("path-finder-result");
  if(!from||!to||from===to){result.innerHTML='<div class="warning">Select two different nodes.</div>';return}
  const adj=new Map();
  for(const e of reviewData.edges){if(!adj.has(e.fromNodeId))adj.set(e.fromNodeId,[]);if(!adj.has(e.toNodeId))adj.set(e.toNodeId,[]);adj.get(e.fromNodeId).push(e.toNodeId)}
  const q=[{id:from,path:[from]}],seen=new Set([from]);let found=null;
  while(q.length){const cur=q.shift();if(cur.id===to){found=cur.path;break}for(const next of adj.get(cur.id)||[]){if(!seen.has(next)){seen.add(next);q.push({id:next,path:cur.path.concat(next)})}}}
  if(!found){result.innerHTML='<div class="warning">No path found.</div>';return}
  result.innerHTML='<div class="list">'+found.map(function(id,idx){const n=nodeById.get(id);return '<button class="link-row" onclick="'+jsHandler(jsCall("selectNode",id),"closePathFinder()")+'"><strong>'+String(idx+1)+'. '+esc(n?.displayName||id)+'</strong><span>'+esc(n?.caseId||id)+'</span></button>'}).join("")+'</div>';
}`
}

func mapReviewBootJS() string {
	return `function renderAll(){renderGraph();renderHistory();renderDetails()}
window.selectNode=selectNode;window.highlightPath=highlightPath;window.showInterfaceCases=showInterfaceCases;window.navigateToState=navigateToState;window.openPathFinder=openPathFinder;window.closePathFinder=closePathFinder;window.findPath=findPath;window.toggleFocusNode=toggleFocusNode;window.goBackNode=goBackNode;window.navigateToHistoryIndex=navigateToHistoryIndex;window.copyCurrentLink=copyCurrentLink;window.onpopstate=function(){hydrateFromHash();renderAll()};
renderWorkflowFilter();renderPathFinderOptions();hydrateFromHash();renderAll();updateHash(true);`
}
