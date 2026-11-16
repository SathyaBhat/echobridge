// Compose page. Depends on common.js (API_BASE, esc, toast, History, ...).

let mediaFiles = [];
let accounts = [];
let selectedIds = null; // null = not yet decided (default: all selected)

const $ = (id) => document.getElementById(id);

document.addEventListener('DOMContentLoaded', async () => {
    restoreDraft();
    setupMedia();
    setupDraftAndCounter();
    setupFormSubmit();
    await loadAccounts();
    updateComposeState();
});

// ---------- Accounts ----------

async function loadAccounts() {
    const container = $('accounts-list');
    try {
        const response = await fetch(`${API_BASE}/accounts`);
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        accounts = (await response.json()) || [];
    } catch (error) {
        container.innerHTML = '<p class="hint">Failed to load accounts.</p>';
        console.error('Failed to load accounts:', error);
        return;
    }

    if (accounts.length === 0) {
        container.innerHTML = '<p class="hint">No accounts connected. <a href="profile.html">Add an account</a></p>';
        return;
    }

    const known = new Set(accounts.map((a) => a.id));
    if (selectedIds) selectedIds = selectedIds.filter((id) => known.has(id));
    const isSelected = (id) => (selectedIds === null ? true : selectedIds.includes(id));

    container.innerHTML = accounts.map((a) => {
        const p = platformInfo(a.provider);
        const sub = a.instance_url ? ` · ${esc(a.instance_url)}` : '';
        return `
            <label class="chip">
                <input type="checkbox" name="account_ids" value="${esc(a.id)}" data-provider="${esc(a.provider)}" ${isSelected(a.id) ? 'checked' : ''}>
                <span class="avatar avatar--${esc(a.provider)}">${p.icon}</span>
                <span>${esc(a.display_name)}<span class="chip-limit">${sub}${p.limit ? ` · ${p.limit}` : ''}</span></span>
            </label>`;
    }).join('');

    container.addEventListener('change', (e) => {
        if (e.target.matches('input[name="account_ids"]')) {
            selectedIds = checkedIds();
            saveDraft();
            updateComposeState();
        }
    });
}

function checkedBoxes() {
    return Array.from(document.querySelectorAll('input[name="account_ids"]:checked'));
}

function checkedIds() {
    return checkedBoxes().map((cb) => cb.value);
}

// ---------- Counter, preview, draft ----------

function selectedProviders() {
    return [...new Set(checkedBoxes().map((cb) => cb.dataset.provider).filter((p) => PLATFORMS[p]))];
}

function updateComposeState() {
    const text = $('content').value;
    const providers = selectedProviders();
    const count = checkedBoxes().length;

    $('selected-summary').textContent = accounts.length ? `${count} of ${accounts.length} selected` : '';

    $('char-count').innerHTML = providers.map((provider) => {
        const { icon, limit } = platformInfo(provider);
        const remaining = limit - text.length;
        let cls = 'char-pill';
        if (remaining < 0) cls += ' char-pill--over';
        else if (remaining <= Math.floor(limit * 0.2)) cls += ' char-pill--warning';
        return `<span class="${cls}" title="${esc(platformInfo(provider).label)}">${icon} ${remaining}</span>`;
    }).join('');

    const section = $('preview-section');
    if (!text.trim() || providers.length === 0) {
        section.classList.add('hidden');
        return;
    }
    section.classList.remove('hidden');
    $('preview-list').innerHTML = providers.map((provider) => {
        const { label, icon, limit } = platformInfo(provider);
        const chars = Array.from(text);
        const ok = esc(chars.slice(0, limit).join(''));
        const over = chars.length > limit ? `<mark>${esc(chars.slice(limit).join(''))}</mark>` : '';
        return `
            <div class="preview-card">
                <div class="preview-head"><span>${icon}</span><strong>${esc(label)}</strong>
                    <span>${text.length}/${limit}</span></div>
                <div class="preview-text">${ok}${over}</div>
            </div>`;
    }).join('');
}

function saveDraft() {
    const content = $('content').value;
    if (!content && mediaFiles.length === 0) {
        localStorage.removeItem(DRAFT_KEY);
        return;
    }
    localStorage.setItem(DRAFT_KEY, JSON.stringify({ content, accountIds: selectedIds }));
}

function restoreDraft() {
    try {
        const draft = JSON.parse(localStorage.getItem(DRAFT_KEY));
        if (draft) {
            $('content').value = draft.content || '';
            selectedIds = Array.isArray(draft.accountIds) ? draft.accountIds : null;
        }
    } catch (_) {
        localStorage.removeItem(DRAFT_KEY);
    }
}

function setupDraftAndCounter() {
    $('content').addEventListener('input', () => {
        saveDraft();
        updateComposeState();
    });

    $('clear-draft').addEventListener('click', () => {
        $('content').value = '';
        clearMedia();
        localStorage.removeItem(DRAFT_KEY);
        updateComposeState();
        $('content').focus();
    });
}

// ---------- Media: picker, drag & drop, clipboard paste ----------

function addFiles(files) {
    let added = 0;
    for (const file of files) {
        if (!file.type.startsWith('image/') && !file.type.startsWith('video/')) continue;
        const dup = mediaFiles.some((f) => f.name === file.name && f.size === file.size && f.lastModified === file.lastModified);
        if (dup) continue;
        mediaFiles.push(file);
        added++;
    }
    if (added) renderMediaPreviews();
    return added;
}

function setupMedia() {
    $('media').addEventListener('change', (e) => {
        addFiles(Array.from(e.target.files));
        e.target.value = '';
    });

    // Paste images/video from the clipboard anywhere on the page. Plain text paste is untouched.
    document.addEventListener('paste', (e) => {
        const files = Array.from(e.clipboardData?.files || []);
        if (files.length === 0) return;
        if (addFiles(files)) {
            e.preventDefault();
            toast(`Added ${files.length} file${files.length > 1 ? 's' : ''} from clipboard`);
        }
    });

    const box = $('compose-box');
    let depth = 0;
    const hasFiles = (e) => Array.from(e.dataTransfer?.types || []).includes('Files');

    document.addEventListener('dragenter', (e) => {
        if (!hasFiles(e)) return;
        depth++;
        box.classList.add('drag-over');
    });
    document.addEventListener('dragleave', (e) => {
        if (!hasFiles(e)) return;
        depth = Math.max(0, depth - 1);
        if (depth === 0) box.classList.remove('drag-over');
    });
    document.addEventListener('dragover', (e) => {
        if (hasFiles(e)) e.preventDefault();
    });
    document.addEventListener('drop', (e) => {
        if (!hasFiles(e)) return;
        e.preventDefault();
        depth = 0;
        box.classList.remove('drag-over');
        addFiles(Array.from(e.dataTransfer.files));
    });
}

function renderMediaPreviews() {
    const preview = $('media-preview');
    preview.querySelectorAll('img, video').forEach((el) => URL.revokeObjectURL(el.src));
    preview.innerHTML = '';

    mediaFiles.forEach((file, index) => {
        const item = document.createElement('div');
        item.className = 'media-preview-item';

        const mediaEl = document.createElement(file.type.startsWith('video/') ? 'video' : 'img');
        mediaEl.src = URL.createObjectURL(file);
        if (mediaEl.tagName === 'IMG') mediaEl.alt = file.name;
        else mediaEl.muted = true;

        const removeBtn = document.createElement('button');
        removeBtn.type = 'button';
        removeBtn.className = 'remove-btn';
        removeBtn.textContent = '×';
        removeBtn.setAttribute('aria-label', `Remove ${file.name}`);
        removeBtn.addEventListener('click', () => {
            mediaFiles.splice(index, 1);
            renderMediaPreviews();
        });

        item.append(mediaEl, removeBtn);
        preview.appendChild(item);
    });
}

function clearMedia() {
    mediaFiles = [];
    renderMediaPreviews();
}

// ---------- Submit ----------

function setupFormSubmit() {
    const form = $('compose-form');
    const submitBtn = $('submit-btn');

    $('content').addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
            e.preventDefault();
            form.requestSubmit();
        }
    });

    form.addEventListener('submit', async (e) => {
        e.preventDefault();

        const content = $('content').value.trim();
        if (!content) {
            toast('Please enter a message', 'error');
            return;
        }

        const accountIds = checkedIds();
        if (accountIds.length === 0) {
            toast('Select at least one account', 'error');
            return;
        }

        setBusy(submitBtn, true, 'Posting…');

        try {
            const uploadedMediaIds = [];
            for (const file of mediaFiles) {
                const formData = new FormData();
                formData.append('file', file);
                const uploadRes = await fetch(`${API_BASE}/media/upload`, { method: 'POST', body: formData });
                if (!uploadRes.ok) throw new Error(`Failed to upload ${file.name}`);
                uploadedMediaIds.push((await uploadRes.json()).id);
            }

            const response = await fetch(`${API_BASE}/posts`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ content, media_ids: uploadedMediaIds, account_ids: accountIds }),
            });
            const result = await response.json().catch(() => ({}));
            if (!response.ok) throw new Error(result.error || `Post failed (HTTP ${response.status})`);

            const results = result.results || [];
            displayResults(results);

            const failed = results.filter((r) => !r.success).length;
            if (failed === 0) {
                toast('Posted!', 'success');
                $('content').value = '';
                clearMedia();
                localStorage.removeItem(DRAFT_KEY);
                updateComposeState();
            } else {
                toast(`${failed} of ${results.length} failed. Your draft was kept.`, 'error');
            }
        } catch (error) {
            toast(error.message, 'error');
            console.error('Post error:', error);
        } finally {
            setBusy(submitBtn, false);
        }
    });
}

function displayResults(results) {
    $('results-section').classList.remove('hidden');
    $('results-list').innerHTML = results.map(renderResultItem).join('');
    $('results-section').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}
