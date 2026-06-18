package main

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"

	"agent-testbench/internal/store"
)

func renderMapAtlasHTML(document mapAtlasDocument) (string, error) {
	document = normalizeMapAtlasDocument(document)
	raw, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	data := strings.ReplaceAll(string(raw), "</", "<\\/")
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>`)
	b.WriteString(html.EscapeString(document.Map.DisplayName))
	b.WriteString(` Test Scenario Atlas</title><style>`)
	b.WriteString(mapAtlasCSS())
	b.WriteString(`</style></head><body><script type="application/json" id="map-atlas-data">`)
	b.WriteString(data)
	b.WriteString(`</script><div class="app"><header><div><h1>`)
	b.WriteString(html.EscapeString(stringDefault(document.Map.DisplayName, document.Map.ID)))
	b.WriteString(` Test Scenario Atlas</h1><p>`)
	b.WriteString(html.EscapeString(document.Map.ID))
	b.WriteString(` &middot; profile `)
	b.WriteString(html.EscapeString(document.Map.ProfileID))
	b.WriteString(`</p></div><div class="metrics"><span id="metric-nodes">nodes `)
	b.WriteString(strconv.Itoa(document.Counts.Nodes))
	b.WriteString(`</span><span id="metric-paths">paths `)
	b.WriteString(strconv.Itoa(document.Counts.Paths))
	b.WriteString(`</span><span id="metric-steps">steps `)
	b.WriteString(strconv.Itoa(document.Counts.PathSteps))
	b.WriteString(`</span><span id="metric-warnings">warnings `)
	b.WriteString(strconv.Itoa(document.Counts.Warnings))
	b.WriteString(`</span></div></header><section class="toolbar"><button id="back-node" type="button">Back</button><label id="search-label"><span id="search-label-text">Search</span> <input id="case-search" type="search" placeholder="case, node, template, workflow"></label><label id="workflow-label"><span id="workflow-label-text">Workflow</span> <select id="workflow-filter"><option value="">All workflows</option></select></label><button id="focus-node" type="button">Focus</button><button id="toggle-validation" type="button">Validation</button><button id="fit-selected" type="button">Fit</button><button id="path-finder-open" type="button">Path Finder</button><button id="reset-view" type="button">Reset</button><label id="language-label"><span id="language-label-text">Language</span> <select id="language-select"><option value="en">EN</option><option value="zh">中文</option></select></label></section><main><section class="graph-shell"><svg id="edge-layer" aria-hidden="true"></svg><div id="node-layer"></div><svg id="map-atlas-minimap" aria-label="Map minimap"></svg></section><aside><div id="node-history"></div><div id="details"></div></aside></main><div id="path-finder" class="modal hidden"><div class="modal-card"><div class="modal-head"><h2 id="path-finder-title">Path Finder</h2><button id="path-finder-close" type="button">Close</button></div><div class="path-form"><label><span id="path-from-label">From</span> <select id="path-from"></select></label><label><span id="path-to-label">To</span> <select id="path-to"></select></label><button id="path-find-run" type="button">Find Path</button></div><div id="path-finder-result"></div></div></div></div><script>`)
	b.WriteString(mapAtlasJS())
	b.WriteString(`</script></body></html>`)
	return b.String(), nil
}

func normalizeMapAtlasDocument(document mapAtlasDocument) mapAtlasDocument {
	if document.Nodes == nil {
		document.Nodes = []mapAtlasNode{}
	}
	if document.Edges == nil {
		document.Edges = []mapAtlasEdge{}
	}
	if document.Paths == nil {
		document.Paths = []mapAtlasPath{}
	}
	for index := range document.Paths {
		if document.Paths[index].Steps == nil {
			document.Paths[index].Steps = []store.TestPlanPathStep{}
		}
	}
	if document.Materializations == nil {
		document.Materializations = []store.TestPlanMaterialization{}
	}
	if document.Warnings == nil {
		document.Warnings = []string{}
	}
	return document
}

func mapAtlasCSS() string {
	return `:root{color-scheme:light;--bg:#f7f8fb;--panel:#fff;--ink:#172033;--muted:#65708a;--line:#d8deea;--soft:#eef2f8;--accent:#2563eb}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.app{height:100vh;display:flex;flex-direction:column}header{height:76px;display:flex;align-items:center;justify-content:space-between;gap:20px;padding:14px 20px;background:#111827;color:#fff;border-bottom:1px solid #0b1220}h1{font-size:20px;margin:0 0 4px}header p{margin:0;color:#aeb8cf;font-size:12px}.metrics{display:flex;gap:8px;flex-wrap:wrap;justify-content:flex-end}.metrics span{font-size:12px;background:rgba(255,255,255,.08);border:1px solid rgba(255,255,255,.14);border-radius:999px;padding:5px 9px}.toolbar{min-height:52px;display:flex;align-items:center;gap:9px;padding:8px 16px;background:var(--panel);border-bottom:1px solid var(--line);flex-wrap:wrap}label{font-size:12px;color:var(--muted);display:flex;align-items:center;gap:7px}input,select,button{font:inherit;border:1px solid var(--line);background:#fff;border-radius:7px;padding:7px 9px;color:var(--ink)}input{width:300px}button{cursor:pointer}button.active{border-color:#2563eb;background:#eff6ff;color:#1d4ed8}main{flex:1;display:grid;grid-template-columns:minmax(0,1fr) 410px;min-height:0}.graph-shell{position:relative;overflow:auto;background:linear-gradient(#f9fbff,#f4f7fc);min-height:0}#edge-layer{position:absolute;inset:0;min-width:1400px;min-height:780px;pointer-events:none}.edge{fill:none;stroke:#9aa8bd;stroke-width:2}.edge.path{stroke-width:3}.edge.fixture{stroke:#b45309;stroke-dasharray:7 5}.edge.validation{stroke:#64748b;stroke-dasharray:3 4;opacity:.78}.edge.dim{opacity:.12}#node-layer{position:relative;min-width:1400px;min-height:780px}.node{position:absolute;width:230px;min-height:96px;background:var(--panel);border:1px solid var(--line);border-left:5px solid var(--accent);border-radius:8px;box-shadow:0 7px 20px rgba(17,24,39,.08);padding:10px 11px;text-align:left;cursor:pointer;transition:.16s transform,.16s box-shadow,.16s opacity}.node:hover{transform:translateY(-1px);box-shadow:0 10px 25px rgba(17,24,39,.14)}.node.selected{outline:3px solid rgba(37,99,235,.24)}.node.focused{box-shadow:0 0 0 4px rgba(22,163,74,.18),0 10px 25px rgba(17,24,39,.14)}.node.dim{opacity:.18}.node-title{font-weight:700;font-size:13px;line-height:1.25;margin-bottom:6px}.node-meta{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:10px;color:var(--muted);word-break:break-all}.badges{display:flex;gap:5px;flex-wrap:wrap;margin-top:7px}.badge{font-size:10px;border-radius:999px;background:var(--soft);color:#475569;padding:3px 6px}.badge.warn{background:#fff7ed;color:#9a3412}.badge.ok{background:#ecfdf5;color:#047857}#map-atlas-minimap{position:absolute;right:14px;bottom:14px;width:178px;height:124px;border:1px solid var(--line);border-radius:8px;background:rgba(255,255,255,.9);box-shadow:0 8px 24px rgba(15,23,42,.14);z-index:4}.mini-node{cursor:pointer;fill:#94a3b8;stroke:#fff;stroke-width:10}.mini-node.selected{fill:#2563eb}.mini-node.dim{opacity:.2}aside{background:var(--panel);border-left:1px solid var(--line);overflow:auto}.empty{padding:28px;color:var(--muted)}#node-history{padding:9px 14px;border-bottom:1px solid var(--line);display:flex;gap:6px;align-items:center;flex-wrap:wrap;background:#fbfdff}.history-chip{font-size:11px;max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.detail{padding:18px}.detail h2{font-size:18px;margin:0 0 4px}.detail .sub{color:var(--muted);font-size:12px;margin-bottom:14px;word-break:break-all}.detail-actions{display:flex;gap:7px;flex-wrap:wrap;margin:8px 0 12px}.section{border-top:1px solid var(--line);padding-top:13px;margin-top:13px}.section h3{font-size:12px;text-transform:uppercase;letter-spacing:.08em;color:#475569;margin:0 0 9px}.kv{display:grid;grid-template-columns:120px minmax(0,1fr);gap:5px 10px;font-size:12px}.kv div:nth-child(odd){color:var(--muted)}.mini-link{border:0;background:transparent;color:#2563eb;padding:0;text-align:left;font-size:12px;word-break:break-all}.mini-link:hover{text-decoration:underline}pre{white-space:pre-wrap;word-break:break-word;background:#0f172a;color:#dbeafe;border-radius:7px;padding:10px;font-size:11px;line-height:1.45;max-height:240px;overflow:auto}.list{display:flex;flex-direction:column;gap:6px}.link-row{border:1px solid var(--line);border-radius:7px;background:#fbfdff;padding:8px;text-align:left}.link-row strong{font-size:12px}.link-row span{display:block;color:var(--muted);font-size:11px;margin-top:2px}.family-grid{display:grid;grid-template-columns:1fr 1fr;gap:7px}.family-card{border:1px solid var(--line);border-radius:7px;background:#fbfdff;padding:8px;min-width:0}.family-card strong{display:block;font-size:12px;line-height:1.25}.family-count{display:inline-flex;margin-top:7px;border-radius:999px;background:#fff7ed;color:#9a3412;padding:3px 7px;font-size:11px}.family-meta{display:block;color:var(--muted);font-size:11px;margin-top:4px}.warning{background:#fff7ed;border:1px solid #fed7aa;color:#9a3412;border-radius:7px;padding:9px;font-size:12px;margin-bottom:8px}.modal{position:fixed;inset:0;background:rgba(15,23,42,.48);display:flex;align-items:center;justify-content:center;z-index:20}.hidden{display:none}.modal-card{width:min(720px,calc(100vw - 32px));max-height:82vh;overflow:auto;background:#fff;border-radius:10px;border:1px solid var(--line);box-shadow:0 22px 60px rgba(15,23,42,.25)}.modal-head{display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--line);padding:14px 16px}.modal-head h2{margin:0;font-size:18px}.path-form{display:grid;grid-template-columns:1fr 1fr auto;gap:10px;padding:14px 16px;border-bottom:1px solid var(--line)}#path-finder-result{padding:14px 16px}@media(max-width:900px){main{grid-template-columns:1fr}aside{height:42vh;border-left:0;border-top:1px solid var(--line)}input{width:170px}.family-grid{grid-template-columns:1fr}#map-atlas-minimap{display:none}.path-form{grid-template-columns:1fr}}`
}

func mapAtlasJS() string {
	return strings.Join([]string{
		mapAtlasStateJS(),
		mapAtlasI18NJS(),
		mapAtlasVisibilityJS(),
		mapAtlasGraphJS(),
		mapAtlasDetailJS(),
		mapAtlasNavigationJS(),
		mapAtlasBootJS(),
	}, "\n")
}

func mapAtlasStateJS() string {
	return `const atlasData=JSON.parse(document.getElementById("map-atlas-data").textContent);
	let selectedNodeId=atlasData.nodes[0]?.id||"";
	let activePathId="";
	let activeInterfaceKey="";
	let searchText="";
	let focusNodeId="";
	let showValidationNodes=false;
	let activeView="map";
	let language="en";
	let nodeHistory=[];
	const nodeById=new Map(atlasData.nodes.map(function(n){return [n.id,n]}));
	const pathById=new Map(atlasData.paths.map(function(p){return [p.id,p]}));
	const esc=function(v){return String(v??"").replace(/[&<>"']/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]})};
	const jsArg=function(v){return JSON.stringify(String(v??""))};
	const jsCall=function(name){return name+"("+Array.prototype.slice.call(arguments,1).map(jsArg).join(",")+")"};
	const jsHandler=function(){return esc(Array.prototype.slice.call(arguments).join(";"))};
	function pretty(v){if(!v||v==="{}"||v==="[]")return "";try{return JSON.stringify(JSON.parse(v),null,2)}catch{return String(v)}}
	function hashState(){
	  const params=new URLSearchParams(window.location.hash.replace(/^#/,""));
	  return {node:params.get("node")||"",path:params.get("path")||"",interfaceKey:params.get("interface")||"",search:params.get("search")||"",focus:params.get("focus")||"",validation:params.get("validation")==="1",view:params.get("view")||"map",lang:params.get("lang")||"en"};
	}
function updateHash(replace){
  const params=new URLSearchParams();
  if(selectedNodeId)params.set("node",selectedNodeId);
  if(activePathId)params.set("path",activePathId);
	  if(activeInterfaceKey)params.set("interface",activeInterfaceKey);
	  if(searchText)params.set("search",searchText);
	  if(focusNodeId)params.set("focus",focusNodeId);
	  if(showValidationNodes)params.set("validation","1");
	  if(activeView!=="map")params.set("view",activeView);
	  if(language!=="en")params.set("lang",language);
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
	  showValidationNodes=state.validation;
	  activeView=state.view==="interface"||state.interfaceKey?"interface":"map";
	  language=state.lang==="zh"?"zh":"en";
	  syncControls();
	}
function navigateToState(next, push){
  if(Object.prototype.hasOwnProperty.call(next,"node")&&nodeById.has(next.node))selectedNodeId=next.node;
  if(Object.prototype.hasOwnProperty.call(next,"path"))activePathId=next.path&&pathById.has(next.path)?next.path:"";
	  if(Object.prototype.hasOwnProperty.call(next,"interfaceKey"))activeInterfaceKey=next.interfaceKey||"";
	  if(Object.prototype.hasOwnProperty.call(next,"search"))searchText=next.search||"";
	  if(Object.prototype.hasOwnProperty.call(next,"focus"))focusNodeId=next.focus&&nodeById.has(next.focus)?next.focus:"";
	  if(Object.prototype.hasOwnProperty.call(next,"showValidationNodes"))showValidationNodes=!!next.showValidationNodes;
	  if(Object.prototype.hasOwnProperty.call(next,"view"))activeView=next.view==="interface"?"interface":"map";
	  if(Object.prototype.hasOwnProperty.call(next,"language"))language=next.language==="zh"?"zh":"en";
	  if(activeView==="map"&&Object.prototype.hasOwnProperty.call(next,"interfaceKey")&&!next.interfaceKey)activeInterfaceKey="";
	  syncControls();renderAll();updateHash(!push);if(next.node)fitSelectedNode();
	}
function syncControls(){
	  document.getElementById("workflow-filter").value=activePathId;
	  document.getElementById("case-search").value=searchText;
	  document.getElementById("focus-node").classList.toggle("active",!!focusNodeId);
	  document.getElementById("toggle-validation").classList.toggle("active",activeView==="interface");
	  document.getElementById("language-select").value=language;
	  syncText();
	}`
}

func mapAtlasI18NJS() string {
	return `const translations={
  en:{
    back:"Back",backToMap:"Back to map",search:"Search",workflow:"Workflow",allWorkflows:"All workflows",focus:"Focus",validation:"Validation",fit:"Fit",pathFinder:"Path Finder",reset:"Reset",language:"Language",close:"Close",from:"From",to:"To",findPath:"Find Path",searchPlaceholder:"case, node, template, workflow",
    nodes:"nodes",paths:"paths",steps:"steps",warnings:"warnings",map:"Map",interfaceDetail:"Interface detail",interfaceOverview:"Interface overview",selectedCase:"Selected case",primaryAnchors:"Primary anchors",testFamilies:"Test families",casePreview:"Case preview",openInterfaceDetail:"Open interface detail",showValidationGraph:"Show on map",showingFirst:"Showing first",of:"of",cases:"cases",run:"run",failed:"failed",notRun:"not run",validationCases:"validation cases",totalCases:"total cases",anchor:"anchor",
    mapRunPlan:"Map run plan",caseSection:"Case",request:"Request",runTasks:"Run tasks",workflowUsage:"Workflow usage",plannerOperations:"Planner operations",connections:"Connections",plan:"plan",status:"status",scope:"scope",caseID:"case id",type:"type",stateEffect:"state effect",interface:"interface",template:"template",baseCase:"base case",method:"method",path:"path",patch:"Patch",expected:"Expected",requiredProperties:"Required properties",providedProperties:"Provided properties",requestTemplate:"Request template",
    selectCaseNode:"Select a case node.",noRunTask:"No saved run task is linked to this node.",noValidationFamilies:"No validation families.",noWorkflowUsage:"No workflow path usage.",noOperations:"No operations.",selectCase:"Select a case...",selectDifferent:"Select two different nodes.",noPathFound:"No path found.",copied:"Copy link",shared:"shared",validationBadge:"validation",
    familyAuth:"Auth and signature",familyRequired:"Required fields",familyBlank:"Blank and null",familyLength:"Length limits",familyType:"Type checks",familyInvalid:"Invalid values",familyIdempotency:"Idempotency",familyConcurrency:"Concurrency",familyContract:"Contract validation"
  },
  zh:{
    back:"返回",backToMap:"返回主图",search:"搜索",workflow:"工作流",allWorkflows:"全部工作流",focus:"聚焦",validation:"验证用例",fit:"定位",pathFinder:"路径查找",reset:"重置",language:"语言",close:"关闭",from:"起点",to:"终点",findPath:"查找路径",searchPlaceholder:"case、节点、模板、工作流",
    nodes:"节点",paths:"路径",steps:"步骤",warnings:"告警",map:"主图",interfaceDetail:"接口详情",interfaceOverview:"接口概览",selectedCase:"当前用例",primaryAnchors:"基准节点",testFamilies:"测试用例族",casePreview:"用例预览",openInterfaceDetail:"打开接口详情",showValidationGraph:"回到主图定位",showingFirst:"仅显示前",of:"共",cases:"条用例",run:"已运行",failed:"失败",notRun:"未运行",validationCases:"负向/验证用例",totalCases:"全部用例",anchor:"锚点",
    mapRunPlan:"Map run plan / 运行计划",caseSection:"用例",request:"请求",runTasks:"Run tasks / 运行任务",workflowUsage:"工作流使用",plannerOperations:"Planner 操作",connections:"连接关系",plan:"计划",status:"状态",scope:"范围",caseID:"用例 ID",type:"类型",stateEffect:"状态影响",interface:"接口",template:"模板",baseCase:"基准用例",method:"方法",path:"路径",patch:"Patch",expected:"期望结果",requiredProperties:"必需属性",providedProperties:"已提供属性",requestTemplate:"请求模板",
    selectCaseNode:"请选择一个 case 节点。",noRunTask:"这个节点没有关联已保存的运行任务。",noValidationFamilies:"暂无验证用例族。",noWorkflowUsage:"暂无工作流路径使用。",noOperations:"暂无 Planner 操作。",selectCase:"选择一个 case...",selectDifferent:"请选择两个不同节点。",noPathFound:"没有找到路径。",copied:"复制链接",shared:"复用",validationBadge:"验证",
    familyAuth:"鉴权与签名",familyRequired:"必填字段",familyBlank:"空值/Null",familyLength:"长度限制",familyType:"类型校验",familyInvalid:"非法取值",familyIdempotency:"幂等性",familyConcurrency:"并发",familyContract:"契约校验"
  }
};
function tr(key){return (translations[language]&&translations[language][key])||translations.en[key]||key}
function syncText(){
  document.documentElement.lang=language==="zh"?"zh-CN":"en";
  document.getElementById("back-node").textContent=activeView==="interface"?tr("backToMap"):tr("back");
  document.getElementById("search-label-text").textContent=tr("search");
  document.getElementById("workflow-label-text").textContent=tr("workflow");
  document.getElementById("focus-node").textContent=tr("focus");
  document.getElementById("toggle-validation").textContent=tr("validation");
  document.getElementById("fit-selected").textContent=tr("fit");
  document.getElementById("path-finder-open").textContent=tr("pathFinder");
  document.getElementById("reset-view").textContent=tr("reset");
  document.getElementById("language-label-text").textContent=tr("language");
  document.getElementById("path-finder-title").textContent=tr("pathFinder");
  document.getElementById("path-finder-close").textContent=tr("close");
  document.getElementById("path-from-label").textContent=tr("from");
  document.getElementById("path-to-label").textContent=tr("to");
  document.getElementById("path-find-run").textContent=tr("findPath");
  document.getElementById("case-search").placeholder=tr("searchPlaceholder");
  document.getElementById("metric-nodes").textContent=tr("nodes")+" "+atlasData.counts.nodes;
  document.getElementById("metric-paths").textContent=tr("paths")+" "+atlasData.counts.paths;
  document.getElementById("metric-steps").textContent=tr("steps")+" "+atlasData.counts.pathSteps;
  document.getElementById("metric-warnings").textContent=tr("warnings")+" "+atlasData.counts.warnings;
  const first=document.querySelector("#workflow-filter option[value='']");
  if(first)first.textContent=tr("allWorkflows");
}`
}

func mapAtlasVisibilityJS() string {
	return `function interfaceKey(n){return n.interfaceNodeId||n.requestTemplateId||""}
	function interfaceCasesByKey(key){
	  if(!key)return [];
	  return atlasData.nodes.filter(function(item){return interfaceKey(item)===key}).sort(function(a,b){return (a.displayName||a.id).localeCompare(b.displayName||b.id)})
	}
	function interfaceCases(n){
	  const key=interfaceKey(n);if(!key)return [];
	  return interfaceCasesByKey(key)
}
function isReverseCase(n){
  const role=String(n.role||"").toLowerCase(),caseType=String(n.caseType||"").toLowerCase(),state=String(n.stateEffect||"").toLowerCase(),mode=String(n.renderMode||"").toLowerCase();
  return role==="validation"||role==="negative"||caseType.includes("negative")||caseType.includes("validation")||state==="unchanged"||mode==="template_patch"||!!n.baseCaseId||!!n.anchorNodeId;
	}
	function interfaceReverseCases(n){return interfaceCases(n).filter(isReverseCase)}
	function interfacePrimaryCasesByKey(key){return interfaceCasesByKey(key).filter(function(item){return !isReverseCase(item)})}
	function interfaceReverseCasesByKey(key){return interfaceCasesByKey(key).filter(isReverseCase)}
	function nodeSearchText(n){
	  const pathText=(n.paths||[]).map(function(p){return p.pathId+" "+p.workflowId+" "+p.displayName}).join(" ");
	  return [n.id,n.caseId,n.displayName,n.interfaceNodeId,n.requestTemplateId,n.role,n.stateEffect,n.caseType,(n.tags||[]).join(" "),pathText].join(" ").toLowerCase();
	}
	function currentValidationScopeKey(){
	  const selected=nodeById.get(selectedNodeId),focused=nodeById.get(focusNodeId);
	  return activeInterfaceKey||interfaceKey(selected)||interfaceKey(focused)||"";
	}
	function validationNodeInScope(n){
	  if(!isReverseCase(n))return true;
	  if(n.id===selectedNodeId||n.id===focusNodeId)return true;
	  return false;
	}
	function nodeDrawn(n){
	  if(!isReverseCase(n))return true;
	  return validationNodeInScope(n);
	}
	function nodeTouchesPath(n,pathId){
  if(!pathId)return true;
  if((n.paths||[]).some(function(p){return p.pathId===pathId}))return true;
  return atlasData.edges.some(function(e){return e.pathId===pathId&&(e.fromNodeId===n.id||e.toNodeId===n.id)});
}
function nodeTouchesFocus(n){
  if(!focusNodeId)return true;
  if(n.id===focusNodeId)return true;
  return atlasData.edges.some(function(e){return (e.fromNodeId===focusNodeId&&e.toNodeId===n.id)||(e.toNodeId===focusNodeId&&e.fromNodeId===n.id)});
	}
	function nodeVisible(n){
	  if(!nodeDrawn(n))return false;
	  const textOK=!searchText||nodeSearchText(n).includes(searchText.toLowerCase());
	  const pathOK=nodeTouchesPath(n,activePathId);
	  const interfaceOK=!activeInterfaceKey||interfaceKey(n)===activeInterfaceKey;
	  return textOK&&pathOK&&interfaceOK&&nodeTouchesFocus(n);
	}
	function edgeVisible(e){
	  if(!nodeById.has(e.fromNodeId)||!nodeById.has(e.toNodeId))return false;
	  if(!nodeDrawn(nodeById.get(e.fromNodeId))||!nodeDrawn(nodeById.get(e.toNodeId)))return false;
	  if(e.kind==="validation"&&!validationEdgeInScope(e))return false;
	  if(activePathId&&e.pathId&&e.pathId!==activePathId)return false;
	  return nodeVisible(nodeById.get(e.fromNodeId))&&nodeVisible(nodeById.get(e.toNodeId));
	}
	function validationEdgeInScope(e){
	  if(e.kind!=="validation")return true;
	  if(e.fromNodeId===selectedNodeId||e.toNodeId===selectedNodeId||e.fromNodeId===focusNodeId||e.toNodeId===focusNodeId)return true;
	  return false;
	}
	function edgeDrawn(e){
	  if(!nodeById.has(e.fromNodeId)||!nodeById.has(e.toNodeId))return false;
	  if(!nodeDrawn(nodeById.get(e.fromNodeId))||!nodeDrawn(nodeById.get(e.toNodeId)))return false;
	  return e.kind!=="validation"||validationEdgeInScope(e);
	}
function edgeColor(e){if(e.pathId&&pathById.has(e.pathId))return pathById.get(e.pathId).color;if(e.kind==="fixture")return "#b45309";return "#94a3b8"}`
}

func mapAtlasGraphJS() string {
	return `function renderWorkflowFilter(){
  const select=document.getElementById("workflow-filter");
  if(select.options.length===1){
    for(const p of atlasData.paths){const steps=p.steps||[];const o=document.createElement("option");o.value=p.id;o.textContent=(p.displayName||p.id)+" ("+steps.length+")";select.appendChild(o)}
  }
  select.onchange=function(){navigateToState({path:select.value,interfaceKey:"",view:"map"},true)};
  document.getElementById("case-search").oninput=function(e){navigateToState({search:e.target.value},false)};
  document.getElementById("reset-view").onclick=function(){navigateToState({path:"",interfaceKey:"",search:"",focus:"",view:"map"},true)};
	  document.getElementById("back-node").onclick=goBackNode;
	  document.getElementById("focus-node").onclick=toggleFocusNode;
	  document.getElementById("toggle-validation").onclick=toggleValidationNodes;
	  document.getElementById("fit-selected").onclick=fitSelectedNode;
  document.getElementById("path-finder-open").onclick=openPathFinder;
  document.getElementById("path-finder-close").onclick=closePathFinder;
  document.getElementById("path-find-run").onclick=findPath;
  document.getElementById("language-select").onchange=function(e){navigateToState({language:e.target.value},true)};
}
function renderGraph(){
  const svg=document.getElementById("edge-layer");
  const nodes=document.getElementById("node-layer");
  svg.innerHTML="";nodes.innerHTML="";
  setInterfaceMode(false);
  nodes.className="";document.getElementById("map-atlas-minimap").style.display="";
  let maxX=1200,maxY=680;
  for(const n of atlasData.nodes){maxX=Math.max(maxX,n.layout.x+320);maxY=Math.max(maxY,n.layout.y+180)}
	  svg.setAttribute("width",maxX);svg.setAttribute("height",maxY);nodes.style.minWidth=maxX+"px";nodes.style.minHeight=maxY+"px";
  renderArrowMarkers(svg);
	  for(const e of atlasData.edges){
	    const from=nodeById.get(e.fromNodeId),to=nodeById.get(e.toNodeId);if(!from||!to)continue;
	    if(!edgeDrawn(e))continue;
	    const hidden=!edgeVisible(e);
    const color=edgeColor(e);
    const p=document.createElementNS("http://www.w3.org/2000/svg","path");
    p.setAttribute("d",edgePath(from,to));
    p.setAttribute("class","edge "+(e.kind||"")+" "+(hidden?"dim":""));p.setAttribute("stroke",color);p.setAttribute("marker-end","url(#"+arrowMarkerID(color)+")");svg.appendChild(p);
	  }
	  for(const n of atlasData.nodes){
	    if(!nodeDrawn(n))continue;
	    const el=document.createElement("button");const visible=nodeVisible(n);el.type="button";
    el.className="node "+(n.id===selectedNodeId?"selected":"")+" "+(n.id===focusNodeId?"focused":"")+" "+(visible?"":"dim");
    el.style.left=n.layout.x+"px";el.style.top=n.layout.y+"px";
    const firstPath=n.paths&&n.paths[0];el.style.borderLeftColor=(firstPath&&pathById.get(firstPath.pathId)?.color)||"#2563eb";
	    el.onclick=function(){selectNode(n.id)};
	    const shared=n.sharedCount>1?'<span class="badge">'+esc(tr("shared"))+' x'+n.sharedCount+'</span>':"";
	    const reverseCount=!isReverseCase(n)?interfaceReverseCases(n).length:0;
	    const validationBadge=reverseCount?'<span class="badge warn">'+esc(tr("validationBadge"))+' x'+reverseCount+'</span>':"";
	    el.innerHTML='<div class="node-title">'+esc(n.displayName)+'</div><div class="node-meta">'+esc(n.caseId||n.id)+'</div><div class="badges"><span class="badge">'+esc(n.role||"case")+'</span><span class="badge '+(n.stateEffect==="unchanged"?"warn":"ok")+'">'+esc(n.stateEffect||"")+'</span>'+shared+validationBadge+'</div>';
	    nodes.appendChild(el);
  }
  renderMinimap(maxX,maxY);
}
function arrowMarkerID(color){return "map-atlas-arrow-"+String(color||"").replace(/[^a-zA-Z0-9_-]/g,"")}
function renderArrowMarkers(svg){
  const defs=document.createElementNS("http://www.w3.org/2000/svg","defs");
  const colors=new Set(["#94a3b8","#b45309"]);
  for(const p of atlasData.paths||[]){if(p.color)colors.add(p.color)}
  for(const color of colors){
    const marker=document.createElementNS("http://www.w3.org/2000/svg","marker");
    marker.setAttribute("id",arrowMarkerID(color));marker.setAttribute("viewBox","0 0 10 10");marker.setAttribute("refX","8.6");marker.setAttribute("refY","5");marker.setAttribute("markerWidth","7");marker.setAttribute("markerHeight","7");marker.setAttribute("orient","auto-start-reverse");
    const arrow=document.createElementNS("http://www.w3.org/2000/svg","path");
    arrow.setAttribute("d","M 0 0 L 10 5 L 0 10 z");arrow.setAttribute("fill",color);
    marker.appendChild(arrow);defs.appendChild(marker);
  }
  svg.appendChild(defs);
}
function edgePath(from,to){
  const ports=edgePorts(from,to),x1=ports.from.x,y1=ports.from.y,x2=ports.to.x,y2=ports.to.y;
  if(ports.mode==="row"){const mid=(x1+x2)/2;return "M "+x1+" "+y1+" C "+mid+" "+y1+", "+mid+" "+y2+", "+x2+" "+y2}
  const midY=(y1+y2)/2;
  return "M "+x1+" "+y1+" L "+x1+" "+midY+" L "+x2+" "+midY+" L "+x2+" "+y2;
}
function edgePorts(from,to){
  const fromLeft={x:from.layout.x,y:from.layout.y+48},fromRight={x:from.layout.x+230,y:from.layout.y+48},fromBottom={x:from.layout.x+115,y:from.layout.y+96};
  const toLeft={x:to.layout.x,y:to.layout.y+48},toRight={x:to.layout.x+230,y:to.layout.y+48},toTop={x:to.layout.x+115,y:to.layout.y};
  if(Math.abs(from.layout.y-to.layout.y)<80){
    if(to.layout.x>=from.layout.x)return {from:fromRight,to:toLeft,mode:"row"};
    return {from:fromLeft,to:toRight,mode:"row"};
  }
  return {from:fromBottom,to:toTop,mode:"fold"};
}
function renderMinimap(maxX,maxY){
  const svg=document.getElementById("map-atlas-minimap");svg.innerHTML="";
	  svg.setAttribute("viewBox","0 0 "+maxX+" "+maxY);
	  for(const n of atlasData.nodes){
	    if(!nodeDrawn(n))continue;
	    const c=document.createElementNS("http://www.w3.org/2000/svg","circle");
    c.setAttribute("cx",n.layout.x+115);c.setAttribute("cy",n.layout.y+48);c.setAttribute("r",38);
    c.setAttribute("class","mini-node "+(n.id===selectedNodeId?"selected":"")+" "+(nodeVisible(n)?"":"dim"));
    c.addEventListener("click",function(){selectNode(n.id)});
    svg.appendChild(c);
  }
}
function validationAnchor(item, primaries){
  const explicit=item.anchorNodeId||item.baseCaseId;
  if(explicit&&nodeById.has(explicit))return nodeById.get(explicit);
  return primaries[0]||null;
}
function setInterfaceMode(enabled){
  const main=document.querySelector("main"),shell=document.querySelector(".graph-shell"),aside=document.querySelector("aside");
  if(main){main.classList.toggle("interface-main",enabled);main.style.gridTemplateColumns=enabled?"minmax(0,1fr)":""}
  if(shell){shell.classList.toggle("interface-mode",enabled);shell.style.background=enabled?"#f7f8fb":""}
  if(aside)aside.style.display=enabled?"none":"";
}
function selectedCaseSummary(current, primaries){
  if(!current)return "";
  const anchor=isReverseCase(current)?validationAnchor(current,primaries):null;
  const tasks=tasksForNode(current);
  const taskStatus=tasks.length?tasks.map(function(t){return t.status||""}).filter(Boolean).join(", "):tr("notRun");
  return '<div class="section"><h3>'+esc(tr("selectedCase"))+'</h3><div class="kv"><div>'+esc(tr("caseID"))+'</div><div>'+esc(current.caseId||current.id)+'</div><div>'+esc(tr("type"))+'</div><div>'+esc(current.caseType||current.role||"case")+'</div><div>'+esc(tr("stateEffect"))+'</div><div>'+esc(current.stateEffect||"")+'</div><div>'+esc(tr("anchor"))+'</div><div>'+esc(anchor?(anchor.displayName+" / "+(anchor.caseId||anchor.id)):(current.anchorNodeId||current.baseCaseId||""))+'</div><div>'+esc(tr("runTasks"))+'</div><div>'+esc(taskStatus)+'</div></div></div>';
}
function renderInterfaceView(){
  const svg=document.getElementById("edge-layer"),nodes=document.getElementById("node-layer"),mini=document.getElementById("map-atlas-minimap");
  svg.innerHTML="";svg.setAttribute("width",0);svg.setAttribute("height",0);mini.style.display="none";
  setInterfaceMode(true);
  nodes.className="";nodes.style.minWidth="100%";nodes.style.minHeight="100%";
  const current=nodeById.get(selectedNodeId);
  const key=activeInterfaceKey||interfaceKey(current)||"";
  const all=interfaceCasesByKey(key),primaries=all.filter(function(item){return !isReverseCase(item)}),validations=all.filter(isReverseCase),families=caseFamilySummaries(validations);
  const title=current?current.displayName:key;
  const primaryCards=primaries.map(function(item){
    const count=validations.filter(function(v){const anchor=validationAnchor(v,primaries);return anchor&&anchor.id===item.id}).length;
    return '<button class="link-row" onclick="'+jsHandler(jsCall("selectNode",item.id))+'"><strong>'+esc(item.displayName)+'</strong><span>'+esc(item.caseId||item.id)+'</span><span style="float:right;border-radius:999px;background:#eff6ff;color:#1d4ed8;padding:3px 7px;font-size:11px">'+esc(tr("validationBadge"))+' x'+count+'</span></button>';
  }).join("")||'<span class="sub">'+esc(tr("selectCaseNode"))+'</span>';
  const familyCards=families.map(function(f){
    const status=f.run?f.run+" "+tr("run")+(f.failed?", "+f.failed+" "+tr("failed"):""):tr("notRun");
    return '<button class="family-card" onclick="'+jsHandler(jsCall("selectNode",f.items[0]?.id||""))+'"><strong>'+esc(f.label)+'</strong><span class="family-count">'+f.total+'</span><span class="family-meta">'+esc(status)+'</span></button>';
  }).join("")||'<span class="sub">'+esc(tr("noValidationFamilies"))+'</span>';
  const preview=validations.slice(0,50).map(function(item){
    const anchor=validationAnchor(item,primaries);
    return '<button class="case-row" style="display:grid;grid-template-columns:minmax(0,1.2fr) minmax(0,1fr) auto;gap:8px;align-items:center;width:100%;text-align:left" onclick="'+jsHandler(jsCall("selectNode",item.id))+'"><strong style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+esc(item.displayName)+'</strong><span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#65708a;font-size:11px">'+esc(anchor?anchor.displayName:tr("anchor"))+'</span><span class="badge warn">'+esc(item.stateEffect||item.caseType||"")+'</span></button>';
  }).join("")||'<span class="sub">'+esc(tr("noValidationFamilies"))+'</span>';
  const more=validations.length>50?'<div class="sub">'+esc(tr("showingFirst"))+' 50 '+esc(tr("of"))+' '+validations.length+' '+esc(tr("cases"))+'</div>':"";
  nodes.innerHTML='<div class="detail" style="max-width:1220px;margin:0 auto;padding:22px"><div class="detail-actions"><button onclick="'+jsHandler("goBackToMap()")+'">'+esc(tr("backToMap"))+'</button><button onclick="'+jsHandler("navigateToState({view:\"map\",interfaceKey:\"\",showValidationNodes:false},true)")+'">'+esc(tr("showValidationGraph"))+'</button></div><h2>'+esc(tr("interfaceDetail"))+'</h2><div class="sub">'+esc(key)+' - '+validations.length+' '+esc(tr("validationCases"))+' / '+all.length+' '+esc(tr("totalCases"))+'</div><div class="section"><h3>'+esc(tr("interfaceOverview"))+'</h3><div class="kv"><div>'+esc(tr("interface"))+'</div><div>'+esc(key)+'</div><div>'+esc(tr("caseSection"))+'</div><div>'+esc(title||key)+'</div></div></div>'+selectedCaseSummary(current,primaries)+'<div style="display:grid;grid-template-columns:1fr 1fr 1.25fr;gap:12px" class="interface-grid"><div class="section" style="margin-top:0"><h3>'+esc(tr("primaryAnchors"))+'</h3><div class="list">'+primaryCards+'</div></div><div class="section" style="margin-top:0"><h3>'+esc(tr("testFamilies"))+'</h3><div class="family-grid">'+familyCards+'</div></div><div class="section" style="margin-top:0"><h3>'+esc(tr("casePreview"))+'</h3><div class="case-table" style="display:flex;flex-direction:column;gap:6px">'+preview+'</div>'+more+'</div></div></div>';
}`
}

func mapAtlasDetailJS() string {
	return `function renderHistory(){
  const box=document.getElementById("node-history");
  const chips=nodeHistory.slice(-4).map(function(id,idx){
    const n=nodeById.get(id);if(!n)return "";
    const fullIndex=nodeHistory.length-Math.min(nodeHistory.length,4)+idx;
    return '<button class="history-chip" onclick="navigateToHistoryIndex('+fullIndex+')" title="'+esc(n.displayName)+'">'+esc(n.displayName)+'</button>';
	  }).join("");
	  box.innerHTML='<button type="button" onclick="goBackNode()">'+esc(activeView==="interface"?tr("backToMap"):tr("back"))+'</button>'+chips+'<span class="sub">'+esc(activePathId||activeInterfaceKey||focusNodeId||tr("map"))+'</span>';
	}
	function caseFamilyKey(item){
	  const hay=[item.id,item.caseId,item.displayName,item.caseType,item.role,item.stateEffect,item.renderMode,(item.tags||[]).join(" "),item.patchJson,item.expectedJson].join(" ").toLowerCase();
	  if(hay.includes("sign")||hay.includes("auth")||hay.includes("public_key"))return "auth-signature";
	  if(hay.includes("required")||hay.includes("missing"))return "required-missing";
	  if(hay.includes("blank")||hay.includes("empty")||hay.includes("null"))return "blank-null";
	  if(hay.includes("too_long")||hay.includes("length"))return "length-limit";
	  if(hay.includes("not_number")||hay.includes("type")||hay.includes("text"))return "type-check";
	  if(hay.includes("repeat")||hay.includes("idempot"))return "idempotency";
	  if(hay.includes("race")||hay.includes("concurrent"))return "concurrency";
	  if(hay.includes("enum")||hay.includes("invalid")||hay.includes("illegal"))return "invalid-value";
	  return "contract-validation";
	}
	function caseFamilyLabel(key){
	  return ({
	    "auth-signature":tr("familyAuth"),
	    "required-missing":tr("familyRequired"),
	    "blank-null":tr("familyBlank"),
	    "length-limit":tr("familyLength"),
	    "type-check":tr("familyType"),
	    "invalid-value":tr("familyInvalid"),
	    "idempotency":tr("familyIdempotency"),
	    "concurrency":tr("familyConcurrency"),
	    "contract-validation":tr("familyContract")
	  })[key]||key;
	}
	function caseFamilySummaries(items){
	  const groups=new Map();
	  for(const item of items){
	    const key=caseFamilyKey(item);
	    if(!groups.has(key))groups.set(key,{key,label:caseFamilyLabel(key),total:0,run:0,failed:0,items:[]});
	    const group=groups.get(key),tasks=tasksForNode(item);
	    group.total++;group.items.push(item);group.run+=tasks.length?1:0;
	    if(tasks.some(function(t){return String(t.status||"").toLowerCase()==="failed"}))group.failed++;
	  }
	  return Array.from(groups.values()).sort(function(a,b){return b.failed-a.failed||b.total-a.total||a.label.localeCompare(b.label)});
	}
	function renderInterfaceCaseFamilies(n, reverseCases, sameInterface, key){
	  const families=caseFamilySummaries(reverseCases);
	  const cards=families.map(function(f){
	    const status=f.run?f.run+" "+tr("run")+(f.failed?", "+f.failed+" "+tr("failed"):""):tr("notRun");
	    return '<div class="family-card"><strong>'+esc(f.label)+'</strong><span class="family-count">'+f.total+'</span><span class="family-meta">'+esc(status)+'</span></div>';
	  }).join("");
	  const actions=key?'<div class="detail-actions"><button onclick="'+jsHandler(jsCall("showInterfaceCases",n.id))+'">'+esc(tr("openInterfaceDetail"))+'</button></div>':"";
	  return '<div class="section"><h3>'+esc(tr("testFamilies"))+'</h3><div class="sub">'+esc(key||"No interface key")+' - '+reverseCases.length+' '+esc(tr("validationCases"))+' / '+sameInterface.length+' '+esc(tr("totalCases"))+'</div><div class="family-grid">'+(cards||"<span class='sub'>"+esc(tr("noValidationFamilies"))+"</span>")+'</div>'+actions+'</div>';
	}
	function renderDetails(){
	  const box=document.getElementById("details");const n=nodeById.get(selectedNodeId);
  if(!n){box.innerHTML='<div class="empty">'+esc(tr("selectCaseNode"))+'</div>';return}
  const incoming=atlasData.edges.filter(function(e){return e.toNodeId===n.id&&nodeById.has(e.fromNodeId)});
  const outgoing=atlasData.edges.filter(function(e){return e.fromNodeId===n.id&&nodeById.has(e.toNodeId)});
  const ops=n.explanation?.operations||[];const template=n.requestTemplate;
  const runTasks=tasksForNode(n);
  const reverseCases=interfaceReverseCases(n),sameInterface=interfaceCases(n),key=interfaceKey(n);
  const jsonBlocks=[[tr("patch"),n.patchJson],[tr("expected"),n.expectedJson],[tr("requiredProperties"),n.requiredPropertyJson],[tr("providedProperties"),n.providedPropertyJson],[tr("requestTemplate"),template?.templateJson]].filter(function(x){return pretty(x[1])});
  let html='<div class="detail"><h2>'+esc(n.displayName)+'</h2><div class="sub">'+esc(n.id)+'</div>';
  html+='<div class="detail-actions"><button onclick="toggleFocusNode()">'+esc(tr("focus"))+'</button><button onclick="fitSelectedNode()">'+esc(tr("fit"))+'</button><button onclick="copyCurrentLink()">'+esc(tr("copied"))+'</button><button onclick="openPathFinder()">'+esc(tr("pathFinder"))+'</button></div>';
  html+=atlasData.warnings.filter(function(w){return w.includes(n.id)}).map(function(w){return '<div class="warning">'+esc(w)+'</div>'}).join("");
  if(atlasData.plan){html+='<div class="section"><h3>'+esc(tr("mapRunPlan"))+'</h3><div class="kv"><div>'+esc(tr("plan"))+'</div><div>'+esc(atlasData.plan.planId)+'</div><div>'+esc(tr("status"))+'</div><div>'+esc(atlasData.plan.status)+'</div><div>'+esc(tr("scope"))+'</div><div>'+esc(atlasData.plan.scope||"")+'</div></div></div>'}
	  html+='<div class="section"><h3>'+esc(tr("caseSection"))+'</h3><div class="kv"><div>'+esc(tr("caseID"))+'</div><div>'+esc(n.caseId)+'</div><div>'+esc(tr("type"))+'</div><div>'+esc(n.caseType||n.role)+'</div><div>'+esc(tr("stateEffect"))+'</div><div>'+esc(n.stateEffect)+'</div><div>'+esc(tr("interface"))+'</div><div>'+(key?'<button class="mini-link" onclick="'+jsHandler(jsCall("showInterfaceCases",n.id))+'">'+esc(key)+'</button>':"")+'</div><div>'+esc(tr("template"))+'</div><div>'+esc(n.requestTemplateId)+'</div><div>'+esc(tr("baseCase"))+'</div><div>'+esc(n.baseCaseId)+'</div><div>'+esc(tr("anchor"))+'</div><div>'+esc(n.anchorNodeId)+'</div></div></div>';
	  if(template){html+='<div class="section"><h3>'+esc(tr("request"))+'</h3><div class="kv"><div>'+esc(tr("method"))+'</div><div>'+esc(template.method)+'</div><div>'+esc(tr("path"))+'</div><div>'+esc(template.path)+'</div><div>'+esc(tr("template"))+'</div><div>'+esc(template.id)+'</div></div></div>'}
	  html+='<div class="section"><h3>'+esc(tr("runTasks"))+'</h3><div class="list">'+(runTasks.map(taskRow).join("")||"<span class='sub'>"+esc(tr("noRunTask"))+"</span>")+'</div></div>';
	  html+=renderInterfaceCaseFamilies(n,reverseCases,sameInterface,key);
  html+='<div class="section"><h3>'+esc(tr("workflowUsage"))+'</h3><div class="list">'+((n.paths||[]).map(function(p){return '<button class="link-row" onclick="'+jsHandler(jsCall("highlightPath",p.pathId))+'"><strong>'+esc(p.displayName||p.pathId)+'</strong><span>'+esc(p.workflowId)+' - step '+p.stepIndex+' - '+esc(p.stepId)+'</span></button>'}).join("")||"<span class='sub'>"+esc(tr("noWorkflowUsage"))+"</span>")+'</div></div>';
  html+='<div class="section"><h3>'+esc(tr("plannerOperations"))+'</h3><div class="list">'+(ops.map(function(o){return '<div class="link-row"><strong>'+esc(o.kind)+'</strong><span>'+esc(o.reason||"")+" "+esc(o.pathId||"")+" "+esc(o.untilNodeId||"")+" "+esc(o.caseId||"")+'</span></div>'}).join("")||"<span class='sub'>"+esc(tr("noOperations"))+"</span>")+'</div></div>';
  html+='<div class="section"><h3>'+esc(tr("connections"))+'</h3><div class="list">'+incoming.map(function(e){return edgeRow(e,true)}).join("")+outgoing.map(function(e){return edgeRow(e,false)}).join("")+'</div></div>';
  html+=jsonBlocks.map(function(x){return '<div class="section"><h3>'+esc(x[0])+'</h3><pre>'+esc(pretty(x[1]))+'</pre></div>'}).join("")+'</div>';
  box.innerHTML=html;
}
function tasksForNode(n){return (atlasData.plan?.tasks||[]).filter(function(t){return t.nodeId===n.id||t.caseId===n.caseId||t.caseId===n.id})}
function taskRow(t){const ref=t.apiCaseRunId||t.workflowRunId||t.evidenceRoot||"";return '<div class="link-row"><strong>'+esc(t.index)+'. '+esc(t.kind)+' - '+esc(t.status)+'</strong><span>'+esc(t.id)+' '+esc(t.reason||"")+'</span><span>'+esc(ref)+'</span></div>'}
function interfaceCaseRow(item){return '<button class="link-row" onclick="'+jsHandler(jsCall("selectNode",item.id))+'"><strong>'+esc(item.displayName)+'</strong><span>'+esc(item.caseId||item.id)+' - '+esc(item.caseType||item.role||"case")+' - '+esc(item.stateEffect||"")+'</span></button>'}
function edgeRow(e,incoming){const other=nodeById.get(incoming?e.fromNodeId:e.toNodeId);return '<button class="link-row" onclick="'+jsHandler(jsCall("selectNode",other.id))+'"><strong>'+(incoming?"in":"out")+' '+esc(e.kind)+'</strong><span>'+esc(other.displayName)+' - '+esc(e.pathId||e.materializationId||"")+'</span></button>'}`
}

func mapAtlasNavigationJS() string {
	return `function selectNode(id){if(!nodeById.has(id))return;if(selectedNodeId&&selectedNodeId!==id)nodeHistory.push(selectedNodeId);navigateToState({node:id},true)}
function navigateToHistoryIndex(index){const id=nodeHistory[index];nodeHistory=nodeHistory.slice(0,index);if(id)navigateToState({node:id},true)}
function goBackNode(){if(activeView==="interface"){goBackToMap();return}const id=nodeHistory.pop();if(id)navigateToState({node:id},true)}
	function goBackToMap(){navigateToState({view:"map",interfaceKey:""},true)}
	function highlightPath(id){navigateToState({path:id,interfaceKey:"",view:"map"},true)}
	function showInterfaceCases(id){const n=nodeById.get(id);if(!n)return;navigateToState({view:"interface",interfaceKey:interfaceKey(n),path:"",search:""},true)}
	function toggleFocusNode(){navigateToState({focus:focusNodeId?"":selectedNodeId},true)}
	function openValidationDetail(){
  const selected=nodeById.get(selectedNodeId),focused=nodeById.get(focusNodeId);
  const key=activeInterfaceKey||interfaceKey(selected)||interfaceKey(focused)||"";
  if(!key)return;
  navigateToState({view:"interface",interfaceKey:key,path:"",search:"",showValidationNodes:false},true);
}
function toggleValidationNodes(){openValidationDetail()}
	function fitSelectedNode(){
  if(activeView==="interface")return;
  const n=nodeById.get(selectedNodeId);const shell=document.querySelector(".graph-shell");if(!n||!shell)return;
  shell.scrollTo({left:Math.max(n.layout.x-120,0),top:Math.max(n.layout.y-90,0),behavior:"smooth"});
}
function copyCurrentLink(){updateHash(true);if(navigator.clipboard)navigator.clipboard.writeText(window.location.href).catch(function(){})}
function renderPathFinderOptions(){
  const opts='<option value="">'+esc(tr("selectCase"))+'</option>'+atlasData.nodes.map(function(n){return '<option value="'+esc(n.id)+'">'+esc(n.displayName)+' ('+esc(n.caseId||n.id)+')</option>'}).join("");
  document.getElementById("path-from").innerHTML=opts;document.getElementById("path-to").innerHTML=opts;
}
function openPathFinder(){renderPathFinderOptions();document.getElementById("path-from").value=selectedNodeId;document.getElementById("path-finder-result").innerHTML="";document.getElementById("path-finder").classList.remove("hidden")}
function closePathFinder(){document.getElementById("path-finder").classList.add("hidden")}
function findPath(){
  const from=document.getElementById("path-from").value,to=document.getElementById("path-to").value,result=document.getElementById("path-finder-result");
  if(!from||!to||from===to){result.innerHTML='<div class="warning">'+esc(tr("selectDifferent"))+'</div>';return}
  const adj=new Map();
  for(const e of atlasData.edges){if(!adj.has(e.fromNodeId))adj.set(e.fromNodeId,[]);if(!adj.has(e.toNodeId))adj.set(e.toNodeId,[]);adj.get(e.fromNodeId).push(e.toNodeId)}
  const q=[{id:from,path:[from]}],seen=new Set([from]);let found=null;
  while(q.length){const cur=q.shift();if(cur.id===to){found=cur.path;break}for(const next of adj.get(cur.id)||[]){if(!seen.has(next)){seen.add(next);q.push({id:next,path:cur.path.concat(next)})}}}
  if(!found){result.innerHTML='<div class="warning">'+esc(tr("noPathFound"))+'</div>';return}
  result.innerHTML='<div class="list">'+found.map(function(id,idx){const n=nodeById.get(id);return '<button class="link-row" onclick="'+jsHandler(jsCall("selectNode",id),"closePathFinder()")+'"><strong>'+String(idx+1)+'. '+esc(n?.displayName||id)+'</strong><span>'+esc(n?.caseId||id)+'</span></button>'}).join("")+'</div>';
}`
}

func mapAtlasBootJS() string {
	return `function renderAll(){if(activeView==="interface"){renderInterfaceView()}else{renderGraph()}renderHistory();renderDetails();syncText()}
	window.selectNode=selectNode;window.highlightPath=highlightPath;window.showInterfaceCases=showInterfaceCases;window.navigateToState=navigateToState;window.openPathFinder=openPathFinder;window.closePathFinder=closePathFinder;window.findPath=findPath;window.toggleFocusNode=toggleFocusNode;window.toggleValidationNodes=toggleValidationNodes;window.goBackNode=goBackNode;window.goBackToMap=goBackToMap;window.navigateToHistoryIndex=navigateToHistoryIndex;window.copyCurrentLink=copyCurrentLink;window.onpopstate=function(){hydrateFromHash();renderAll()};
renderWorkflowFilter();renderPathFinderOptions();hydrateFromHash();renderAll();updateHash(true);`
}
