'use strict';

// ── State ─────────────────────────────────────────────────────────────────────
let device = { cols: 5, rows: 3, keys: 15, model_name: '', reserved_keys: [] };
let layout = { pages: [] };
let packages = [];
let customTemplates = [];
let scripts = [];
let mode = 'folder';

// Multi-device state
let allDevices = [];       // DeviceGeometry[] from /api/devices
let currentDeviceID = '';  // currently selected device ID

let currentPageIdx = 0;
let selectedSlot = null;
let pendingButton = null;

// Monaco
let monacoReady = false;
let monacoEditor = null;
let monacoCurrentPath = null;  // for folder mode: relative file path
let monacoCustomId = null;     // for layout mode: custom template id
let activeConfigTab = 'form';

// ── Monaco init ───────────────────────────────────────────────────────────────
require.config({ paths: { vs: '/api/monaco/vs' } });
require(['vs/editor/editor.main'], function () {
	monacoReady = true;
	monacoEditor = monaco.editor.create(document.getElementById('monaco-tab'), {
		value: '-- select a script button to edit',
		language: 'lua',
		theme: 'vs-dark',
		automaticLayout: true,
		minimap: { enabled: false },
		fontSize: 13,
		scrollBeyondLastLine: false,
		wordWrap: 'on',
	});
	monacoEditor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => saveLuaFile());
});

// ── Boot ──────────────────────────────────────────────────────────────────────
async function boot() {
	try {
		const [dev, lay, pkgs, scr, modeRes, devList] = await Promise.all([
			fetch('/api/device').then(r => r.json()),
			fetch('/api/layout').then(r => r.json()),
			fetch('/api/packages').then(r => r.json()),
			fetch('/api/scripts').then(r => r.json()),
			fetch('/api/mode').then(r => r.json()),
			fetch('/api/devices').then(r => r.json()).catch(() => []),
		]);
		device = dev;
		layout = lay || { pages: [] };
		packages = pkgs || [];
		scripts = scr || [];
		mode = modeRes.style || 'folder';
		allDevices = devList || [];

		populateDeviceSelector();
		updateModeToggle();
		await refreshCustomTemplates();
		renderAll();
	} catch (e) {
		toast('Failed to load: ' + e, true);
	}
}

// ── Device selector ───────────────────────────────────────────────────────────
function populateDeviceSelector() {
	const sel = document.getElementById('hdr-device-select');
	sel.innerHTML = '';
	if (allDevices.length === 0) {
		const opt = document.createElement('option');
		opt.value = '';
		opt.textContent = 'No devices connected';
		sel.appendChild(opt);
		currentDeviceID = '';
		return;
	}
	// Add a "default (hardware)" option that uses the /api/device dimensions.
	const defOpt = document.createElement('option');
	defOpt.value = '';
	defOpt.textContent = 'Hardware device (' + device.cols + 'x' + device.rows + ')';
	sel.appendChild(defOpt);

	for (const d of allDevices) {
		const opt = document.createElement('option');
		opt.value = d.id;
		opt.textContent = d.name + ' — ' + d.cols + 'x' + d.rows + ' [' + d.source + ']';
		if (d.id === currentDeviceID) opt.selected = true;
		sel.appendChild(opt);
	}
}

async function onDeviceChange(deviceID) {
	currentDeviceID = deviceID;
	selectedSlot = null;
	pendingButton = null;

	if (deviceID !== '') {
		// Load the geometry for this device.
		const geom = allDevices.find(d => d.id === deviceID);
		if (geom) {
			device = {
				cols: geom.cols,
				rows: geom.rows,
				keys: geom.inputs ? geom.inputs.length : geom.cols * geom.rows,
				model_name: geom.name,
				reserved_keys: [],
				inputs: geom.inputs || [],
			};
		}
		// Load the layout assigned to this device.
		try {
			const lay = await fetch('/api/layout?device=' + encodeURIComponent(deviceID)).then(r => r.json());
			layout = lay || { pages: [] };
		} catch (e) {
			toast('Layout load failed: ' + e, true);
		}
	} else {
		// Revert to hardware device dimensions.
		try {
			const [dev, lay] = await Promise.all([
				fetch('/api/device').then(r => r.json()),
				fetch('/api/layout').then(r => r.json()),
			]);
			device = dev;
			layout = lay || { pages: [] };
		} catch (e) {
			toast('Device reload failed: ' + e, true);
		}
	}
	renderAll();
}

async function reloadFromDisk() {
	try {
		const layoutURL = currentDeviceID
			? '/api/layout?device=' + encodeURIComponent(currentDeviceID)
			: '/api/layout';
		const [lay, scr, devList] = await Promise.all([
			fetch(layoutURL).then(r => r.json()),
			fetch('/api/scripts').then(r => r.json()),
			fetch('/api/devices').then(r => r.json()).catch(() => []),
		]);
		layout = lay || { pages: [] };
		scripts = scr || [];
		allDevices = devList || [];
		selectedSlot = null; pendingButton = null;
		populateDeviceSelector();
		await refreshCustomTemplates();
		renderAll();
		toast('Reloaded from disk');
	} catch (e) {
		toast('Reload failed: ' + e, true);
	}
}

async function refreshCustomTemplates() {
	try {
		customTemplates = await fetch('/api/custom-template').then(r => r.json()) || [];
	} catch (_) {
		customTemplates = [];
	}
}

// ── Mode ──────────────────────────────────────────────────────────────────────
function updateModeToggle() {
	document.getElementById('btn-mode-folder').classList.toggle('active', mode === 'folder');
	document.getElementById('btn-mode-layout').classList.toggle('active', mode === 'layout');
}

async function setMode(newMode) {
	if (newMode === mode) return;
	try {
		const resp = await fetch('/api/mode', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ style: newMode }),
		});
		if (!resp.ok) throw new Error(await resp.text());
		mode = newMode;
		selectedSlot = null; pendingButton = null;
		updateModeToggle();
		renderAll();
		toast('Mode: ' + newMode);
	} catch (e) { toast('Mode change failed: ' + e, true); }
}

// ── Layout save/load ──────────────────────────────────────────────────────────
async function saveLayout() {
	hideSaveErrors();
	const saveURL = currentDeviceID
		? '/api/layout?device=' + encodeURIComponent(currentDeviceID)
		: '/api/layout';
	try {
		const resp = await fetch(saveURL, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(layout),
		});
		if (resp.status === 400) {
			const body = await resp.json();
			showSaveErrors(body.errors || ['Unknown validation error']);
			return;
		}
		if (!resp.ok) throw new Error(await resp.text());
		toast('Saved');
		renderPageTabs();
	} catch (e) { toast('Save failed: ' + e, true); }
}

function showSaveErrors(errs) {
	const el = document.getElementById('save-errors');
	el.textContent = errs.join('\n');
	el.style.display = 'block';
}
function hideSaveErrors() {
	document.getElementById('save-errors').style.display = 'none';
}

// ── Render coordination ───────────────────────────────────────────────────────
function renderAll() {
	renderPageTabs();
	renderGrid();
	renderConfigPanel();
	renderPackages();
}

// ── Page tabs ─────────────────────────────────────────────────────────────────
function renderPageTabs() {
	const container = document.getElementById('page-tabs');
	const addBtn = document.getElementById('tab-add');
	Array.from(container.querySelectorAll('.page-tab')).forEach(el => el.remove());

	(layout.pages || []).forEach((page, i) => {
		const tab = document.createElement('div');
		tab.className = 'page-tab' + (i === currentPageIdx ? ' active' : '');
		const hasHome = pageHasHome(page);
		const warnHTML = (!hasHome && mode === 'layout')
			? '<span class="tab-err" title="Missing SET/HOME button">!</span>'
			: '';
		const delHTML = '<span class="tab-del" title="Delete page">x</span>';
		tab.innerHTML = '<span class="tab-name">' + esc(page.name || 'Page ' + (i + 1)) + '</span>' + warnHTML + delHTML;
		tab.querySelector('.tab-del').addEventListener('click', function (e) {
			e.stopPropagation();
			deletePage(i);
		});
		tab.addEventListener('click', function () { currentPageIdx = i; selectedSlot = null; pendingButton = null; renderAll(); });
		container.insertBefore(tab, addBtn);
	});
}

function pageHasHome(page) {
	return (page.buttons || []).some(function (b) { return b.action === 'home'; });
}

// ── Grid ──────────────────────────────────────────────────────────────────────
function renderGrid() {
	const grid = document.getElementById('deck-grid');
	const pg = currentPage();
	const btnMap = {};
	if (pg) (pg.buttons || []).forEach(function (b) { btnMap[b.slot] = b; });

	// If the current device has explicit input geometry, use it for positioning.
	const inputs = device.inputs && device.inputs.length > 0 ? device.inputs : null;

	if (inputs) {
		renderGridFromInputs(grid, inputs, btnMap);
	} else {
		renderGridSimple(grid, btnMap);
	}
}

// renderGridSimple renders a uniform rows×cols grid (hardware device or no geometry).
function renderGridSimple(grid, btnMap) {
	grid.style.gridTemplateColumns = 'repeat(' + device.cols + ', 82px)';
	grid.style.gridTemplateRows = '';
	grid.innerHTML = '';

	for (let i = 0; i < device.keys; i++) {
		const btn = btnMap[i];
		const reserved = isReservedKey(i);
		const el = document.createElement('div');
		el.className = 'key-cell';
		if (reserved) el.classList.add('reserved');
		if (i === selectedSlot) el.classList.add('selected');

		if (reserved && mode === 'folder') {
			const role = folderReservedLabel(i);
			let inner = '<span class="slot-num">' + i + '</span>';
			inner += '<span class="lock-icon">&#x1F512;</span>';
			inner += '<span class="key-label reserved-label">' + esc(role) + '</span>';
			el.innerHTML = inner;
			grid.appendChild(el);
			continue;
		}

		decorateKeyCell(el, btn, i);
		grid.appendChild(el);
	}
}

// renderGridFromInputs renders an irregular grid using explicit x/y positions.
function renderGridFromInputs(grid, inputs, btnMap) {
	// Compute bounding box.
	let maxCol = 0, maxRow = 0;
	for (const inp of inputs) {
		if (inp.x > maxCol) maxCol = inp.x;
		if (inp.y > maxRow) maxRow = inp.y;
	}
	const cols = maxCol + 1;
	const rows = maxRow + 1;

	grid.style.gridTemplateColumns = 'repeat(' + cols + ', 82px)';
	grid.style.gridTemplateRows = 'repeat(' + rows + ', 82px)';
	grid.innerHTML = '';

	// Create a cell for each grid position.
	for (let row = 0; row < rows; row++) {
		for (let col = 0; col < cols; col++) {
			// Find which input occupies this position.
			const slotIdx = inputs.findIndex(function (inp) { return inp.x === col && inp.y === row; });
			const el = document.createElement('div');
			if (slotIdx === -1) {
				// Empty grid cell (no input here).
				el.className = 'key-cell key-cell-empty';
				grid.appendChild(el);
				continue;
			}
			el.className = 'key-cell';
			if (slotIdx === selectedSlot) el.classList.add('selected');
			const btn = btnMap[slotIdx];
			decorateKeyCell(el, btn, slotIdx);
			grid.appendChild(el);
		}
	}
}

// decorateKeyCell fills a key cell element with button data and click handler.
function decorateKeyCell(el, btn, slot) {
	if (btn) {
		const act = btn.action || 'script';
		if (act === 'home') el.classList.add('action-home');
		else if (act === 'settings') el.classList.add('action-settings');
		else if (act === 'page') el.classList.add('has-page');
		else if (act === 'back') el.classList.add('has-back');
		else if (btn.script || btn.template) el.classList.add('has-script');
	}

	let inner = '<span class="slot-num">' + slot + '</span>';
	if (btn) {
		if (btn.icon) {
			const iconURL = resolveIconURL(btn.icon);
			inner += '<img class="key-icon" src="' + esc(iconURL) + '" loading="lazy" onerror="this.style.display=\'none\'">';
		}
		inner += '<span class="key-label">' + esc(btn.label || '') + '</span>';
		if (btn.action && btn.action !== 'script') {
			inner += '<span class="badge">' + esc(btn.action) + '</span>';
		}
	}
	el.innerHTML = inner;

	(function (s) {
		el.addEventListener('click', function () { selectedSlot = s; initPending(); renderGrid(); renderConfigPanel(); });
	})(slot);
}

function folderReservedLabel(slot) {
	// key 0 = Back/SET, others in column 0 based on row
	if (slot === 0) return 'BACK / SET';
	const row = Math.floor(slot / device.cols);
	if (row === 1) return 'T1 / PG\u25BC';
	if (row === 2) return 'T2';
	return 'RSV';
}

function isReservedKey(slot) {
	if (mode === 'folder') {
		return (device.reserved_keys || []).indexOf(slot) !== -1;
	}
	// Layout mode: no reserved keys -- all slots are freely usable
	return false;
}

function resolveIconURL(icon) {
	if (!icon) return '';
	if (icon.startsWith('pkg://') || icon.startsWith('http://') || icon.startsWith('https://')) {
		return '/api/resource?ref=' + encodeURIComponent(icon);
	}
	return icon;
}

// ── Config panel ──────────────────────────────────────────────────────────────
function initPending() {
	const pg = currentPage();
	const existing = pg ? (pg.buttons || []).find(function (b) { return b.slot === selectedSlot; }) : null;
	pendingButton = existing
		? JSON.parse(JSON.stringify(existing))
		: { slot: selectedSlot, label: '', action: '', script: '', template: '', target_page: '', icon: '', metadata: {} };
}

function renderConfigPanel() {
	if (mode === 'folder') {
		renderConfigPanelFolder();
	} else {
		renderConfigPanelLayout();
	}
}

// ── Config panel: FOLDER mode ─────────────────────────────────────────────────
function renderConfigPanelFolder() {
	const formTab = document.getElementById('form-tab');
	const btnActions = document.getElementById('btn-actions');
	const configTabs = document.getElementById('config-tabs');
	const panelTitle = document.getElementById('config-panel-title');
	const monacoTab = document.getElementById('ctab-monaco');

	if (selectedSlot === null || pendingButton === null) {
		formTab.innerHTML = '<p style="color:var(--txt3);font-size:.82rem;padding:8px">Click a button to edit it</p>';
		formTab.style.display = '';
		document.getElementById('monaco-tab').style.display = 'none';
		btnActions.style.display = 'none';
		configTabs.style.display = 'none';
		panelTitle.textContent = 'Button config \u2014 Folder';
		return;
	}

	const v = pendingButton;
	const action = v.action || 'script';
	panelTitle.textContent = 'Slot ' + selectedSlot + ' \u2014 Folder';
	btnActions.style.display = 'flex';
	configTabs.style.display = 'flex';

	const hasScript = !!(v.script);
	monacoTab.style.display = hasScript ? '' : 'none';

	if (!hasScript && activeConfigTab === 'monaco') {
		activeConfigTab = 'form';
	}
	applySplit(activeConfigTab);

	// page options
	const pageOpts = (layout.pages || []).map(function (p) {
		return '<option value="' + esc(p.name) + '"' + (v.target_page === p.name ? ' selected' : '') + '>' + esc(p.name) + '</option>';
	}).join('');

	// script datalist
	const scriptOpts = scripts.map(function (s) { return '<option value="' + esc(s) + '">'; }).join('');

	// template datalist
	const tmplOpts = packages.flatMap(function (p) {
		return (p.templates || []).map(function (t) {
			return '<option value="' + esc(t.key) + '" label="' + esc(t.label) + '">';
		});
	}).join('');

	// metadata schema for current template
	let metaSchema = [];
	if (v.template) {
		for (const pkg of packages) {
			const tmpl = (pkg.templates || []).find(function (t) { return t.key === v.template; });
			if (tmpl && tmpl.metadata_schema) { metaSchema = tmpl.metadata_schema; break; }
		}
	}

	const metaFields = metaSchema.map(function (f) {
		const val = (v.metadata || {})[f.key] || f.default || '';
		return '<div class="form-row"><label>' + esc(f.label || f.key) +
			' <span style="color:var(--blue-l);font-size:.65rem">[' + esc(f.key) + ']</span></label>' +
			'<input type="text" data-meta-key="' + esc(f.key) + '" value="' + esc(val) + '"' +
			' placeholder="' + esc(f.description || '') + '" oninput="updateMeta(this)"></div>';
	}).join('');

	let scriptSection = '';
	if (action === 'script' || action === '') {
		const editBtn = v.script
			? '<button class="btn-ghost" style="white-space:nowrap;padding:4px 8px;flex-shrink:0" onclick="openInMonaco()">Edit</button>'
			: '';
		scriptSection = '<div class="form-row">' +
			'<label>Script path</label>' +
			'<div style="display:flex;gap:4px">' +
			'<input id="f-script" type="text" list="script-list" value="' + esc(v.script || '') + '" oninput="pendingButton.script=this.value;renderConfigPanel()">' +
			editBtn + '</div>' +
			'<datalist id="script-list">' + scriptOpts + '</datalist>' +
			'</div>' +
			'<div class="form-row">' +
			'<label>Template key <span style="color:var(--txt3);font-size:.65rem">(pkg://...)</span></label>' +
			'<input id="f-template" type="text" list="tmpl-list" value="' + esc(v.template || '') + '" oninput="pendingButton.template=this.value;renderConfigPanel()" placeholder="pkg://riverdeck/home">' +
			'<datalist id="tmpl-list">' + tmplOpts + '</datalist>' +
			'</div>' +
			metaFields;
	}

	let pageSection = '';
	if (action === 'page') {
		pageSection = '<div class="form-row">' +
			'<label>Target page</label>' +
			'<select id="f-target" onchange="pendingButton.target_page=this.value">' +
			'<option value="">-- choose --</option>' +
			pageOpts + '</select></div>';
	}

	formTab.innerHTML =
		'<div class="form-row">' +
		'<label>Label</label>' +
		'<input id="f-label" type="text" value="' + esc(v.label || '') + '" oninput="pendingButton.label=this.value;liveUpdateGridKey()">' +
		'</div>' +
		'<div class="form-row">' +
		'<label>Action</label>' +
		'<select id="f-action" onchange="pendingButton.action=this.value;renderConfigPanel()">' +
		'<option value="script"' + (action === 'script' || action === '' ? ' selected' : '') + '>Script (Lua)</option>' +
		'<option value="page"' + (action === 'page' ? ' selected' : '') + '>Go to page</option>' +
		'<option value="back"' + (action === 'back' ? ' selected' : '') + '>Back</option>' +
		'<option value="home"' + (action === 'home' ? ' selected' : '') + '>SET / Home</option>' +
		'<option value="settings"' + (action === 'settings' ? ' selected' : '') + '>Settings</option>' +
		'</select></div>' +
		scriptSection + pageSection +
		'<div class="form-row">' +
		'<label>Icon <span style="color:var(--txt3);font-size:.65rem">(pkg://... or path)</span></label>' +
		'<input id="f-icon" type="text" value="' + esc(v.icon || '') + '" oninput="pendingButton.icon=this.value;liveUpdateGridKey()" placeholder="pkg://riverdeck/icons/home.png">' +
		'</div>';
}

// ── Config panel: LAYOUT mode ─────────────────────────────────────────────────
function renderConfigPanelLayout() {
	const formTab = document.getElementById('form-tab');
	const btnActions = document.getElementById('btn-actions');
	const configTabs = document.getElementById('config-tabs');
	const panelTitle = document.getElementById('config-panel-title');
	const monacoTab = document.getElementById('ctab-monaco');

	if (selectedSlot === null || pendingButton === null) {
		formTab.innerHTML = '<p style="color:var(--txt3);font-size:.82rem;padding:8px">Click a button to edit it</p>';
		formTab.style.display = '';
		document.getElementById('monaco-tab').style.display = 'none';
		btnActions.style.display = 'none';
		configTabs.style.display = 'none';
		panelTitle.textContent = 'Button config \u2014 Layout';
		return;
	}

	const v = pendingButton;
	const action = v.action || 'script';
	panelTitle.textContent = 'Slot ' + selectedSlot + ' \u2014 Layout';
	btnActions.style.display = 'flex';
	configTabs.style.display = 'flex';

	// In layout mode, Monaco is available for custom templates
	const isCustom = !!(v.template && v.template.startsWith('pkg://_custom/'));
	const customId = isCustom ? v.template.replace('pkg://_custom/', '') : null;
	monacoTab.style.display = isCustom ? '' : 'none';

	if (!isCustom && activeConfigTab === 'monaco') {
		activeConfigTab = 'form';
	}
	applySplit(activeConfigTab);

	// page options
	const pageOpts = (layout.pages || []).map(function (p) {
		return '<option value="' + esc(p.name) + '"' + (v.target_page === p.name ? ' selected' : '') + '>' + esc(p.name) + '</option>';
	}).join('');

	// Build full template list (packages + custom)
	const allTemplateOpts = [];
	for (const pkg of packages) {
		for (const t of (pkg.templates || [])) {
			allTemplateOpts.push({ key: t.key, label: t.label, group: pkg.name || pkg.id });
		}
	}
	for (const ct of customTemplates) {
		allTemplateOpts.push({ key: 'pkg://_custom/' + ct.id, label: ct.label, group: 'Custom' });
	}

	// metadata schema for current template
	let metaSchema = [];
	if (v.template) {
		// Check packages first
		for (const pkg of packages) {
			const tmpl = (pkg.templates || []).find(function (t) { return t.key === v.template; });
			if (tmpl && tmpl.metadata_schema) { metaSchema = tmpl.metadata_schema; break; }
		}
	}

	const metaFields = metaSchema.map(function (f) {
		const val = (v.metadata || {})[f.key] || f.default || '';
		return '<div class="form-row"><label>' + esc(f.label || f.key) +
			' <span style="color:var(--blue-l);font-size:.65rem">[' + esc(f.key) + ']</span></label>' +
			'<input type="text" data-meta-key="' + esc(f.key) + '" value="' + esc(val) + '"' +
			' placeholder="' + esc(f.description || '') + '" oninput="updateMeta(this)"></div>';
	}).join('');

	let scriptSection = '';
	if (action === 'script' || action === '') {
		// Template selector  (select dropdown instead of raw text input)
		const tmplSelectOpts = allTemplateOpts.map(function (t) {
			return '<option value="' + esc(t.key) + '"' +
				(v.template === t.key ? ' selected' : '') + '>' +
				esc(t.label) + ' (' + esc(t.group) + ')</option>';
		}).join('');

		const editBtn = isCustom
			? '<button class="btn-ghost" style="white-space:nowrap;padding:4px 8px;flex-shrink:0" onclick="openCustomInMonaco(\'' + esc(customId) + '\')">Edit Lua</button>'
			: '';

		scriptSection = '<div class="form-row">' +
			'<label>Template</label>' +
			'<div style="display:flex;gap:4px">' +
			'<select id="f-template" onchange="pendingButton.template=this.value;renderConfigPanel()" style="flex:1">' +
			'<option value="">-- none --</option>' +
			tmplSelectOpts +
			'</select>' +
			editBtn + '</div></div>' +
			metaFields;
	}

	let pageSection = '';
	if (action === 'page') {
		pageSection = '<div class="form-row">' +
			'<label>Target page</label>' +
			'<select id="f-target" onchange="pendingButton.target_page=this.value">' +
			'<option value="">-- choose --</option>' +
			pageOpts + '</select></div>';
	}

	formTab.innerHTML =
		'<div class="form-row">' +
		'<label>Label</label>' +
		'<input id="f-label" type="text" value="' + esc(v.label || '') + '" oninput="pendingButton.label=this.value;liveUpdateGridKey()">' +
		'</div>' +
		'<div class="form-row">' +
		'<label>Action</label>' +
		'<select id="f-action" onchange="pendingButton.action=this.value;renderConfigPanel()">' +
		'<option value="script"' + (action === 'script' || action === '' ? ' selected' : '') + '>Script (Lua)</option>' +
		'<option value="page"' + (action === 'page' ? ' selected' : '') + '>Go to page</option>' +
		'<option value="back"' + (action === 'back' ? ' selected' : '') + '>Back</option>' +
		'<option value="home"' + (action === 'home' ? ' selected' : '') + '>SET / Home</option>' +
		'<option value="settings"' + (action === 'settings' ? ' selected' : '') + '>Settings</option>' +
		'</select></div>' +
		scriptSection + pageSection +
		'<div class="form-row">' +
		'<label>Icon <span style="color:var(--txt3);font-size:.65rem">(pkg://... or path)</span></label>' +
		'<input id="f-icon" type="text" value="' + esc(v.icon || '') + '" oninput="pendingButton.icon=this.value;liveUpdateGridKey()" placeholder="pkg://riverdeck/icons/home.png">' +
		'</div>';
}

// ── Shared config panel helpers ───────────────────────────────────────────────
function applySplit(tab) {
	activeConfigTab = tab;
	document.getElementById('ctab-form').classList.toggle('active', tab === 'form');
	const ctabMonaco = document.getElementById('ctab-monaco');
	if (ctabMonaco) ctabMonaco.classList.toggle('active', tab === 'monaco');
	document.getElementById('form-tab').style.display = tab === 'form' ? '' : 'none';
	document.getElementById('monaco-tab').style.display = tab === 'monaco' ? '' : 'none';
}

function switchConfigTab(tab) {
	if (tab === 'monaco') {
		if (mode === 'folder') {
			openInMonacoNoSwitch().then(function () { applySplit('monaco'); });
		} else {
			// layout mode -- open custom template Lua
			const tmpl = pendingButton ? pendingButton.template : '';
			if (tmpl && tmpl.startsWith('pkg://_custom/')) {
				const id = tmpl.replace('pkg://_custom/', '');
				openCustomInMonacoNoSwitch(id).then(function () { applySplit('monaco'); });
			}
		}
	} else {
		applySplit('form');
	}
}

function updateMeta(input) {
	if (!pendingButton) return;
	if (!pendingButton.metadata) pendingButton.metadata = {};
	pendingButton.metadata[input.dataset.metaKey] = input.value;
}

function liveUpdateGridKey() {
	if (selectedSlot === null) return;
	const grid = document.getElementById('deck-grid');
	// Find the cell with this slot (may not be at index selectedSlot if using geometry grid).
	const cells = grid.querySelectorAll('.key-cell:not(.key-cell-empty)');
	let target = null;
	for (let i = 0; i < cells.length; i++) {
		const slotEl = cells[i].querySelector('.slot-num');
		if (slotEl && parseInt(slotEl.textContent) === selectedSlot) {
			target = cells[i];
			break;
		}
	}
	if (!target) return;
	const lbl = target.querySelector('.key-label');
	if (lbl && pendingButton) lbl.textContent = pendingButton.label || '';
}

// ── Monaco: folder mode (filesystem scripts) ─────────────────────────────────
async function openInMonacoNoSwitch() {
	if (!monacoReady || !pendingButton || !pendingButton.script) return;
	const path = pendingButton.script;
	if (path === monacoCurrentPath && monacoCustomId === null) return;
	try {
		const resp = await fetch('/api/file?path=' + encodeURIComponent(path));
		if (resp.status === 404) {
			monacoEditor.setValue('-- new file: ' + path + '\n');
		} else if (!resp.ok) {
			throw new Error(await resp.text());
		} else {
			monacoEditor.setValue(await resp.text());
		}
		monacoCurrentPath = path;
		monacoCustomId = null;
		monacoEditor.setScrollPosition({ scrollTop: 0 });
	} catch (e) {
		toast('Could not open file: ' + e, true);
	}
}

function openInMonaco() {
	openInMonacoNoSwitch().then(function () { applySplit('monaco'); });
}

// ── Monaco: layout mode (custom templates) ────────────────────────────────────
async function openCustomInMonacoNoSwitch(id) {
	if (!monacoReady || !id) return;
	if (monacoCustomId === id) return;
	try {
		const resp = await fetch('/api/custom-template/file?id=' + encodeURIComponent(id));
		if (resp.status === 404) {
			monacoEditor.setValue('-- custom template: ' + id + '\n');
		} else if (!resp.ok) {
			throw new Error(await resp.text());
		} else {
			monacoEditor.setValue(await resp.text());
		}
		monacoCustomId = id;
		monacoCurrentPath = null;
		monacoEditor.setScrollPosition({ scrollTop: 0 });
	} catch (e) {
		toast('Could not open custom template: ' + e, true);
	}
}

function openCustomInMonaco(id) {
	openCustomInMonacoNoSwitch(id).then(function () { applySplit('monaco'); });
}

// ── Monaco: save (auto-detects folder vs custom) ─────────────────────────────
async function saveLuaFile() {
	if (!monacoReady) return;

	if (monacoCustomId) {
		// Layout mode: save to custom template
		try {
			const resp = await fetch('/api/custom-template/file?id=' + encodeURIComponent(monacoCustomId), {
				method: 'POST',
				headers: { 'Content-Type': 'text/plain' },
				body: monacoEditor.getValue(),
			});
			if (!resp.ok) throw new Error(await resp.text());
			toast('Custom template saved');
		} catch (e) {
			toast('Save failed: ' + e, true);
		}
	} else if (monacoCurrentPath) {
		// Folder mode: save to filesystem
		try {
			const resp = await fetch('/api/file?path=' + encodeURIComponent(monacoCurrentPath), {
				method: 'POST',
				headers: { 'Content-Type': 'text/plain' },
				body: monacoEditor.getValue(),
			});
			if (!resp.ok) throw new Error(await resp.text());
			toast('Lua saved');
		} catch (e) {
			toast('Lua save failed: ' + e, true);
		}
	}
}

// ── Button mutations ──────────────────────────────────────────────────────────
function applyButton() {
	if (selectedSlot === null || pendingButton === null) return;
	const pg = currentPage();
	if (!pg) return;
	pg.buttons = pg.buttons || [];
	let btn = pg.buttons.find(function (b) { return b.slot === selectedSlot; });
	if (!btn) { btn = { slot: selectedSlot }; pg.buttons.push(btn); }

	btn.label = (document.getElementById('f-label')?.value ?? pendingButton.label) || '';
	btn.action = (document.getElementById('f-action')?.value ?? pendingButton.action) || '';
	btn.icon = (document.getElementById('f-icon')?.value ?? pendingButton.icon) || '';
	btn.metadata = pendingButton.metadata || {};

	if (mode === 'folder') {
		btn.script = (document.getElementById('f-script')?.value ?? pendingButton.script) || '';
		btn.template = (document.getElementById('f-template')?.value ?? pendingButton.template) || '';
	} else {
		// Layout mode: template from select, no raw script path
		btn.template = (document.getElementById('f-template')?.value ?? pendingButton.template) || '';
		delete btn.script;
	}

	btn.target_page = (document.getElementById('f-target')?.value ?? pendingButton.target_page) || '';

	// strip empties
	['label', 'script', 'template', 'target_page', 'icon'].forEach(function (k) { if (!btn[k]) delete btn[k]; });
	if (!btn.action || btn.action === 'script') delete btn.action;
	if (!btn.metadata || Object.keys(btn.metadata).length === 0) delete btn.metadata;

	selectedSlot = null; pendingButton = null;
	renderAll();
	toast('Applied');
}

function deleteButton() {
	const pg = currentPage();
	if (!pg) return;
	pg.buttons = (pg.buttons || []).filter(function (b) { return b.slot !== selectedSlot; });
	selectedSlot = null; pendingButton = null;
	renderAll();
}

// ── Page management ───────────────────────────────────────────────────────────
function currentPage() {
	if (!layout.pages || layout.pages.length === 0) return null;
	if (currentPageIdx >= layout.pages.length) currentPageIdx = 0;
	return layout.pages[currentPageIdx];
}

function addPage() {
	const name = prompt('New page name:', 'Page ' + (layout.pages.length + 1));
	if (!name) return;
	layout.pages.push({ name: name, buttons: [] });
	currentPageIdx = layout.pages.length - 1;
	selectedSlot = null; pendingButton = null;
	renderAll();
}

function deletePage(i) {
	if (layout.pages.length === 1) { toast('Cannot delete the last page'); return; }
	if (!confirm('Delete page "' + layout.pages[i].name + '"?')) return;
	layout.pages.splice(i, 1);
	if (currentPageIdx >= layout.pages.length) currentPageIdx = layout.pages.length - 1;
	selectedSlot = null; pendingButton = null;
	renderAll();
}

// ── Package browser ───────────────────────────────────────────────────────────
function renderPackages() {
	const container = document.getElementById('pkg-body');
	const query = (document.getElementById('pkg-search')?.value || '').toLowerCase().trim();
	container.innerHTML = '';

	// In layout mode, show "Create Custom" button at top
	if (mode === 'layout') {
		const createRow = document.createElement('div');
		createRow.className = 'pkg-actions';
		createRow.innerHTML =
			'<button class="btn-primary pkg-action-btn" onclick="createCustomTemplate()">+ Create Custom Template</button>';
		container.appendChild(createRow);
	}

	let anyShown = false;

	// Show custom templates first in layout mode
	if (mode === 'layout' && customTemplates.length > 0) {
		const filtered = customTemplates.filter(function (ct) {
			if (!query) return true;
			return (ct.label || '').toLowerCase().indexOf(query) !== -1 ||
				(ct.id || '').toLowerCase().indexOf(query) !== -1 ||
				(ct.description || '').toLowerCase().indexOf(query) !== -1;
		});
		if (filtered.length > 0) {
			anyShown = true;
			const det = document.createElement('details');
			det.className = 'pkg-group';
			det.open = true;
			det.innerHTML = '<summary>Custom Templates <span style="color:var(--txt3);font-size:.7rem">_custom</span></summary>';

			const BLANK_SVG = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Crect width='24' height='24' fill='%23333'/%3E%3C/svg%3E";

			for (const ct of filtered) {
				const row = document.createElement('div');
				row.className = 'pkg-tmpl';
				row.title = ct.description || ct.label || '';
				row.innerHTML =
					'<img src="' + BLANK_SVG + '" alt="" loading="lazy">' +
					'<div class="pkg-tmpl-info">' +
					'<div class="tmpl-label">' + esc(ct.label) + '</div>' +
					'<div class="tmpl-desc">' + esc(ct.description || ct.id || '') + '</div>' +
					'</div>' +
					'<button class="btn-danger" style="padding:2px 6px;font-size:.6rem;flex-shrink:0" onclick="event.stopPropagation();deleteCustomTemplate(\'' + esc(ct.id) + '\')">\u2715</button>' +
					'<span class="pkg-badge custom-badge">CUSTOM</span>';

				(function (cid, clabel) {
					row.addEventListener('click', function () {
						applyCustomTemplate(cid, clabel);
					});
				})(ct.id, ct.label);
				det.appendChild(row);
			}
			container.appendChild(det);
		}
	}

	for (const pkg of packages) {
		const templates = (pkg.templates || []).filter(function (t) {
			if (!query) return true;
			return (t.label || '').toLowerCase().indexOf(query) !== -1 ||
				(t.description || '').toLowerCase().indexOf(query) !== -1 ||
				(pkg.id || '').toLowerCase().indexOf(query) !== -1;
		});
		if (templates.length === 0) continue;
		anyShown = true;

		const det = document.createElement('details');
		det.className = 'pkg-group';
		if (!query) det.open = true;
		det.innerHTML = '<summary>' + esc(pkg.name || pkg.id) +
			' <span style="color:var(--txt3);font-size:.7rem">' + esc(pkg.id) + '</span></summary>';

		const BLANK_SVG = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Crect width='24' height='24' fill='%23333'/%3E%3C/svg%3E";

		for (const tmpl of templates) {
			const row = document.createElement('div');
			row.className = 'pkg-tmpl';
			row.title = tmpl.description || tmpl.label || '';
			const iconSrc = tmpl.icon_url ? esc(tmpl.icon_url) : BLANK_SVG;
			let badgeHTML = '<span class="pkg-badge">PKG</span>';
			// In layout mode, add "Duplicate & Edit" button
			if (mode === 'layout') {
				badgeHTML = '<button class="btn-ghost" style="padding:2px 6px;font-size:.58rem;flex-shrink:0;white-space:nowrap" ' +
					'onclick="event.stopPropagation();duplicateTemplate(\'' + esc(pkg.id) + '\',\'' + esc(tmpl.key) + '\',\'' + esc(tmpl.label) + '\')">' +
					'Dup&amp;Edit</button>' + badgeHTML;
			}
			row.innerHTML =
				'<img src="' + iconSrc + '" alt="" loading="lazy" onerror="this.src=\'' + BLANK_SVG + '\'">' +
				'<div class="pkg-tmpl-info">' +
				'<div class="tmpl-label">' + esc(tmpl.label) + '</div>' +
				'<div class="tmpl-desc">' + esc(tmpl.description || tmpl.key || '') + '</div>' +
				'</div>' +
				badgeHTML;

			(function (p, t) { row.addEventListener('click', function () { applyTemplate(p, t); }); })(pkg, tmpl);
			det.appendChild(row);
		}
		container.appendChild(det);
	}

	if (!anyShown && mode !== 'layout') {
		container.innerHTML = '<p class="pkg-no-results">No templates found</p>';
	}
}

async function applyTemplate(pkg, tmpl) {
	if (selectedSlot === null) {
		toast('Select a slot first', true);
		return;
	}
	if (isReservedKey(selectedSlot)) {
		toast('That slot is reserved', true);
		return;
	}

	if (mode === 'folder') {
		try {
			const resp = await fetch('/api/folder/assign', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ template_key: tmpl.key, slot: selectedSlot }),
			});
			if (!resp.ok) throw new Error(await resp.text());
			const result = await resp.json();
			const scr = await fetch('/api/scripts').then(function (r) { return r.json(); });
			scripts = scr || [];
			initPending();
			pendingButton.script = result.path || '';
			pendingButton.template = tmpl.key;
			pendingButton.label = tmpl.label || '';
			if (tmpl.icon_url) pendingButton.icon = 'pkg://' + pkg.id + '/' + (tmpl.icon || '');
			renderConfigPanel();
			toast('Template copied to filesystem \u2014 click Apply');
		} catch (e) { toast('Assign failed: ' + e, true); }
	} else {
		// Layout mode: create an instance reference (not a copy)
		initPending();
		pendingButton.template = tmpl.key;
		pendingButton.label = tmpl.label || '';
		if (tmpl.icon_url) pendingButton.icon = 'pkg://' + pkg.id + '/' + (tmpl.icon || '');
		pendingButton.metadata = {};
		for (const f of (tmpl.metadata_schema || [])) {
			if (f.default) pendingButton.metadata[f.key] = f.default;
		}
		renderConfigPanel();
		toast('Template instance set \u2014 click Apply');
	}
}

// ── Custom templates (layout mode) ────────────────────────────────────────────
async function createCustomTemplate() {
	const id = prompt('Template ID (letters, digits, underscore, dash):');
	if (!id) return;
	const label = prompt('Display label:', id);
	if (!label) return;
	try {
		const resp = await fetch('/api/custom-template', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ id: id, label: label }),
		});
		if (!resp.ok) throw new Error(await resp.text());
		await refreshCustomTemplates();
		// Also re-scan packages so _custom shows up
		packages = await fetch('/api/packages').then(r => r.json()) || [];
		renderPackages();
		toast('Custom template created: ' + label);
	} catch (e) {
		toast('Create failed: ' + e, true);
	}
}

async function duplicateTemplate(pkgId, templateKey, templateLabel) {
	const id = prompt('ID for the duplicate (letters, digits, _, -):', templateLabel.toLowerCase().replace(/[^a-z0-9_-]/g, '_'));
	if (!id) return;
	const label = prompt('Label:', templateLabel + ' (custom)');
	if (!label) return;
	try {
		const resp = await fetch('/api/custom-template', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ id: id, label: label, source_script: templateKey }),
		});
		if (!resp.ok) throw new Error(await resp.text());
		await refreshCustomTemplates();
		packages = await fetch('/api/packages').then(r => r.json()) || [];
		renderPackages();
		toast('Duplicated as custom: ' + label);
	} catch (e) {
		toast('Duplicate failed: ' + e, true);
	}
}

async function deleteCustomTemplate(id) {
	if (!confirm('Delete custom template "' + id + '"?')) return;
	try {
		const resp = await fetch('/api/custom-template?id=' + encodeURIComponent(id), { method: 'DELETE' });
		if (!resp.ok) throw new Error(await resp.text());
		await refreshCustomTemplates();
		packages = await fetch('/api/packages').then(r => r.json()) || [];
		renderPackages();
		toast('Deleted');
	} catch (e) {
		toast('Delete failed: ' + e, true);
	}
}

function applyCustomTemplate(id, label) {
	if (selectedSlot === null) {
		toast('Select a slot first', true);
		return;
	}
	initPending();
	pendingButton.template = 'pkg://_custom/' + id;
	pendingButton.label = label || id;
	pendingButton.metadata = {};
	renderConfigPanel();
	toast('Custom template set \u2014 click Apply');
}

// ── Utilities ─────────────────────────────────────────────────────────────────
function esc(s) {
	return String(s || '')
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}

let toastTimer;
function toast(msg, err) {
	const el = document.getElementById('toast');
	el.textContent = msg;
	el.className = err ? 'show err' : 'show';
	clearTimeout(toastTimer);
	toastTimer = setTimeout(function () { el.className = ''; }, err ? 4000 : 2000);
}

// ── App settings ──────────────────────────────────────────────────────────────
let appCfgCache = {};

function showSettings() {
	loadAppConfig().then(function () {
		document.getElementById('settings-overlay').classList.add('open');
	});
}

function hideSettings() {
	document.getElementById('settings-overlay').classList.remove('open');
}

async function loadAppConfig() {
	try {
		const resp = await fetch('/api/app-config');
		if (!resp.ok) throw new Error('HTTP ' + resp.status);
		appCfgCache = await resp.json();
		const app = appCfgCache.application || {};
		const ui = appCfgCache.ui || {};
		const net = appCfgCache.network || {};
		document.getElementById('scfg-brightness').value = (app.brightness !== undefined) ? app.brightness : 75;
		document.getElementById('scfg-fps').value = (app.passive_fps !== undefined) ? app.passive_fps : 30;
		document.getElementById('scfg-timeout').value = (app.timeout !== undefined) ? app.timeout : 0;
		document.getElementById('scfg-debug').value = (app.debug === true) ? 'true' : 'false';
		document.getElementById('scfg-navstyle').value = ui.navigation_style || 'folder';
		document.getElementById('scfg-hidden').value = (ui.show_hidden_files === true) ? 'true' : 'false';
		document.getElementById('scfg-httptmo').value = (net.http_timeout !== undefined) ? net.http_timeout : 10;
		document.getElementById('scfg-ssl').value = (net.verify_ssl === false) ? 'false' : 'true';
	} catch (e) {
		toast('Failed to load config: ' + e, true);
	}
}

async function saveSettings() {
	const patch = {
		application: {
			brightness: parseInt(document.getElementById('scfg-brightness').value) || 75,
			passive_fps: parseInt(document.getElementById('scfg-fps').value) || 30,
			timeout: parseInt(document.getElementById('scfg-timeout').value) || 0,
			debug: document.getElementById('scfg-debug').value === 'true',
		},
		ui: {
			navigation_style: document.getElementById('scfg-navstyle').value,
			show_hidden_files: document.getElementById('scfg-hidden').value === 'true',
		},
		network: {
			http_timeout: parseInt(document.getElementById('scfg-httptmo').value) || 10,
			verify_ssl: document.getElementById('scfg-ssl').value === 'true',
		},
	};
	try {
		const resp = await fetch('/api/app-config', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(patch),
		});
		if (!resp.ok) throw new Error(await resp.text());
		hideSettings();
		toast('Config saved \u2014 restart Riverdeck to apply changes');
	} catch (e) {
		toast('Save failed: ' + e, true);
	}
}

// ── Start ─────────────────────────────────────────────────────────────────────
boot();
